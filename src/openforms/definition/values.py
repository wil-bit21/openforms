"""Per-type answer validation and normalization (spec §5.4 + Plan 02 clarifications)."""

from __future__ import annotations

import math
from decimal import Decimal
from typing import Any

from .common import is_bare_email, is_date, is_http_url, trim
from .patterns import anchored
from .types import Option, Validation

_STRING_TYPES = {"text", "textarea", "email", "url", "date", "select"}


def check_value(t: str, options: list[Option], v: Validation | None, raw: Any) -> tuple[Any, bool, str]:
    """Validate one answer. Returns ``(value, provided, message)``: ``message`` set means
    present but invalid; ``provided`` False with no message means "not provided"."""
    if raw is None:
        return None, False, ""
    if t in _STRING_TYPES:
        if not isinstance(raw, str):
            return None, False, "must be a string"
        s = trim(raw)
        if s == "":
            return None, False, ""
        msg = _check_string(t, options, v, s)
        if msg:
            return None, False, msg
        return s, True, ""
    if t == "number":
        n = to_number(raw)
        if n is None:
            return None, False, "must be a number"
        if v is not None and v.min is not None and n < v.min:
            return None, False, "must be at least " + format_number(v.min)
        if v is not None and v.max is not None and n > v.max:
            return None, False, "must be at most " + format_number(v.max)
        return n, True, ""
    if t == "checkbox":
        if not isinstance(raw, bool):
            return None, False, "must be true or false"
        return raw, True, ""
    if t == "multiselect":
        if not isinstance(raw, list) or not all(isinstance(x, str) for x in raw):
            return None, False, "must be a list of options"
        if not raw:
            return None, False, ""
        seen: set[str] = set()
        out: list[Any] = []
        for it in raw:
            if not _has_option(options, it):
                return None, False, "must only contain valid options"
            if it in seen:
                return None, False, "must not contain duplicates"
            seen.add(it)
            out.append(it)
        return out, True, ""
    return None, False, "unsupported field type"


def _check_string(t: str, options: list[Option], v: Validation | None, s: str) -> str:
    if t == "email" and not is_bare_email(s):
        return "must be a valid email address"
    if t == "url" and not is_http_url(s):
        return "must be an absolute http(s) URL"
    if t == "date" and not is_date(s):
        return "must be a date in YYYY-MM-DD format"
    if t == "select" and not _has_option(options, s):
        return "must be one of the options"
    if v is None:
        return ""
    n = len(s)  # code points
    if v.min_length is not None and n < v.min_length:
        return f"must be at least {v.min_length} characters"
    if v.max_length is not None and n > v.max_length:
        return f"must be at most {v.max_length} characters"
    if v.pattern:
        re = anchored(v.pattern)
        if re is not None and re.search(s) is None:
            return "must match the required format"
    return ""


def _has_option(options: list[Option], v: str) -> bool:
    return any(o.value == v for o in options)


def to_number(v: Any) -> int | float | None:
    """JSON number → int (when integral) or float; None for non-numbers, bools, NaN, ±Inf."""
    if isinstance(v, bool) or not isinstance(v, (int, float)):
        return None
    if isinstance(v, float):
        if math.isnan(v) or math.isinf(v):
            return None
        if v.is_integer() and abs(v) < 1e21:
            return int(v)
    return v


def format_number(f: float) -> str:
    """Go ``strconv.FormatFloat(f, 'f', -1, 64)``."""
    if isinstance(f, int) or float(f).is_integer():
        return str(int(f))
    return format(Decimal(repr(float(f))), "f")


def _norm(v: Any) -> Any:
    n = to_number(v)
    if n is not None:
        return ("n", float(n))
    if isinstance(v, bool):
        return ("b", v)
    if isinstance(v, list):
        return ("l", tuple(_norm(x) for x in v))
    if isinstance(v, dict):
        return ("d", tuple(sorted((k, _norm(x)) for k, x in v.items())))
    return ("v", v)


def json_equal(a: Any, b: Any) -> bool:
    """Deep equality of JSON values; numbers compare numerically, booleans never equal numbers."""
    return _norm(a) == _norm(b)

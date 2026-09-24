"""Shared rules: name patterns, Go-compatible whitespace, e-mail/URL/date checks, options."""

from __future__ import annotations

import datetime as _dt
import re
from urllib.parse import urlsplit

from .problems import Problems
from .types import Option

SLUG_RE = re.compile(r"[a-z0-9][a-z0-9-]{0,62}")
KEY_RE = re.compile(r"[a-zA-Z][a-zA-Z0-9_]{0,63}")
SLUG_RULE = "must match ^[a-z0-9][a-z0-9-]{0,62}$"
KEY_RULE = "must match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$"

# Go's unicode.IsSpace set (Python's str.isspace also counts \x1c-\x1f).
GO_SPACE = "\t\n\v\f\r \x85\xa0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000"


def is_slug(s: object) -> bool:
    return isinstance(s, str) and SLUG_RE.fullmatch(s) is not None


def is_key(s: object) -> bool:
    return isinstance(s, str) and KEY_RE.fullmatch(s) is not None


def trim(s: str) -> str:
    return s.strip(GO_SPACE)


def blank(s: str) -> bool:
    return trim(s) == ""


_ATEXT = r"[A-Za-z0-9!#$%&'*+/=?^_`{|}~\-\u0080-\U0010ffff]"
_DOT_ATOM = rf"{_ATEXT}+(?:\.{_ATEXT}+)*"
_QUOTED = r'"(?:[^"\\\r\n]|\\.)*"'
_EMAIL_RE = re.compile(rf"(?:{_DOT_ATOM}|{_QUOTED})@(?:{_DOT_ATOM}|\[[^\[\]\\\r\n]*\])")


def is_bare_email(s: str) -> bool:
    """A bare RFC 5322 addr-spec (``local@domain``), as Go's ``mail.ParseAddress``
    accepts it with no display name and nothing around it."""
    return _EMAIL_RE.fullmatch(s) is not None


def is_http_url(s: str) -> bool:
    """Absolute http(s) URL with a host (Go ``url.Parse`` + scheme/host check)."""
    if any(c.isspace() or ord(c) < 0x20 or ord(c) == 0x7F for c in s):
        return False
    try:
        u = urlsplit(s)
        _ = u.port  # raises on an invalid port, as Go does
    except ValueError:
        return False
    host = u.netloc.rpartition("@")[2]
    return u.scheme in ("http", "https") and host != ""


_DATE_RE = re.compile(r"\d{4}-\d{2}-\d{2}")


def is_date(s: str) -> bool:
    if _DATE_RE.fullmatch(s) is None:
        return False
    try:
        _dt.date.fromisoformat(s)
    except ValueError:
        return False
    return True


def validate_options(ps: Problems, path: str, allowed: bool, not_allowed_msg: str, opts: list[Option]) -> None:
    """Check an options list. ``allowed`` says whether the owning field type takes
    options; when it does, at least one option is required."""
    if not allowed:
        if opts:
            ps.add(path + ".options", not_allowed_msg)
        return
    if not opts:
        ps.add(path + ".options", "at least one option is required")
        return
    seen: set[str] = set()
    for j, o in enumerate(opts):
        op = f"{path}.options[{j}]"
        if o.value == "":
            ps.add(op + ".value", "is required")
        elif trim(o.value) != o.value:
            ps.add(op + ".value", "must not start or end with whitespace")
        elif o.value in seen:
            ps.add(op + ".value", f'duplicate option value "{o.value}"')
        seen.add(o.value)
        if blank(o.label):
            ps.add(op + ".label", "is required")

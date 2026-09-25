"""Visibility and submission validation (spec §5.4). One evaluator serves both,
so ``visible`` and ``validate_submission`` can never disagree."""

from __future__ import annotations

from typing import Any

from .problems import Problems
from .types import FIELD_CHECKBOX, FIELD_MULTISELECT, Condition, Form
from .values import check_value, json_equal


def visible(f: Form, data: dict[str, Any] | None) -> dict[str, bool]:
    """Whether each field is shown for ``data`` (every field key is present)."""
    vis, _, _ = _evaluate(f, data or {})
    return vis


def validate_submission(f: Form, data: dict[str, Any] | None) -> dict[str, Any]:
    """Return cleaned data (unknown keys, hidden fields and blanks dropped; strings
    trimmed) or raise ValidationError with ``data.<key>`` paths in field order."""
    _, clean, ps = _evaluate(f, data or {})
    ps.raise_if_any()
    return clean


def _evaluate(f: Form, data: dict[str, Any]) -> tuple[dict[str, bool], dict[str, Any], Problems]:
    vis: dict[str, bool] = {}
    clean: dict[str, Any] = {}
    types: dict[str, str] = {}
    ps = Problems()
    for fld in f.field_list():
        types[fld.key] = fld.type
        shown = True
        c = fld.show_if
        if c is not None:
            if not vis.get(c.field, False):
                shown = False
            else:
                shown = _holds(c, types.get(c.field, ""), clean.get(c.field), c.field in clean)
        vis[fld.key] = shown
        if not shown:
            continue
        path = "data." + fld.key
        val, provided, msg = check_value(fld.type, fld.options, fld.validation, data.get(fld.key))
        if msg:
            ps.add(path, msg)
        elif not provided:
            if fld.required:
                ps.add(path, "is required")
        elif fld.type == FIELD_CHECKBOX and fld.required and val is False:
            ps.add(path, "must be checked")
        else:
            clean[fld.key] = val
    return vis, clean, ps


def _contains(items: list[Any], v: Any) -> bool:
    return any(json_equal(it, v) for it in items)


def _holds(c: Condition, ctrl: str, val: Any, present: bool) -> bool:
    multi = ctrl == FIELD_MULTISELECT
    if c.equals is not None:
        if not present:
            return False
        return _contains(val, c.equals) if multi else json_equal(val, c.equals)
    if c.not_equals is not None:
        if not present:
            return True
        return not _contains(val, c.not_equals) if multi else not json_equal(val, c.not_equals)
    if c.in_ is not None:
        if not present:
            return False
        if multi:
            return any(_contains(c.in_, item) for item in val)
        return _contains(c.in_, val)
    return True

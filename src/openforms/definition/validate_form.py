"""Semantic validation of forms (spec §5.3)."""

from __future__ import annotations

from .common import KEY_RULE, SLUG_RULE, blank, is_key, is_slug, validate_options
from .patterns import compile_error
from .problems import Problems, ValidationError
from .types import FIELD_MULTISELECT, FIELD_NUMBER, FIELD_SELECT, Condition, Field, Form

FORM_FIELD_TYPES = {"text", "textarea", "email", "number", "select", "multiselect", "checkbox", "date", "url"}
STRING_FIELD_TYPES = {"text", "textarea", "email", "url"}


def validate_form(f: Form) -> None:
    """Raise ValidationError listing every problem in ``f``."""
    ps = Problems()
    if not is_slug(f.slug):
        ps.add("slug", SLUG_RULE)
    if blank(f.title):
        ps.add("title", "is required")
    if f.workflow and not is_slug(f.workflow):
        ps.add("workflow", "must be a valid workflow slug")
    fields = f.field_list()
    if not fields:
        ps.add("fields", "at least one field is required")
    seen: dict[str, int] = {}
    for i, fld in enumerate(fields):
        p = f"fields[{i}]"
        if not is_key(fld.key):
            ps.add(p + ".key", KEY_RULE)
        elif fld.key in seen:
            ps.add(p + ".key", f'duplicate key "{fld.key}" (also used by fields[{seen[fld.key]}])')
        else:
            seen[fld.key] = i
        if fld.type not in FORM_FIELD_TYPES:
            ps.add(p + ".type", f'unknown field type "{fld.type}"')
        if blank(fld.label):
            ps.add(p + ".label", "is required")
        validate_options(
            ps,
            p,
            fld.type in (FIELD_SELECT, FIELD_MULTISELECT),
            "options are only allowed on select and multiselect fields",
            fld.options,
        )
        _validate_field_validation(ps, p, fld)
        if fld.show_if is not None:
            _validate_condition(ps, p + ".showIf", fld.show_if, seen, i)
    ps.raise_if_any()


def form_error(f: Form) -> ValidationError | None:
    try:
        validate_form(f)
    except ValidationError as e:
        return e
    return None


def _validate_field_validation(ps: Problems, p: str, fld: Field) -> None:
    v = fld.validation
    if v is None:
        return
    vp = p + ".validation"
    if fld.type not in STRING_FIELD_TYPES:
        msg = "is only allowed on text, textarea, email and url fields"
        if v.min_length is not None:
            ps.add(vp + ".minLength", "minLength " + msg)
        if v.max_length is not None:
            ps.add(vp + ".maxLength", "maxLength " + msg)
        if v.pattern:
            ps.add(vp + ".pattern", "pattern " + msg)
    if fld.type != FIELD_NUMBER:
        if v.min is not None:
            ps.add(vp + ".min", "min is only allowed on number fields")
        if v.max is not None:
            ps.add(vp + ".max", "max is only allowed on number fields")
    if v.min_length is not None and v.min_length < 0:
        ps.add(vp + ".minLength", "must be 0 or greater")
    if v.max_length is not None and v.max_length < 0:
        ps.add(vp + ".maxLength", "must be 0 or greater")
    if v.min_length is not None and v.max_length is not None and v.min_length > v.max_length:
        ps.add(vp + ".maxLength", "must be greater than or equal to minLength")
    if v.min is not None and v.max is not None and v.min > v.max:
        ps.add(vp + ".max", "must be greater than or equal to min")
    if v.pattern:
        err = compile_error(v.pattern)
        if err is not None:
            ps.add(vp + ".pattern", "is not a valid regular expression: " + err)


def _validate_condition(ps: Problems, p: str, c: Condition, seen: dict[str, int], i: int) -> None:
    j = seen.get(c.field)
    if j is None or j >= i:
        ps.add(p + ".field", f'must reference a field declared before this one (got "{c.field}")')
    n = sum(x is not None for x in (c.equals, c.not_equals, c.in_))
    if n != 1:
        ps.add(p, "exactly one of equals, notEquals or in is required")
    elif c.in_ is not None and len(c.in_) == 0:
        ps.add(p + ".in", "must not be empty")

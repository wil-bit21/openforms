"""Bundle validation: every definition, duplicate slugs and workflow references."""

from __future__ import annotations

from collections.abc import Iterable

from .problems import Problems, ValidationError
from .types import Form, Workflow
from .validate_form import validate_form
from .validate_workflow import validate_workflow


def _err(fn, arg) -> ValidationError | None:
    try:
        fn(arg)
    except ValidationError as e:
        return e
    return None


def validate_bundle(forms: list[Form], workflows: list[Workflow], existing_workflow_slugs: Iterable[str] = ()) -> None:
    """Problem paths are prefixed ``forms[i]`` / ``workflows[i]``."""
    ps = Problems()
    known = set(existing_workflow_slugs)
    seen_w: set[str] = set()
    for i, w in enumerate(workflows):
        p = f"workflows[{i}]"
        ps.merge(p, _err(validate_workflow, w))
        if w.slug in seen_w:
            ps.add(p + ".slug", f'duplicate workflow slug "{w.slug}"')
        seen_w.add(w.slug)
        known.add(w.slug)
    seen_f: set[str] = set()
    for i, f in enumerate(forms):
        p = f"forms[{i}]"
        ps.merge(p, _err(validate_form, f))
        if f.slug in seen_f:
            ps.add(p + ".slug", f'duplicate form slug "{f.slug}"')
        seen_f.add(f.slug)
        if f.workflow and f.workflow not in known:
            ps.add(p + ".workflow", f'unknown workflow "{f.workflow}"')
    ps.raise_if_any()

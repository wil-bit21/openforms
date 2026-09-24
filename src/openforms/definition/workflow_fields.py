"""Validation of workflow-managed field patches."""

from __future__ import annotations

from typing import Any

from .problems import Problems
from .types import Workflow
from .values import check_value


def validate_workflow_fields(w: Workflow, fields: dict[str, Any]) -> dict[str, Any]:
    """Validate a patch against ``w.fields``. The result has the same keys: a
    normalized value, or None when the patch unsets the field (null or blank).
    Callers merge it: None → delete the key, otherwise set it. Problems use paths
    ``fields.<key>`` sorted by key."""
    out: dict[str, Any] = {}
    ps = Problems()
    for k in sorted(fields):
        path = "fields." + k
        wf = w.field(k)
        if wf is None:
            ps.add(path, "unknown workflow field")
            continue
        val, provided, msg = check_value(wf.type, wf.options, None, fields[k])
        if msg:
            ps.add(path, msg)
        elif not provided:
            out[k] = None
        else:
            out[k] = val
    ps.raise_if_any()
    return out

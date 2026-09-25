"""``{{a.b.c}}`` template substitution (plain text, no HTML escaping)."""

from __future__ import annotations

import json
import re
from typing import Any

from ...definition.canonical import normalize_json
from ...definition.values import format_number

_PLACEHOLDER = re.compile(r"\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}")


def render(tmpl: str, variables: dict[str, Any]) -> str:
    """Replace placeholders with values looked up in ``variables``; missing paths render as ""."""
    return _PLACEHOLDER.sub(lambda m: _format(_lookup(variables, m.group(1).split("."))), tmpl)


def _lookup(v: Any, parts: list[str]) -> Any:
    cur = v
    for p in parts:
        if not isinstance(cur, dict) or p not in cur:
            return None
        cur = cur[p]
    return cur


def _format(v: Any) -> str:
    if v is None:
        return ""
    if isinstance(v, str):
        return v
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return format_number(v)
    if isinstance(v, list):
        return ", ".join(_format(e) for e in v)
    if isinstance(v, dict):
        return json.dumps(normalize_json(v), sort_keys=True, separators=(",", ":"), ensure_ascii=False)
    return str(v)

"""Canonical JSON and hashing (spec §5.5)."""

from __future__ import annotations

import hashlib
import json
from typing import Any


def normalize_json(v: Any) -> Any:
    if isinstance(v, bool) or v is None or isinstance(v, str):
        return v
    if isinstance(v, float) and v.is_integer() and abs(v) < 1e21:
        return int(v)
    if isinstance(v, dict):
        return {str(k): normalize_json(x) for k, x in v.items()}
    if isinstance(v, (list, tuple)):
        return [normalize_json(x) for x in v]
    return v


def canonical_json(v: Any) -> bytes:
    """Sorted keys, no insignificant whitespace, HTML characters unescaped.
    Accepts definition objects (anything with ``to_dict``) or plain JSON values."""
    if hasattr(v, "to_dict"):
        v = v.to_dict()
    text = json.dumps(normalize_json(v), sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False)
    # encoding/json escapes these two even with SetEscapeHTML(false)
    text = text.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return text.encode("utf-8")


def canonical(v: Any) -> tuple[bytes, str]:
    """Return the canonical bytes of ``v`` and their lowercase hex sha256."""
    out = canonical_json(v)
    return out, hashlib.sha256(out).hexdigest()

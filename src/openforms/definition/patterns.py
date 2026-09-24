"""RE2 patterns (the Go server used RE2; this keeps syntax and linear-time matching)."""

from __future__ import annotations

from functools import lru_cache

import re2

_OPTS = re2.Options()
_OPTS.log_errors = False


def compile_error(pattern: str) -> str | None:
    """Return why ``pattern`` does not compile, or None when it is valid."""
    try:
        re2.compile(pattern, options=_OPTS)
    except re2.error as e:
        msg = e.args[0] if e.args else str(e)
        return msg.decode() if isinstance(msg, bytes) else str(msg)
    return None


@lru_cache(maxsize=1024)
def anchored(pattern: str):  # -> re2._Regexp | None
    """``^(?:pattern)$``; None when uncompilable (rejected at definition time)."""
    try:
        return re2.compile(f"^(?:{pattern})$", options=_OPTS)
    except re2.error:
        return None

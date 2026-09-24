"""Small helpers shared by the model modules."""

from __future__ import annotations

from sqlalchemy.exc import IntegrityError


def is_unique_violation(err: BaseException) -> bool:
    orig = getattr(err, "orig", None)
    code = getattr(orig, "sqlstate", None) or getattr(getattr(orig, "__cause__", None), "sqlstate", None)
    return isinstance(err, IntegrityError) and code == "23505"

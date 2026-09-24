"""Validation problems shared by definitions, submissions and the HTTP layer."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Problem:
    """One validation failure. ``path`` is JSON-pointer-ish (``fields[2].showIf.field``,
    ``data.email``); ``""`` means the whole document."""

    path: str
    message: str

    def to_dict(self) -> dict[str, str]:
        return {"path": self.path, "message": self.message}


class ValidationError(Exception):
    """Raised for any invalid definition or submission."""

    def __init__(self, problems: list[Problem] | None = None) -> None:
        self.problems: list[Problem] = list(problems or [])
        super().__init__(str(self))

    def __str__(self) -> str:
        if not self.problems:
            return "validation failed"
        parts = [p.message if not p.path else f"{p.path}: {p.message}" for p in self.problems]
        return "validation failed: " + "; ".join(parts)


def join_path(prefix: str, path: str) -> str:
    if not prefix:
        return path
    if not path:
        return prefix
    return f"{prefix}.{path}"


class Problems(list[Problem]):
    """Collects problems while validating."""

    def add(self, path: str, message: str) -> None:
        self.append(Problem(path, message))

    def merge(self, prefix: str, err: BaseException | None) -> None:
        """Append the problems of ``err`` with ``prefix`` prepended to their paths."""
        if err is None:
            return
        if not isinstance(err, ValidationError):
            self.add(prefix, str(err))
            return
        for p in err.problems:
            self.add(join_path(prefix, p.path), p.message)

    def error(self) -> ValidationError | None:
        return ValidationError(list(self)) if self else None

    def raise_if_any(self) -> None:
        if self:
            raise ValidationError(list(self))

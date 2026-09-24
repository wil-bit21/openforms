"""The demo bundle shipped with the package (``openforms seed --demo`` and the /demo page)."""

from __future__ import annotations

from collections.abc import Callable
from pathlib import Path
from typing import TypeVar

from ._paths import examples_dir
from .definition import Form, Workflow, parse_form, parse_workflow

T = TypeVar("T")


def _parse_dir(d: Path, parse: Callable[[bytes], T]) -> list[T]:
    out = []
    for p in sorted(d.glob("*.yaml"), key=lambda p: p.name):
        try:
            out.append(parse(p.read_bytes()))
        except Exception as e:
            raise ValueError(f"{d.name}/{p.name}: {e}") from e
    return out


def bundle() -> tuple[list[Form], list[Workflow]]:
    """Every example form and workflow, sorted by file name."""
    root = examples_dir()
    return _parse_dir(root / "forms", parse_form), _parse_dir(root / "workflows", parse_workflow)

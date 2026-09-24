"""Reading a local definitions directory (``<dir>/forms`` and ``<dir>/workflows``)."""

from __future__ import annotations

from dataclasses import dataclass, field
from pathlib import Path
from typing import TextIO

from ..client import Bundle
from ..definition import ValidationError, parse_form, parse_workflow, validate_bundle


@dataclass(frozen=True)
class LocalProblem:
    """One validation finding for a local definition file."""

    file: str  # slash-separated, relative to the definitions directory
    path: str  # inside the document, e.g. "fields[2].showIf.field"
    message: str

    def __str__(self) -> str:
        return f"{self.file}: {self.message}" if not self.path else f"{self.file}:{self.path}: {self.message}"


@dataclass
class LocalBundle:
    bundle: Bundle = field(default_factory=Bundle)
    form_files: dict[str, str] = field(default_factory=dict)
    workflow_files: dict[str, str] = field(default_factory=dict)


_BOM = b"\xef\xbb\xbf"


def load_local(d: Path) -> tuple[LocalBundle, list[LocalProblem]]:
    """Parse and validate every file and check cross-references. OSErrors propagate."""
    lb = LocalBundle()
    problems: list[LocalProblem] = []
    for kind in ("forms", "workflows"):
        for p in definition_files(d / kind):
            rel = f"{kind}/{p.name}"
            raw = p.read_bytes().removeprefix(_BOM)
            base = p.stem
            parse = parse_form if kind == "forms" else parse_workflow
            try:
                v = parse(raw)
            except ValidationError as e:
                problems += _to_problems(rel, e)
                continue
            seen = lb.form_files if kind == "forms" else lb.workflow_files
            if v.slug != base:
                problems.append(LocalProblem(rel, "slug", f'slug "{v.slug}" must match file name "{base}"'))
                continue
            if v.slug in seen:
                label = "form" if kind == "forms" else "workflow"
                problems.append(
                    LocalProblem(rel, "slug", f'duplicate {label} slug "{v.slug}" (also defined in {seen[v.slug]})')
                )
                continue
            seen[v.slug] = rel
            (lb.bundle.forms if kind == "forms" else lb.bundle.workflows).append(v)  # type: ignore[arg-type]

    for f in lb.bundle.forms:
        if f.workflow and f.workflow not in lb.workflow_files:
            problems.append(
                LocalProblem(
                    lb.form_files[f.slug], "workflow", f'unknown workflow "{f.workflow}" (not found in workflows/)'
                )
            )
    # Backstop: any remaining bundle-level rule from the definition package.
    if not problems:
        try:
            validate_bundle(lb.bundle.forms, lb.bundle.workflows)
        except ValidationError as e:
            problems += _to_problems("(bundle)", e)
    problems.sort(key=lambda p: (p.file, p.path))
    return lb, problems


def _to_problems(file: str, e: ValidationError) -> list[LocalProblem]:
    if e.problems:
        return [LocalProblem(file, p.path, p.message) for p in e.problems]
    return [LocalProblem(file, "", str(e))]


def definition_files(d: Path) -> list[Path]:
    """*.yaml, *.yml and *.json files directly inside ``d`` (sorted); none if it is missing."""
    try:
        entries = list(d.iterdir())
    except FileNotFoundError:
        return []
    return sorted(
        (e for e in entries if not e.is_dir() and e.suffix.lower() in (".yaml", ".yml", ".json")),
        key=lambda e: str(e),
    )


def print_problems(w: TextIO, problems: list[LocalProblem]) -> None:
    for p in problems:
        print(p, file=w)
    print(f"{len(problems)} problem(s) found", file=w)

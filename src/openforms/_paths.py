"""Locations of data shipped with the package (schemas, example bundle, web UI).

In a wheel, ``schemas/`` and ``examples/openforms/`` are force-included as
``openforms/_schemas`` and ``openforms/_examples``. In a source checkout
(editable install) they are read from the repository root.
"""

from __future__ import annotations

from pathlib import Path

PACKAGE_DIR = Path(__file__).resolve().parent
REPO_ROOT = PACKAGE_DIR.parent.parent


def _first_existing(*candidates: Path) -> Path:
    for c in candidates:
        if c.exists():
            return c
    return candidates[0]


def schemas_dir() -> Path:
    return _first_existing(PACKAGE_DIR / "_schemas", REPO_ROOT / "schemas")


def examples_dir() -> Path:
    return _first_existing(PACKAGE_DIR / "_examples", REPO_ROOT / "examples" / "openforms")


def ui_dir() -> Path:
    return PACKAGE_DIR / "server" / "ui"

"""What the commands touch in the outside world (so tests can swap it), project config
resolution and error reporting."""

from __future__ import annotations

import os
import sys
import uuid
from collections.abc import AsyncIterator, Callable
from contextlib import AbstractAsyncContextManager, asynccontextmanager
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any, Protocol, TextIO

from ..client import APIError, ApplyResult, Bundle, Client, ValidateResult
from ..server.schemas.core import ROLE_ADMIN

CONFIG_FILE = "openforms.yaml"


class CLIError(Exception):
    """A failure reported as ``Error: <message>`` with exit status 1."""


class ProblemsError(CLIError):
    """Returned after validation problems have been printed."""

    def __init__(self) -> None:
        super().__init__("definitions are invalid")


class DriftError(CLIError):
    """``pull --check`` found local files that differ from the server."""

    def __init__(self) -> None:
        super().__init__("local definitions differ from the server")


class Remote(Protocol):
    """The subset of the API the CLI needs; :class:`openforms.client.Client` implements it."""

    async def validate(self, b: Bundle) -> ValidateResult: ...
    async def apply(self, b: Bundle, dry_run: bool = False) -> ApplyResult: ...
    async def export(self) -> Bundle: ...


OpenAuth = Callable[[], AbstractAsyncContextManager[tuple[Any, uuid.UUID]]]


@asynccontextmanager
async def open_auth_from_env() -> AsyncIterator[tuple[Any, uuid.UUID]]:
    """Open the database, migrate and ensure the default org; yields ``(db, org_id)``."""
    from ..server.database.alembic_commands import migrate
    from ..server.database.engine import Database
    from ..server.models import auth
    from ..settings import load_settings

    settings = load_settings()
    if not settings.database_url:
        raise CLIError("OPENFORMS_DATABASE_URL is required for admin commands")
    db = Database(settings.database_url)
    try:
        await migrate(db.engine)
        async with db.transaction() as s:
            org_id = await auth.ensure_default_org(s)
        yield db, org_id
    finally:
        await db.dispose()


@dataclass
class Deps:
    out: TextIO = field(default_factory=lambda: sys.stdout)
    err: TextIO = field(default_factory=lambda: sys.stderr)
    work_dir: Path = field(default_factory=Path.cwd)
    getenv: Callable[[str], str | None] = os.environ.get
    new_client: Callable[[str, str], Remote] = Client
    open_auth: OpenAuth = open_auth_from_env

    def abs(self, p: str) -> Path:
        path = Path(p)
        return path if path.is_absolute() else self.work_dir / path

    def resolve_dir(self, flag: str, pc: ProjectConfig) -> Path:
        return self.abs(flag or pc.dir or "openforms")

    def resolve_remote(self, server: str, api_key: str, pc: ProjectConfig) -> tuple[str, str]:
        server = server or self.getenv("OPENFORMS_URL") or pc.server
        if not server:
            raise CLIError("no server configured: pass --server, set OPENFORMS_URL, or add `server:` to openforms.yaml")
        key = api_key or self.getenv("OPENFORMS_API_KEY") or ""
        if not key:
            raise CLIError(
                "no API key: pass --api-key or set OPENFORMS_API_KEY "
                "(create one with `openforms admin create-api-key --name cli --roles admin`)"
            )
        return server, key


@dataclass
class ProjectConfig:
    """openforms.yaml. API keys are deliberately not supported here."""

    server: str = ""
    dir: str = ""


def load_project_config(work_dir: Path) -> ProjectConfig:
    from ..definition import load_yaml

    try:
        raw = (work_dir / CONFIG_FILE).read_bytes()
    except FileNotFoundError:
        return ProjectConfig()
    try:
        doc = load_yaml(raw)
    except ValueError as e:
        raise CLIError(f"{CONFIG_FILE}: error converting YAML to JSON: {e}") from None
    if doc is None:
        return ProjectConfig()
    if not isinstance(doc, dict):
        raise CLIError(f"{CONFIG_FILE}: error unmarshaling JSON: json: cannot unmarshal into ProjectConfig")
    for k, v in doc.items():
        if k not in ("server", "dir"):
            raise CLIError(f'{CONFIG_FILE}: error unmarshaling JSON: while decoding JSON: json: unknown field "{k}"')
        if v is not None and not isinstance(v, str):
            raise CLIError(f"{CONFIG_FILE}: error unmarshaling JSON: json: {k} must be a string")
    return ProjectConfig(server=doc.get("server") or "", dir=doc.get("dir") or "")


def explain_remote_error(err_out: TextIO, e: Exception) -> Exception:
    """Turn API errors into actionable messages. 422 details are printed as problems."""
    from .local import LocalProblem, print_problems

    if not isinstance(e, APIError):
        return e
    if e.status == 401:
        return CLIError(f"authentication failed ({e.code}): check --api-key or OPENFORMS_API_KEY")
    if e.status == 403:
        return CLIError(f'permission denied ({e.code}): the API key needs the "{ROLE_ADMIN}" role')
    if e.status == 422 and e.details:
        print_problems(err_out, [LocalProblem("server", p.path, p.message) for p in e.details])
        return ProblemsError()
    return e

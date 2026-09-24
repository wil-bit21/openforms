"""The ``openforms`` command: the server (serve, migrate, seed, admin) and the developer
CLI (init, validate, push, pull, diff) in one entry point."""

from __future__ import annotations

import asyncio
import sys
from collections.abc import Awaitable
from typing import Annotated

from cyclopts import App, Parameter

from ..client import APIError, ClientError
from ..definition import ValidationError
from ..settings import ConfigError
from . import commands
from .deps import CLIError, Deps

app = App(
    name="openforms",
    help="Developer-first forms with submission workflows.",
    version_flags=[],
    default_parameter=Parameter(show_default=False),
)
admin = App(
    name="admin", help="Administrative commands that talk directly to the database (needs OPENFORMS_DATABASE_URL)."
)
app.command(admin)

DirFlag = Annotated[
    str, Parameter(name="--dir", help="definitions directory (default: dir from openforms.yaml, else ./openforms)")
]
ServerFlag = Annotated[
    str,
    Parameter(name="--server", help="openforms server URL (default: $OPENFORMS_URL, else server from openforms.yaml)"),
]
KeyFlag = Annotated[str, Parameter(name="--api-key", help="API key with the admin role (default: $OPENFORMS_API_KEY)")]


def run(aw: Awaitable[None]) -> None:
    """Run a command; failures print ``Error: <message>`` and exit 1, as the Go CLI did."""
    try:
        asyncio.run(aw)  # type: ignore[arg-type]
    except (CLIError, ConfigError, ValidationError, APIError, ClientError, OSError) as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)


@app.command
def serve() -> None:
    """Run the openforms server (migrates on start)."""
    import uvicorn

    from ..server.api.server import create_app
    from ..settings import load_settings

    try:
        settings = load_settings()
        settings.require_database()
    except ConfigError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
    host, port = settings.listen_host_port
    uvicorn.run(create_app(settings), host=host, port=port, log_level="info", proxy_headers=False)


@app.command
def migrate() -> None:
    """Apply database migrations and exit."""
    run(commands.run_migrate(Deps()))


@app.command
def seed(*, demo: Annotated[bool, Parameter(negative=())] = False) -> None:
    """Load the demo forms, workflows and users (idempotent).

    Parameters
    ----------
    demo
        seed the demo bundle and demo users
    """

    async def go() -> None:
        from ..settings import load_settings

        if not demo:
            raise CLIError("nothing to seed: pass --demo")
        if not load_settings().database_url:
            raise CLIError("OPENFORMS_DATABASE_URL is required")
        await commands.run_seed_demo(Deps())

    run(go())


@admin.command(name="create-user")
def create_user(*, email: str = "", name: str = "", password: str = "", roles: str = "") -> None:
    """Create a user who can sign in to the admin UI.

    Parameters
    ----------
    email
        email address (required)
    name
        display name (default: part of the email before @)
    password
        password, at least 8 characters (required)
    roles
        comma-separated roles, e.g. admin,reviewer
    """
    run(commands.admin_create_user(Deps(), email, name, password, roles))


@admin.command(name="create-api-key")
def create_api_key(*, name: str = "", roles: str = "") -> None:
    """Create an API key (printed once).

    Parameters
    ----------
    name
        a label for the key, e.g. ci (required)
    roles
        comma-separated roles; use admin for push/pull
    """
    run(commands.admin_create_api_key(Deps(), name, roles))


@app.command
def init(dir: str = "", /) -> None:  # noqa: A002
    """Scaffold openforms.yaml and a sample form + workflow."""
    run(commands.run_init(Deps(), dir))


@app.command
def validate(*, dir: DirFlag = "") -> None:  # noqa: A002
    """Validate local form and workflow definitions (offline)."""
    run(commands.run_validate(Deps(), dir))


@app.command
def push(
    *,
    dir: DirFlag = "",  # noqa: A002
    server: ServerFlag = "",
    api_key: KeyFlag = "",
    dry_run: Annotated[
        bool, Parameter(name="--dry-run", negative=(), help="show what would change without applying")
    ] = False,
) -> None:
    """Validate local definitions and apply them to the server."""
    run(commands.run_push(Deps(), dir, server, api_key, dry_run))


@app.command
def pull(
    *,
    dir: DirFlag = "",  # noqa: A002
    server: ServerFlag = "",
    api_key: KeyFlag = "",
    check: Annotated[
        bool, Parameter(negative=(), help="write nothing; exit 1 if any file would change (CI drift check)")
    ] = False,
) -> None:
    """Write the server's definitions to local files as canonical YAML."""
    run(commands.run_pull(Deps(), dir, server, api_key, check))


@app.command
def diff(*, dir: DirFlag = "", server: ServerFlag = "", api_key: KeyFlag = "") -> None:  # noqa: A002
    """Show which definitions differ between local files and the server."""
    run(commands.run_diff(Deps(), dir, server, api_key))


def main() -> None:
    app()

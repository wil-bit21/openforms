"""Programmatic Alembic commands (the equivalent of Prefect's ``alembic_commands``)."""

from __future__ import annotations

from pathlib import Path

from alembic import command
from alembic.config import Config
from sqlalchemy.ext.asyncio import AsyncEngine

HERE = Path(__file__).resolve().parent


def alembic_config(url: str | None = None) -> Config:
    cfg = Config(str(HERE / "alembic.ini"))
    cfg.set_main_option("script_location", str(HERE / "_migrations"))
    if url:
        cfg.attributes["url"] = url
    return cfg


def alembic_upgrade(url: str, revision: str = "head") -> None:
    """Apply migrations synchronously (runs its own event loop; use from sync code)."""
    command.upgrade(alembic_config(url), revision)


def alembic_downgrade(url: str, revision: str = "-1") -> None:
    command.downgrade(alembic_config(url), revision)


def alembic_revision(message: str, autogenerate: bool = False) -> None:
    command.revision(alembic_config(), message=message, autogenerate=autogenerate)


async def migrate(engine: AsyncEngine) -> None:
    """Apply all pending migrations over an existing async engine (from async code)."""

    def _upgrade(connection) -> None:
        cfg = alembic_config()
        cfg.attributes["connection"] = connection
        command.upgrade(cfg, "head")

    async with engine.begin() as conn:
        await conn.run_sync(_upgrade)

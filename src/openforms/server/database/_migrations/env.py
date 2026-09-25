"""Alembic environment: async engine, URL from ``config.attributes['url']`` or
``OPENFORMS_DATABASE_URL``; honours ``search_path`` in the URL (test schemas)."""

from __future__ import annotations

import asyncio

from alembic import context
from sqlalchemy.engine import Connection

from openforms.server.database.engine import create_engine
from openforms.server.database.orm_models import Base
from openforms.settings import load_settings

config = context.config
target_metadata = Base.metadata


def _url() -> str:
    url = config.attributes.get("url") or load_settings().database_url
    if not url:
        raise RuntimeError("OPENFORMS_DATABASE_URL is required")
    return url


def _run(connection: Connection) -> None:
    context.configure(connection=connection, target_metadata=target_metadata, compare_type=True)
    with context.begin_transaction():
        context.run_migrations()


async def _run_async() -> None:
    engine = create_engine(_url(), pool_size=1, max_overflow=0)
    try:
        async with engine.connect() as conn:
            await conn.run_sync(_run)
            await conn.commit()
    finally:
        await engine.dispose()


def run_migrations_online() -> None:
    conn = config.attributes.get("connection")
    if conn is not None:
        _run(conn)
        return
    asyncio.run(_run_async())


if context.is_offline_mode():
    raise RuntimeError("offline migrations are not supported")
run_migrations_online()

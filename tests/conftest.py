"""Shared fixtures. DB-backed tests get an isolated, migrated Postgres schema
(port of the Go ``dbtest`` package)."""

from __future__ import annotations

import os
import secrets
from collections.abc import AsyncIterator
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

import asyncpg
import pytest

from openforms.server.database.alembic_commands import migrate
from openforms.server.database.engine import Database

DEFAULT_TEST_URL = "postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable"


def base_url() -> str:
    return os.environ.get("OPENFORMS_TEST_DATABASE_URL") or DEFAULT_TEST_URL


def _plain_dsn(url: str) -> str:
    parts = urlsplit(url)
    q = {k: v for k, v in parse_qsl(parts.query) if k != "search_path"}
    return urlunsplit(("postgresql", parts.netloc, parts.path, urlencode(q), ""))


def with_search_path(url: str, schema: str) -> str:
    parts = urlsplit(url)
    q = dict(parse_qsl(parts.query))
    q["search_path"] = schema
    return urlunsplit((parts.scheme, parts.netloc, parts.path, urlencode(q), ""))


async def new_schema_url() -> tuple[str, str]:
    schema = "t_" + secrets.token_hex(8)
    try:
        conn = await asyncpg.connect(_plain_dsn(base_url()))
    except OSError as e:
        pytest.fail(f"connect to test database: {e} (start it with `docker compose up -d postgres`)")
    try:
        await conn.execute(f"CREATE SCHEMA {schema}")
    finally:
        await conn.close()
    return with_search_path(base_url(), schema), schema


async def drop_schema(schema: str) -> None:
    conn = await asyncpg.connect(_plain_dsn(base_url()))
    try:
        await conn.execute(f"DROP SCHEMA {schema} CASCADE")
    finally:
        await conn.close()


@pytest.fixture
async def db_url() -> AsyncIterator[str]:
    """An empty schema ``t_<hex>`` and a URL whose connections use it."""
    url, schema = await new_schema_url()
    yield url
    await drop_schema(schema)


async def migrated_database(url: str) -> Database:
    db = Database(url, pool_size=5, max_overflow=5)
    await migrate(db.engine)
    return db


@pytest.fixture
async def database(db_url: str) -> AsyncIterator[Database]:
    """A Database bound to a fresh, fully migrated schema."""
    db = await migrated_database(db_url)
    yield db
    await db.dispose()


@pytest.fixture(autouse=True)
def fast_bcrypt(monkeypatch):
    """Keep tests fast; production uses cost 12."""
    from openforms.server.models import auth

    monkeypatch.setattr(auth, "BCRYPT_ROUNDS", 4)
    monkeypatch.setattr(auth, "_dummy_hash", None)


@pytest.fixture
async def org_id(database):
    from openforms.server.models import auth

    async with database.transaction() as s:
        return await auth.ensure_default_org(s)

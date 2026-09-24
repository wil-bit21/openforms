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


class Env:
    """Port of Go's ``testutil.Env``: an isolated schema, the default org, an app and a client."""

    def __init__(self, ctx, app, client):
        self.ctx, self.app, self.client = ctx, app, client
        self.db, self.org_id, self.settings = ctx.db, ctx.org_id, ctx.settings
        self._seq = 0

    async def api_key(self, *roles: str) -> str:
        from openforms.server.models import auth

        self._seq += 1
        async with self.db.transaction() as s:
            plaintext, _ = await auth.create_api_key(s, self.org_id, f"test-key-{self._seq}", list(roles))
        return plaintext

    async def user(self, email: str, *roles: str):
        from openforms.server.models import auth

        async with self.db.transaction() as s:
            return await auth.create_user(s, self.org_id, email, email, "password123", list(roles))

    async def do(self, method: str, path: str, api_key: str = "", body=None, **kw):
        headers = kw.pop("headers", {})
        if api_key:
            headers["Authorization"] = "Bearer " + api_key
        if body is None:
            return await self.client.request(method, path, headers=headers, **kw)
        if isinstance(body, (str, bytes)):
            headers.setdefault("Content-Type", "application/json")
            return await self.client.request(method, path, headers=headers, content=body, **kw)
        return await self.client.request(method, path, headers=headers, json=body, **kw)


def test_settings(**kw):
    from openforms.settings import Settings

    base = dict(
        http_addr="127.0.0.1:0",
        base_url="http://test.local",
        smtp_port=1025,
        smtp_from="openforms@test.local",
        worker_concurrency=1,
    )
    base.update(kw)
    return Settings(**base)


test_settings.__test__ = False  # not a test


def error_code(resp) -> str:
    try:
        return resp.json().get("error", {}).get("code", "")
    except ValueError:
        return ""


@pytest.fixture
async def env(database, org_id, tmp_path):
    import httpx

    from openforms.server.api.context import AppContext
    from openforms.server.api.server import build_app

    ctx = AppContext(settings=test_settings(), db=database, org_id=org_id, ui_dir=tmp_path / "ui")
    app = build_app(ctx)
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test.local") as client:
        yield Env(ctx, app, client)


@pytest.fixture
async def api(env):
    """Port of Go's ``newDefsSubsAPI``: an env plus an admin key and helpers to seed and submit."""
    env.admin = await env.api_key("admin")

    async def seed():
        from openforms.server.models import definitions as defs
        from tests.samples import sample_form, sample_workflow

        internal = sample_form("internal", "review")
        internal.settings.public = False
        async with env.db.transaction() as s:
            await defs.apply(
                s,
                env.org_id,
                defs.ApplyInput(
                    workflows=[sample_workflow("review")],
                    forms=[sample_form("contact", "review"), sample_form("plain"), internal],
                ),
            )

    async def create(slug: str = "contact", data=None):
        from openforms.server.models import submissions as subs

        async with env.db.transaction() as s:
            return await subs.create(
                s,
                env.org_id,
                slug,
                data or valid_data(),
                require_public=False,
                on_created=env.ctx.services.get("on_created"),
            )

    env.seed, env.create = seed, create
    return env


def valid_data():
    return {"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}

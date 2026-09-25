"""Async engine construction from libpq-style ``postgres://`` URLs.

``OPENFORMS_DATABASE_URL`` keeps the format the Go server accepted
(``postgres://user:pass@host:port/db?sslmode=disable&search_path=x``); this
module translates it for SQLAlchemy + asyncpg.
"""

from __future__ import annotations

import ssl as _ssl
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any
from urllib.parse import parse_qsl, urlencode, urlsplit, urlunsplit

from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker, create_async_engine


def asyncpg_url(url: str) -> tuple[str, dict[str, Any]]:
    """Return a ``postgresql+asyncpg://`` URL and the asyncpg ``connect_args``."""
    parts = urlsplit(url)
    scheme = parts.scheme
    if scheme in ("postgres", "postgresql", "postgresql+asyncpg"):
        scheme = "postgresql+asyncpg"
    else:
        raise ValueError(f"unsupported database URL scheme {parts.scheme!r} (use postgres://)")
    query = dict(parse_qsl(parts.query, keep_blank_values=True))
    connect_args: dict[str, Any] = {}
    server_settings: dict[str, str] = {}
    sslmode = query.pop("sslmode", None)
    if sslmode == "disable":
        connect_args["ssl"] = False
    elif sslmode in ("require", "prefer", "allow"):
        ctx = _ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = _ssl.CERT_NONE
        connect_args["ssl"] = ctx if sslmode == "require" else sslmode
    elif sslmode in ("verify-ca", "verify-full"):
        connect_args["ssl"] = sslmode
    if "search_path" in query:
        server_settings["search_path"] = query.pop("search_path")
    if "application_name" in query:
        server_settings["application_name"] = query.pop("application_name")
    for k in ("connect_timeout", "pool_max_conns"):
        query.pop(k, None)
    if server_settings:
        connect_args["server_settings"] = server_settings
    return urlunsplit((scheme, parts.netloc, parts.path, urlencode(query), "")), connect_args


def create_engine(url: str, **kw: Any) -> AsyncEngine:
    sa_url, connect_args = asyncpg_url(url)
    kw.setdefault("pool_pre_ping", True)
    kw.setdefault("pool_size", 10)
    kw.setdefault("max_overflow", 10)
    return create_async_engine(sa_url, connect_args=connect_args, **kw)


class Database:
    """One engine + session factory per process (or per test)."""

    def __init__(self, url: str, **engine_kw: Any) -> None:
        self.url = url
        self.engine = create_engine(url, **engine_kw)
        self.sessionmaker = async_sessionmaker(self.engine, expire_on_commit=False, autoflush=False)

    @asynccontextmanager
    async def session(self) -> AsyncIterator[AsyncSession]:
        async with self.sessionmaker() as s:
            yield s

    @asynccontextmanager
    async def transaction(self) -> AsyncIterator[AsyncSession]:
        """A session inside a transaction: commits on success, rolls back on error."""
        async with self.sessionmaker() as s, s.begin():
            yield s

    async def ping(self) -> None:
        from sqlalchemy import text

        async with self.engine.connect() as c:
            await c.execute(text("SELECT 1"))

    async def dispose(self) -> None:
        await self.engine.dispose()

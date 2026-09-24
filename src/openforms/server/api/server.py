"""The ASGI application: ``/healthz``, the ``/api/v1`` sub-app and the web UI fallback.

``build_app`` wires an existing :class:`AppContext` (tests); ``create_app`` also
owns the lifecycle — database, migrations, default org, demo seed and the job
worker — as Prefect's server does in its lifespan.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator, Callable
from contextlib import asynccontextmanager
from typing import Any

from fastapi import Depends, FastAPI
from starlette.routing import Route

from ..._paths import ui_dir
from ...settings import Settings
from ..database.alembic_commands import migrate
from ..database.engine import Database
from ..models import auth as auth_models
from . import api_keys, auth, users
from .context import AppContext
from .dependencies import authenticate
from .errors import install_error_handlers
from .root import healthz
from .ui import serve_ui

log = logging.getLogger("openforms")

# Routers added by later layers register here: (router, needs_auth).
API_ROUTERS: list[Callable[[FastAPI], None]] = []


def build_api(ctx: AppContext) -> FastAPI:
    api = FastAPI(
        title="openforms API",
        version="v1",
        dependencies=[Depends(authenticate)],
        docs_url="/docs",
        openapi_url="/openapi.json",
        redoc_url=None,
    )
    api.state.ctx = ctx
    install_error_handlers(api)
    api.include_router(auth.router)
    api.include_router(users.router)
    api.include_router(api_keys.router)
    for register in API_ROUTERS:
        register(api)
    return api


def build_app(ctx: AppContext, lifespan: Any = None) -> FastAPI:
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
    app.state.ctx = ctx
    install_error_handlers(app)
    app.router.routes.append(Route("/healthz", healthz, methods=["GET", "HEAD"]))
    app.mount("/api/v1", build_api(ctx))
    app.router.routes.append(
        Route("/{path:path}", serve_ui, methods=["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"])
    )
    return app


async def open_context(settings: Settings) -> AppContext:
    """Open the pool, migrate and ensure the default org (Go ``app.New``)."""
    settings.require_database()
    db = Database(settings.database_url)
    try:
        await db.ping()
        await migrate(db.engine)
        async with db.transaction() as s:
            org_id = await auth_models.ensure_default_org(s)
    except BaseException:
        await db.dispose()
        raise
    return AppContext(settings=settings, db=db, org_id=org_id, ui_dir=ui_dir())


# Hooks run inside the lifespan after the context is open (seed, worker…).
STARTUP_HOOKS: list[Callable[[AppContext], Any]] = []


def create_app(settings: Settings, *, run_services: bool = True) -> FastAPI:
    holder: dict[str, AppContext] = {}

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        ctx = await open_context(settings)
        app.state.ctx = ctx
        holder["ctx"] = ctx
        for route in app.routes:
            sub = getattr(route, "app", None)
            if isinstance(sub, FastAPI):
                sub.state.ctx = ctx
        stops = []
        try:
            for hook in STARTUP_HOOKS:
                stop = await hook(ctx) if run_services else None
                if stop is not None:
                    stops.append(stop)
            log.info("openforms listening (baseURL=%s)", settings.base_url)
            yield
        finally:
            for stop in reversed(stops):
                await stop()
            await ctx.db.dispose()

    placeholder = AppContext(settings=settings, db=None, org_id=None, ui_dir=ui_dir())  # type: ignore[arg-type]
    return build_app(placeholder, lifespan=lifespan)

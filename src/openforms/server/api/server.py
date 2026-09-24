"""The ASGI application: ``/healthz``, the ``/api/v1`` sub-app and the web UI fallback.

``build_app`` wires an existing :class:`AppContext` (tests); ``create_app`` also
owns the lifecycle — database, migrations, default org, demo seed and the job
worker — as Prefect's server does in its lifespan.
"""

from __future__ import annotations

import logging
from collections.abc import AsyncIterator
from contextlib import asynccontextmanager
from typing import Any

from fastapi import Depends, FastAPI
from starlette.routing import Route

from ..._paths import ui_dir
from ...settings import Settings
from ..database.alembic_commands import migrate
from ..database.engine import Database
from ..models import auth as auth_models
from . import api_keys, auth, definitions, jobs, public, submissions, users, workflow
from .context import AppContext
from .dependencies import authenticate
from .errors import install_error_handlers
from .root import healthz
from .ui import serve_ui

log = logging.getLogger("openforms")


def build_api(ctx: AppContext, with_workflow: bool) -> FastAPI:
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
    api.include_router(public.router)
    api.include_router(definitions.router)
    api.include_router(submissions.router)
    if with_workflow:
        api.include_router(workflow.router)
        api.include_router(jobs.router)
    return api


def build_app(ctx: AppContext, lifespan: Any = None, with_workflow: bool | None = None) -> FastAPI:
    """Workflow and jobs routes are mounted when the engine is wired (as the Go router did)."""
    if with_workflow is None:
        with_workflow = "engine" in ctx.services
    app = FastAPI(docs_url=None, redoc_url=None, openapi_url=None, lifespan=lifespan)
    app.state.ctx = ctx
    install_error_handlers(app)
    app.router.routes.append(Route("/healthz", healthz, methods=["GET", "HEAD"]))
    app.mount("/api/v1", build_api(ctx, with_workflow))
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


def create_app(settings: Settings, *, run_worker: bool = True) -> FastAPI:
    """The production app: its lifespan opens the database, migrates, ensures the
    default org, wires the workflow engine, seeds the demo when asked and runs the
    job worker until shutdown."""
    from ..wiring import start_worker, wire_workflow

    ctx = AppContext(settings=settings, db=None, org_id=None, ui_dir=ui_dir())  # type: ignore[arg-type]

    @asynccontextmanager
    async def lifespan(app: FastAPI) -> AsyncIterator[None]:
        opened = await open_context(settings)
        ctx.db, ctx.org_id = opened.db, opened.org_id
        stop_worker = None
        try:
            wire_workflow(ctx)
            if settings.seed_demo:
                from ..models.seed import seed_demo

                res = await seed_demo(ctx.db, ctx.org_id)
                log.info("demo bundle seeded (items=%d, usersCreated=%d)", len(res.apply.items), len(res.users_created))
            if run_worker:
                stop_worker = start_worker(ctx)
            log.info("openforms listening (baseURL=%s)", settings.base_url)
            yield
        finally:
            if stop_worker is not None:
                await stop_worker()
            await ctx.db.dispose()

    return build_app(ctx, lifespan=lifespan, with_workflow=True)

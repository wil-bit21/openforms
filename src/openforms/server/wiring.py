"""Wires the job queue, workflow engine and action handlers into an AppContext and
runs the background worker (port of the Go ``app/workers.go``)."""

from __future__ import annotations

import asyncio
import logging
from collections.abc import Awaitable, Callable
from typing import Any

import httpx

from .actions import ActionDeps, register
from .actions.mailer import Mailer
from .api.context import AppContext
from .services.jobs import Queue
from .workflow import Engine

log = logging.getLogger("openforms")


def wire_workflow(
    ctx: AppContext, *, mailer: Mailer | None = None, http_client: httpx.AsyncClient | None = None
) -> None:
    """Build the queue and engine, hook onSubmit actions into submission creation and
    register the action handlers."""
    queue = Queue(ctx.db)
    engine = Engine(ctx.db, queue)
    register(queue, ActionDeps(settings=ctx.settings, db=ctx.db, mailer=mailer, http_client=http_client))

    async def available(session: Any, principal: Any, sub: Any) -> list[dict[str, Any]]:
        return [a.to_dict() for a in await engine.available(session, principal, sub)]

    ctx.services.update(queue=queue, engine=engine, on_created=engine.on_created, available_transitions=available)


def start_worker(ctx: AppContext) -> Callable[[], Awaitable[None]]:
    """Start the job worker; the returned coroutine function stops it and waits."""
    queue: Queue = ctx.services["queue"]
    stop = asyncio.Event()
    task = asyncio.create_task(queue.run(stop, ctx.settings.worker_concurrency), name="openforms-worker")

    async def shutdown() -> None:
        stop.set()
        try:
            await asyncio.wait_for(task, timeout=15)
        except TimeoutError:
            task.cancel()
            log.warning("job worker did not stop within 15s; cancelled")

    return shutdown

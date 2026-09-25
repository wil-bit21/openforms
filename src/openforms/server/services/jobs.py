"""Postgres-backed job queue (port of the Go ``internal/jobs``).

Jobs are enqueued inside the caller's transaction (transactional outbox) and
claimed with ``FOR UPDATE SKIP LOCKED``, so any number of workers can run
concurrently. A job left ``running`` by a crashed worker is reclaimed once
``locked_until`` passes.
"""

from __future__ import annotations

import asyncio
import datetime as dt
import json
import logging
import uuid
from collections.abc import Awaitable, Callable
from dataclasses import dataclass
from typing import Any

import sqlalchemy as sa
from sqlalchemy.ext.asyncio import AsyncSession

from ..database.engine import Database
from ..exceptions import JobNotFound

log = logging.getLogger("openforms.jobs")

STATUS_PENDING, STATUS_RUNNING, STATUS_DONE, STATUS_FAILED = "pending", "running", "done", "failed"
LOCK_DURATION = dt.timedelta(minutes=5)
BASE_BACKOFF = dt.timedelta(seconds=5)
MAX_BACKOFF = dt.timedelta(hours=1)


@dataclass
class Job:
    id: int
    org_id: uuid.UUID
    kind: str
    payload: Any
    status: str
    attempts: int
    max_attempts: int
    run_at: dt.datetime
    last_error: str
    created_at: dt.datetime
    updated_at: dt.datetime


Handler = Callable[[Job], Awaitable[None]]
FailedHook = Callable[[Job, BaseException], Awaitable[None]]


class Permanent(Exception):
    """Marks an error as non-retryable: the job fails immediately."""

    def __init__(self, err: BaseException | str) -> None:
        self.err = err if isinstance(err, BaseException) else Exception(err)
        super().__init__(str(self.err))


def is_permanent(err: BaseException) -> bool:
    e: BaseException | None = err
    while e is not None:
        if isinstance(e, Permanent):
            return True
        e = e.__cause__
    return False


def backoff(attempts: int) -> dt.timedelta:
    """Delay before retry number ``attempts + 1``: ``min(5s * 2^(attempts-1), 1h)``."""
    d = BASE_BACKOFF
    for _ in range(1, max(attempts, 1)):
        d *= 2
        if d >= MAX_BACKOFF:
            return MAX_BACKOFF
    return d


_COLS = "id, org_id, kind, payload, status, attempts, max_attempts, run_at, last_error, created_at, updated_at"


def _job(r: Any) -> Job:
    return Job(
        id=r.id,
        org_id=r.org_id,
        kind=r.kind,
        payload=r.payload,
        status=r.status,
        attempts=r.attempts,
        max_attempts=r.max_attempts,
        run_at=r.run_at,
        last_error=r.last_error,
        created_at=r.created_at,
        updated_at=r.updated_at,
    )


def _utcnow() -> dt.datetime:
    return dt.datetime.now(dt.UTC)


class Queue:
    def __init__(self, db: Database) -> None:
        self.db = db
        self.handlers: dict[str, Handler] = {}
        self.on_failed: FailedHook | None = None
        self.now: Callable[[], dt.datetime] = _utcnow
        self.poll_interval = 1.0

    def register(self, kind: str, handler: Handler) -> None:
        self.handlers[kind] = handler

    def set_failed_hook(self, hook: FailedHook | None) -> None:
        self.on_failed = hook

    async def enqueue(self, session: AsyncSession, org_id: uuid.UUID, kind: str, payload: Any) -> int:
        """Insert a pending job in the caller's transaction (it commits or rolls back with it)."""
        now = self.now()
        return (
            await session.execute(
                sa.text("""INSERT INTO jobs (org_id, kind, payload, run_at, created_at, updated_at)
                VALUES (:org, :kind, CAST(:payload AS jsonb), :now, :now, :now) RETURNING id"""),
                {"org": org_id, "kind": kind, "payload": json.dumps(payload), "now": now},
            )
        ).scalar_one()

    async def run_once(self) -> bool:
        """Claim at most one due job, run its handler and record the outcome.
        Handler errors are recorded on the job, not raised."""
        now = self.now()
        async with self.db.transaction() as s:
            r = (
                await s.execute(
                    sa.text(f"""
                UPDATE jobs SET status = 'running', attempts = attempts + 1, locked_until = :lock, updated_at = :now
                WHERE id = (
                  SELECT id FROM jobs
                  WHERE (status = 'pending' AND run_at <= :now)
                     OR (status = 'running' AND locked_until < :now)
                  ORDER BY run_at, id
                  LIMIT 1
                  FOR UPDATE SKIP LOCKED
                )
                RETURNING {_COLS}"""),
                    {"now": now, "lock": now + LOCK_DURATION},
                )
            ).first()
        if r is None:
            return False
        job = _job(r)
        await self._finish(job, await self._execute(job))
        return True

    async def _execute(self, job: Job) -> BaseException | None:
        handler = self.handlers.get(job.kind)
        if handler is None:
            return Permanent(f'no handler registered for kind "{job.kind}"')
        try:
            await handler(job)
        except asyncio.CancelledError:
            raise
        except Exception as e:  # handler failures are recorded, never propagated
            return e
        return None

    async def _finish(self, job: Job, err: BaseException | None) -> None:
        now = self.now()
        async with self.db.transaction() as s:
            if err is None:
                await s.execute(
                    sa.text(
                        "UPDATE jobs SET status = 'done', locked_until = NULL, last_error = '', updated_at = :now WHERE id = :id"
                    ),
                    {"id": job.id, "now": now},
                )
                return
            msg = str(err) or type(err).__name__
            if is_permanent(err) or job.attempts >= job.max_attempts:
                await s.execute(
                    sa.text(
                        "UPDATE jobs SET status = 'failed', locked_until = NULL, last_error = :err, updated_at = :now WHERE id = :id"
                    ),
                    {"id": job.id, "err": msg, "now": now},
                )
            else:
                await s.execute(
                    sa.text(
                        """UPDATE jobs SET status = 'pending', locked_until = NULL, last_error = :err, run_at = :run_at,
                       updated_at = :now WHERE id = :id"""
                    ),
                    {"id": job.id, "err": msg, "run_at": now + backoff(job.attempts), "now": now},
                )
                return
        job.status, job.last_error = STATUS_FAILED, msg
        if self.on_failed is not None:
            try:
                await self.on_failed(job, err)
            except Exception:
                log.exception("jobs: failed hook for job %s", job.id)

    async def list(self, org_id: uuid.UUID, status: str = "", limit: int = 100) -> list[Job]:
        """The org's jobs, newest first (``status`` "" = all)."""
        async with self.db.session() as s:
            rows = await s.execute(
                sa.text(f"""SELECT {_COLS} FROM jobs
                WHERE org_id = :org AND (CAST(:status AS text) = '' OR status = :status)
                ORDER BY id DESC LIMIT :lim"""),
                {"org": org_id, "status": status, "lim": limit if limit > 0 else 100},
            )
            return [_job(r) for r in rows]

    async def retry(self, org_id: uuid.UUID, job_id: int) -> None:
        """Reset a failed job so it runs again immediately."""
        now = self.now()
        async with self.db.transaction() as s:
            res = await s.execute(
                sa.text("""UPDATE jobs SET status = 'pending', attempts = 0, run_at = :now,
                locked_until = NULL, last_error = '', updated_at = :now
                WHERE org_id = :org AND id = :id AND status = 'failed'"""),
                {"org": org_id, "id": job_id, "now": now},
            )
            if res.rowcount == 0:  # type: ignore[attr-defined]
                raise JobNotFound()

    async def run(self, stop: asyncio.Event, concurrency: int = 1) -> None:
        """Process jobs with ``concurrency`` workers until ``stop`` is set."""

        async def worker() -> None:
            while not stop.is_set():
                processed = False
                try:
                    processed = await self.run_once()
                except asyncio.CancelledError:
                    raise
                except Exception:
                    if not stop.is_set():
                        log.exception("jobs: worker")
                if processed:
                    continue
                try:
                    await asyncio.wait_for(stop.wait(), timeout=self.poll_interval)
                except TimeoutError:
                    pass

        await asyncio.gather(*(worker() for _ in range(max(concurrency, 1))))

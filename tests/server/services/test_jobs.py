import asyncio
import datetime as dt
import uuid

import pytest
import sqlalchemy as sa

from openforms.server.exceptions import JobNotFound
from openforms.server.services.jobs import (
    STATUS_DONE,
    STATUS_FAILED,
    STATUS_PENDING,
    Permanent,
    Queue,
    backoff,
    is_permanent,
)


class Clock:
    def __init__(self):
        self.t = dt.datetime(2030, 1, 1, 12, 0, tzinfo=dt.UTC)

    def __call__(self):
        return self.t

    def advance(self, d: dt.timedelta):
        self.t += d


class H:
    def __init__(self, database):
        self.db, self.q, self.clock, self.org = database, Queue(database), Clock(), uuid.uuid4()
        self.q.now = self.clock

    async def enqueue(self, kind, payload):
        async with self.db.transaction() as s:
            return await self.q.enqueue(s, self.org, kind, payload)

    async def job(self, job_id):
        return next(j for j in await self.q.list(self.org, "", 1000) if j.id == job_id)


@pytest.fixture
def h(database):
    return H(database)


async def test_run_once_no_jobs(h):
    assert await h.q.run_once() is False


async def test_enqueue_and_run_once_success(h):
    got = {}

    async def ok(job):
        got.update(job.payload)

    h.q.register("test.ok", ok)
    jid = await h.enqueue("test.ok", {"n": 1})
    assert await h.q.run_once() is True
    assert got == {"n": 1}
    j = await h.job(jid)
    assert (j.status, j.attempts, j.last_error) == (STATUS_DONE, 1, "")
    assert len(await h.q.list(h.org, STATUS_DONE, 10)) == 1
    assert await h.q.list(uuid.uuid4(), "", 10) == []


async def test_enqueue_rolls_back_with_the_caller(h):
    with pytest.raises(RuntimeError):
        async with h.db.transaction() as s:
            await h.q.enqueue(s, h.org, "test.ok", {})
            raise RuntimeError("abort")
    assert await h.q.list(h.org) == []


async def test_retry_with_backoff(h):
    async def flaky(job):
        raise RuntimeError("boom")

    h.q.register("test.flaky", flaky)
    jid = await h.enqueue("test.flaky", {})
    assert await h.q.run_once()
    j = await h.job(jid)
    assert (j.status, j.attempts, j.last_error) == (STATUS_PENDING, 1, "boom")
    assert j.run_at == h.clock() + dt.timedelta(seconds=5)
    assert not await h.q.run_once()
    h.clock.advance(dt.timedelta(seconds=5))
    assert await h.q.run_once()
    j = await h.job(jid)
    assert j.attempts == 2 and j.run_at == h.clock() + dt.timedelta(seconds=10)


async def test_fails_after_max_attempts(h):
    calls = []

    async def hook(job, err):
        calls.append(job)

    async def down(job):
        raise RuntimeError("down")

    h.q.set_failed_hook(hook)
    h.q.register("test.flaky", down)
    jid = await h.enqueue("test.flaky", {})
    async with h.db.transaction() as s:
        await s.execute(sa.text("UPDATE jobs SET max_attempts = 2 WHERE id = :id"), {"id": jid})
    await h.q.run_once()
    h.clock.advance(dt.timedelta(hours=1))
    await h.q.run_once()
    j = await h.job(jid)
    assert (j.status, j.attempts, j.last_error) == (STATUS_FAILED, 2, "down")
    assert len(calls) == 1 and calls[0].id == jid and calls[0].status == STATUS_FAILED
    h.clock.advance(dt.timedelta(hours=1))
    assert not await h.q.run_once()


async def test_permanent_fails_immediately(h):
    async def perm(job):
        raise Permanent(ValueError("bad input"))

    h.q.register("test.perm", perm)
    jid = await h.enqueue("test.perm", {})
    await h.q.run_once()
    j = await h.job(jid)
    assert (j.status, j.attempts, j.last_error) == (STATUS_FAILED, 1, "bad input")


async def test_unknown_kind_fails(h):
    jid = await h.enqueue("test.nobody", {})
    await h.q.run_once()
    j = await h.job(jid)
    assert j.status == STATUS_FAILED and "no handler" in j.last_error


async def test_unexpected_exception_is_recorded(h):
    async def boom(job):
        raise KeyError("kaboom")

    h.q.register("test.panic", boom)
    jid = await h.enqueue("test.panic", {})
    await h.q.run_once()
    j = await h.job(jid)
    assert j.status == STATUS_PENDING and "kaboom" in j.last_error


def test_permanent():
    base = ValueError("x")
    p = Permanent(base)
    assert is_permanent(p) and p.err is base and str(p) == "x"
    assert not is_permanent(base)
    try:
        raise RuntimeError("wrapped") from p
    except RuntimeError as e:
        assert is_permanent(e)


@pytest.mark.parametrize(
    "attempts,secs", [(0, 5), (1, 5), (2, 10), (3, 20), (9, 1280), (10, 2560), (11, 3600), (50, 3600)]
)
def test_backoff(attempts, secs):
    assert backoff(attempts) == dt.timedelta(seconds=secs)


async def test_two_workers_process_each_job_exactly_once(database):
    org = uuid.uuid4()
    counts: dict[int, int] = {}

    async def handler(job):
        await asyncio.sleep(0.002)
        counts[job.id] = counts.get(job.id, 0) + 1

    q1, q2 = Queue(database), Queue(database)
    for q in (q1, q2):
        q.register("test.count", handler)
        q.poll_interval = 0.01
    n = 30
    async with database.transaction() as s:
        for i in range(n):
            await q1.enqueue(s, org, "test.count", {"i": i})
    stop = asyncio.Event()
    runners = [asyncio.create_task(q.run(stop, 2)) for q in (q1, q2)]
    for _ in range(500):
        if len(await q1.list(org, STATUS_DONE, 1000)) == n:
            break
        await asyncio.sleep(0.02)
    stop.set()
    await asyncio.wait_for(asyncio.gather(*runners), 5)
    assert len(counts) == n and all(c == 1 for c in counts.values())


async def test_run_returns_when_stopped(database):
    stop = asyncio.Event()
    task = asyncio.create_task(Queue(database).run(stop, 0))
    await asyncio.sleep(0.05)
    stop.set()
    await asyncio.wait_for(task, 3)


async def test_reclaims_expired_lock(h):
    ran = []

    async def ok(job):
        ran.append(job.id)

    h.q.register("test.ok", ok)
    jid = await h.enqueue("test.ok", {})
    async with h.db.transaction() as s:
        await s.execute(
            sa.text("UPDATE jobs SET status = 'running', attempts = 1, locked_until = :t WHERE id = :id"),
            {"id": jid, "t": h.clock() + dt.timedelta(minutes=5)},
        )
    assert not await h.q.run_once()
    h.clock.advance(dt.timedelta(minutes=5, seconds=1))
    assert await h.q.run_once()
    j = await h.job(jid)
    assert ran == [jid] and j.status == STATUS_DONE and j.attempts == 2


async def test_retry(h):
    state = {"fail": True}

    async def toggle(job):
        if state["fail"]:
            raise Permanent("nope")

    h.q.register("test.toggle", toggle)
    jid = await h.enqueue("test.toggle", {})
    await h.q.run_once()
    assert (await h.job(jid)).status == STATUS_FAILED
    with pytest.raises(JobNotFound):
        await h.q.retry(uuid.uuid4(), jid)
    await h.q.retry(h.org, jid)
    j = await h.job(jid)
    assert (j.status, j.attempts, j.last_error) == (STATUS_PENDING, 0, "")
    with pytest.raises(JobNotFound):
        await h.q.retry(h.org, jid)
    state["fail"] = False
    await h.q.run_once()
    assert (await h.job(jid)).status == STATUS_DONE

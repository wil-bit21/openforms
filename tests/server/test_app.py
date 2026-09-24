"""Port of ``app/app_test.go`` and ``app/workers_test.go``."""

import asyncio

import httpx
import pytest

from openforms.server.api.context import AppContext
from openforms.server.api.server import create_app
from openforms.server.services.jobs import STATUS_DONE
from openforms.server.wiring import start_worker, wire_workflow
from openforms.settings import ConfigError, Settings
from tests.conftest import test_settings


async def test_worker_processes_on_submit_actions(fx, tmp_path):
    ctx = AppContext(settings=test_settings(worker_concurrency=2), db=fx.db, org_id=fx.org_id, ui_dir=tmp_path)
    wire_workflow(ctx)
    queue = ctx.services["queue"]
    queue.poll_interval = 0.02
    stop = start_worker(ctx)
    try:
        sub = await fx.applicant(on_created=ctx.services["on_created"])
        for _ in range(500):
            if len(await queue.list(fx.org_id, STATUS_DONE, 10)) == 1:
                break
            await asyncio.sleep(0.02)
        else:
            pytest.fail(f"job not processed: {await queue.list(fx.org_id, '', 10)}")
    finally:
        await stop()
    assert await fx.events_of_type(sub.id, "action_succeeded")


class Lifespan:
    """Runs an app's lifespan the way uvicorn does."""

    def __init__(self, app):
        self.app = app

    async def __aenter__(self):
        self._cm = self.app.router.lifespan_context(self.app)
        await self._cm.__aenter__()
        return httpx.AsyncClient(transport=httpx.ASGITransport(app=self.app), base_url="http://test.local")

    async def __aexit__(self, *exc):
        await self._cm.__aexit__(*exc)


async def test_create_app_serves_healthz_and_api(db_url):
    app = create_app(test_settings(database_url=db_url))
    async with Lifespan(app) as client, client:
        assert app.state.ctx.org_id is not None
        r = await client.get("/healthz")
        assert r.status_code == 200 and "ok" in r.text
        r = await client.get("/api/v1/does-not-exist")
        assert r.status_code == 404 and "not_found" in r.text
        r = await client.get("/admin")
        if r.status_code == 200:
            assert '<div id="root">' in r.text
        else:
            assert r.status_code == 503 and "web UI not built" in r.text
        # workflow routes are mounted: an unauthenticated jobs call is 401, not 404
        assert (await client.get("/api/v1/jobs")).status_code == 401


async def test_create_app_is_restart_safe(db_url):
    settings = test_settings(database_url=db_url)
    orgs = []
    for _ in range(2):
        app = create_app(settings)
        async with Lifespan(app) as client, client:
            orgs.append(app.state.ctx.org_id)
    assert orgs[0] == orgs[1]


async def test_create_app_requires_database_url():
    app = create_app(Settings(database_url=""))
    with pytest.raises(ConfigError, match="OPENFORMS_DATABASE_URL"):
        async with Lifespan(app):
            pass


async def test_create_app_seeds_demo_when_asked(db_url):
    from openforms.server.models import auth

    app = create_app(test_settings(database_url=db_url, seed_demo=True, demo=True), run_worker=False)
    async with Lifespan(app) as client, client:
        ctx = app.state.ctx
        async with ctx.db.session() as s:
            assert len(await auth.list_users(s, ctx.org_id)) == 3
        assert (await client.get("/api/v1/public/forms/job-application")).status_code == 200
        assert (await client.get("/api/v1/public/config")).json() == {"demo": True}

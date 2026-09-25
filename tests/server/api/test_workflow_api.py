"""Port of ``httpapi/workflow_handlers_test.go``."""

import httpx
import pytest
import sqlalchemy as sa

from openforms.server.api.context import AppContext
from openforms.server.api.server import build_app
from openforms.server.wiring import wire_workflow
from tests.conftest import Env, error_code


@pytest.fixture
async def wf(fx, tmp_path):
    ctx = AppContext(settings=fx.settings, db=fx.db, org_id=fx.org_id, ui_dir=tmp_path / "ui")
    wire_workflow(ctx)
    fx.queue = ctx.services["queue"]
    app = build_app(ctx)
    async with httpx.AsyncClient(transport=httpx.ASGITransport(app=app), base_url="http://test.local") as client:
        env = Env(ctx, app, client)
        env.fx = fx
        env.applicant = lambda: fx.applicant(on_created=ctx.services["on_created"])
        yield env


async def test_transition_success(wf):
    sub = await wf.applicant()
    key = await wf.api_key("reviewer")
    r = await wf.do(
        "POST",
        f"/api/v1/submissions/{sub.id}/transitions",
        key,
        {"transition": "screen", "comment": "ok", "expectedState": "new"},
    )
    assert r.status_code == 200, r.text
    got = r.json()["submission"]
    assert got["state"] == "screening" and got["id"] == str(sub.id)


async def test_transition_errors(wf):
    sub = await wf.applicant()
    plain = await wf.fx.submit("feedback", {"message": "hi"})
    reviewer = await wf.api_key("reviewer")
    manager = await wf.api_key("hiring-manager")
    path = f"/api/v1/submissions/{sub.id}/transitions"
    cases = [
        ("", path, {"transition": "screen"}, 401, "unauthenticated"),
        (reviewer, path, {}, 422, "validation_failed"),
        (reviewer, path, {"transition": "teleport"}, 422, "unknown_transition"),
        (manager, path, {"transition": "screen"}, 403, "forbidden"),
        (reviewer, path, {"transition": "invite", "fields": {"score": 1}}, 409, "invalid_state"),
        (reviewer, path, {"transition": "screen", "expectedState": "interview"}, 409, "state_conflict"),
        (reviewer, path, {"transition": "reject"}, 422, "validation_failed"),
        (reviewer, f"/api/v1/submissions/{plain.id}/transitions", {"transition": "screen"}, 409, "no_workflow"),
        (reviewer, "/api/v1/submissions/not-a-uuid/transitions", {"transition": "screen"}, 404, "not_found"),
    ]
    for key, p, body, status, code in cases:
        r = await wf.do("POST", p, key, body)
        assert (r.status_code, error_code(r)) == (status, code), (body, r.text)

    r = await wf.do("POST", path, reviewer, {"transition": "reject"})
    details = r.json()["error"]["details"]
    assert [d["path"] for d in details] == ["fields.rejectionReason"]


async def test_fields_comments_assignee(wf):
    sub = await wf.applicant()
    key = await wf.api_key("reviewer")
    base = f"/api/v1/submissions/{sub.id}"

    r = await wf.do("PATCH", base + "/fields", key, {"fields": {"score": 5}})
    assert r.status_code == 200 and r.json()["submission"]["fields"]["score"] == 5, r.text
    r = await wf.do("PATCH", base + "/fields", key, {"fields": {"score": "high"}})
    assert (r.status_code, error_code(r)) == (422, "validation_failed")

    r = await wf.do("POST", base + "/comments", key, {"body": "Nice"})
    assert r.status_code == 201, r.text
    ev = r.json()["event"]
    assert ev["type"] == "comment" and ev["payload"]["body"] == "Nice"
    assert (await wf.do("POST", base + "/comments", key, {"body": ""})).status_code == 422

    u = await wf.user("who@example.com", "reviewer")
    r = await wf.do("PUT", base + "/assignee", key, {"userId": str(u.id)})
    assert r.status_code == 200 and r.json()["submission"]["assignee"]["email"] == "who@example.com", r.text
    r = await wf.do("PUT", base + "/assignee", key, {"userId": None})
    assert r.status_code == 200 and r.json()["submission"].get("assignee") is None, r.text
    r = await wf.do("PUT", base + "/assignee", key, {"userId": "nope"})
    assert r.status_code == 400


async def test_submission_detail_includes_transitions(wf):
    sub = await wf.applicant()
    r = await wf.do("GET", f"/api/v1/submissions/{sub.id}", await wf.api_key("hiring-manager"))
    assert r.status_code == 200, r.text
    trs = {t["key"]: t for t in r.json()["transitions"]}
    assert len(trs) == 2
    screen, reject = trs["screen"], trs["reject"]
    assert not screen["allowed"] and screen["reason"] == "role" and screen["requireFields"] == []
    assert reject["allowed"] and reject["toLabel"] == "Rejected"


async def test_jobs_http(wf):
    await wf.applicant()  # onSubmit email → one pending job
    admin = await wf.api_key("admin")
    assert (await wf.do("GET", "/api/v1/jobs", await wf.api_key("reviewer"))).status_code == 403

    r = await wf.do("GET", "/api/v1/jobs?status=pending", admin)
    items = r.json()["items"]
    assert r.status_code == 200 and len(items) == 1, r.text
    assert items[0]["kind"] == "action.email" and items[0]["maxAttempts"] == 8
    assert items[0]["payload"]["trigger"] == "submit"
    assert (await wf.do("GET", "/api/v1/jobs?status=bogus", admin)).status_code == 400
    for bad in ("0", "501", "x"):
        assert (await wf.do("GET", f"/api/v1/jobs?limit={bad}", admin)).status_code == 400

    jid = items[0]["id"]
    async with wf.db.transaction() as s:
        await s.execute(sa.text("UPDATE jobs SET status = 'failed' WHERE id = :id"), {"id": jid})
    assert (await wf.do("POST", f"/api/v1/jobs/{jid}/retry", admin)).status_code == 204
    r = await wf.do("POST", f"/api/v1/jobs/{jid}/retry", admin)
    assert (r.status_code, error_code(r)) == (404, "not_found")
    assert (await wf.do("POST", "/api/v1/jobs/abc/retry", admin)).status_code == 404
    assert len(await wf.fx.queue.list(wf.org_id, "pending", 10)) == 1

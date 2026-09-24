import datetime as dt
import json
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest
import sqlalchemy as sa

from openforms.definition import Action
from openforms.server.actions import TRIGGER_SUBMIT, ActionDeps, Payload, enqueue, kind_for, register
from openforms.server.actions.webhook import sign
from openforms.server.services.jobs import STATUS_DONE, STATUS_FAILED, STATUS_PENDING
from tests.conftest import test_settings


class FakeMailer:
    def __init__(self):
        self.sent, self.err = [], None

    async def send(self, to, subject, body):
        if self.err:
            raise self.err
        self.sent.append((to, subject, body))


@pytest.fixture
def webhook_server():
    servers = []

    def start(status):
        received = []

        class H(BaseHTTPRequestHandler):
            def do_POST(self):
                body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
                received.append((dict(self.headers), body))
                self.send_response(status)
                self.end_headers()

            def log_message(self, *a):
                pass

        srv = ThreadingHTTPServer(("127.0.0.1", 0), H)
        threading.Thread(target=srv.serve_forever, daemon=True).start()
        servers.append(srv)
        return f"http://127.0.0.1:{srv.server_address[1]}", received

    yield start
    for s in servers:
        s.shutdown()


@pytest.fixture
def actions_setup(fx):
    def setup(**settings_kw):
        mailer = FakeMailer()
        register(fx.queue, ActionDeps(settings=test_settings(**settings_kw), db=fx.db, mailer=mailer))
        return fx, mailer

    return setup


async def enqueue_action(f, sub, trigger, action):
    async with f.db.transaction() as s:
        await enqueue(f.queue, s, f.org_id, Payload(submission_id=sub.id, trigger=trigger, event_id=0, action=action))
    return (await f.jobs())[-1]


async def run_one(f):
    assert await f.queue.run_once()


def test_kind_for():
    assert (kind_for("webhook"), kind_for("email"), kind_for("assign")) == (
        "action.webhook",
        "action.email",
        "action.assign",
    )
    with pytest.raises(ValueError):
        kind_for("sms")


def test_sign():
    got = sign("s3cret", b'{"a":1}')
    assert got.startswith("sha256=") and len(got) == 7 + 64
    assert sign("s3cret", b'{"a":1}') == got != sign("other", b'{"a":1}')


async def test_webhook_delivers_signed_transition_event(actions_setup, webhook_server):
    url, received = webhook_server(200)
    f, _ = actions_setup(webhook_secret="s3cret")
    sub = await f.applicant()
    job = await enqueue_action(f, sub, "invite", Action(type="webhook", url=url + "/hook"))
    await run_one(f)
    assert len(received) == 1
    headers, body = received[0]
    assert headers["Content-Type"] == "application/json"
    assert headers["X-OpenForms-Event"] == "submission.transitioned"
    assert headers["X-OpenForms-Delivery"] == str(job.id)
    assert headers["X-OpenForms-Signature"] == sign("s3cret", body)
    b = json.loads(body)
    assert b["event"] == "submission.transitioned" and b["submission"]["id"] == str(sub.id)
    assert b["submission"]["form"] == "apply" and b["submission"]["data"]["email"] == "ada@example.com"
    assert b["transition"]["key"] == "invite" and b["transition"]["to"] == "interview"
    assert b["form"] == {"slug": "apply", "title": "Apply"}
    assert (await f.job(job.id)).status == STATUS_DONE
    evs = await f.events_of_type(sub.id, "action_succeeded")
    assert len(evs) == 1 and evs[0].actor_type == "system"


async def test_webhook_submit_trigger_and_no_secret(actions_setup, webhook_server):
    url, received = webhook_server(204)
    f, _ = actions_setup()
    sub = await f.applicant()
    await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="webhook", url=url))
    await run_one(f)
    headers, body = received[0]
    assert headers["X-OpenForms-Event"] == "submission.created"
    assert "X-OpenForms-Signature" not in headers
    assert json.loads(body)["transition"] is None


@pytest.mark.parametrize("status,want", [(500, STATUS_PENDING), (429, STATUS_PENDING), (408, STATUS_PENDING)])
async def test_webhook_retryable(actions_setup, webhook_server, status, want):
    url, _ = webhook_server(status)
    f, _ = actions_setup()
    sub = await f.applicant()
    job = await enqueue_action(f, sub, "invite", Action(type="webhook", url=url))
    await run_one(f)
    j = await f.job(job.id)
    assert j.status == want and j.attempts == 1 and str(status) in j.last_error
    assert not await f.events_of_type(sub.id, "action_succeeded") and not await f.events_of_type(
        sub.id, "action_failed"
    )


async def test_webhook_400_is_permanent(actions_setup, webhook_server):
    url, _ = webhook_server(400)
    f, _ = actions_setup()
    sub = await f.applicant()
    job = await enqueue_action(f, sub, "invite", Action(type="webhook", url=url))
    await run_one(f)
    j = await f.job(job.id)
    assert j.status == STATUS_FAILED and j.attempts == 1
    evs = await f.events_of_type(sub.id, "action_failed")
    assert len(evs) == 1 and "400" in evs[0].payload["error"] and evs[0].payload["action"]["type"] == "webhook"


async def test_webhook_unreachable_is_retryable(actions_setup):
    f, _ = actions_setup()
    job = await enqueue_action(f, await f.applicant(), "invite", Action(type="webhook", url="http://127.0.0.1:9/hook"))
    await run_one(f)
    assert (await f.job(job.id)).status == STATUS_PENDING


async def test_webhook_invalid_url_is_permanent(actions_setup):
    f, _ = actions_setup()
    job = await enqueue_action(f, await f.applicant(), "invite", Action(type="webhook", url="ftp://x"))
    await run_one(f)
    assert (await f.job(job.id)).status == STATUS_FAILED


async def test_enqueue_uses_caller_transaction(actions_setup):
    f, _ = actions_setup()
    sub = await f.applicant()
    with pytest.raises(RuntimeError):
        async with f.db.transaction() as s:
            await enqueue(f.queue, s, f.org_id, Payload(sub.id, "invite", 0, Action(type="webhook", url="http://x")))
            raise RuntimeError("rollback")
    assert await f.jobs() == []


async def test_email_renders_and_sends(actions_setup):
    f, m = actions_setup()
    sub = await f.applicant()
    job = await enqueue_action(
        f,
        sub,
        TRIGGER_SUBMIT,
        Action(
            type="email",
            to="{{submission.data.email}}",
            subject="Thanks {{submission.data.name}}",
            body="See {{submission.url}}",
        ),
    )
    await run_one(f)
    assert m.sent == [("ada@example.com", "Thanks Ada", f"See http://test.local/admin/submissions/{sub.id}")]
    assert (await f.job(job.id)).status == STATUS_DONE
    assert len(await f.events_of_type(sub.id, "action_succeeded")) == 1


async def test_email_empty_recipient_is_permanent(actions_setup):
    f, m = actions_setup()
    sub = await f.applicant()
    job = await enqueue_action(
        f, sub, TRIGGER_SUBMIT, Action(type="email", to="{{submission.data.missing}}", subject="s", body="b")
    )
    await run_one(f)
    j = await f.job(job.id)
    assert j.status == STATUS_FAILED and j.attempts == 1
    assert m.sent == [] and len(await f.events_of_type(sub.id, "action_failed")) == 1


async def test_email_invalid_recipient_is_permanent(actions_setup):
    f, _ = actions_setup()
    job = await enqueue_action(
        f, await f.applicant(), TRIGGER_SUBMIT, Action(type="email", to="not-an-email", subject="s", body="b")
    )
    await run_one(f)
    assert (await f.job(job.id)).status == STATUS_FAILED


async def test_email_mailer_error_retries(actions_setup):
    f, m = actions_setup()
    m.err = RuntimeError("smtp down")
    job = await enqueue_action(
        f, await f.applicant(), TRIGGER_SUBMIT, Action(type="email", to="a@example.com", subject="s", body="b")
    )
    await run_one(f)
    j = await f.job(job.id)
    assert j.status == STATUS_PENDING and j.attempts == 1


async def test_assign_by_role_picks_least_loaded(actions_setup):
    from openforms.server.models import submissions as subs

    f, _ = actions_setup()
    busy = await f.user("busy@example.com", "reviewer")
    idle = await f.user("idle@example.com", "reviewer")
    await f.user("other@example.com", "hiring-manager")
    open_ = await f.applicant()
    closed = await f.applicant()
    async with f.db.transaction() as s:
        await subs.set_assignee(s, f.org_id, open_.id, busy.id)
        await s.execute(
            sa.text("UPDATE submissions SET state = 'rejected', assignee_id = :u WHERE id = :id"),
            {"u": idle.id, "id": closed.id},
        )
    sub = await f.applicant()
    job = await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="assign", role="reviewer"))
    await run_one(f)
    assert (await f.reload(sub.id)).assignee_id == idle.id
    evs = await f.events_of_type(sub.id, "assigned")
    assert len(evs) == 1 and evs[0].payload["assigneeId"] == str(idle.id) and evs[0].actor_type == "system"
    assert (await f.job(job.id)).status == STATUS_DONE
    assert len(await f.events_of_type(sub.id, "action_succeeded")) == 1


async def test_assign_tie_goes_to_earliest_created(actions_setup):
    f, _ = actions_setup()
    later = await f.user("later@example.com", "reviewer")
    earlier = await f.user("earlier@example.com", "reviewer")
    base = dt.datetime(2030, 1, 1, tzinfo=dt.UTC)
    async with f.db.transaction() as s:
        await s.execute(
            sa.text("UPDATE users SET created_at = :t WHERE id = :id"),
            {"t": base + dt.timedelta(hours=1), "id": later.id},
        )
        await s.execute(sa.text("UPDATE users SET created_at = :t WHERE id = :id"), {"t": base, "id": earlier.id})
    sub = await f.applicant()
    await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="assign", role="reviewer"))
    await run_one(f)
    assert (await f.reload(sub.id)).assignee_id == earlier.id


async def test_assign_falls_back_to_admin(actions_setup):
    f, _ = actions_setup()
    admin = await f.user("admin@example.com", "admin")
    sub = await f.applicant()
    await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="assign", role="nobody-has-this"))
    await run_one(f)
    assert (await f.reload(sub.id)).assignee_id == admin.id


async def test_assign_no_candidates_is_permanent(actions_setup):
    f, _ = actions_setup()
    sub = await f.applicant()
    job = await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="assign", role="reviewer"))
    await run_one(f)
    assert (await f.job(job.id)).status == STATUS_FAILED
    assert len(await f.events_of_type(sub.id, "action_failed")) == 1


async def test_assign_by_user_email(actions_setup):
    f, _ = actions_setup()
    u = await f.user("named@example.com", "reviewer")
    sub = await f.applicant()
    await enqueue_action(f, sub, TRIGGER_SUBMIT, Action(type="assign", user="NAMED@example.com"))
    await run_one(f)
    assert (await f.reload(sub.id)).assignee_id == u.id
    sub2 = await f.applicant()
    job = await enqueue_action(f, sub2, TRIGGER_SUBMIT, Action(type="assign", user="ghost@example.com"))
    await run_one(f)
    assert (await f.job(job.id)).status == STATUS_FAILED

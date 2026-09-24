import csv
import datetime as dt
import io
import json
import uuid

import sqlalchemy as sa

from openforms.server.api.ratelimit import RateLimiter
from openforms.server.api.serialize import event_json, list_json, submission_json, version_json
from openforms.server.models.definitions import VersionInfo
from openforms.server.models.submissions import Event, Submission
from tests.conftest import error_code, valid_data
from tests.samples import sample_form

SUBMIT = "/api/v1/public/forms/{}/submissions"


async def test_public_get_form(api):
    await api.seed()
    r = await api.do("GET", "/api/v1/public/forms/contact")
    assert r.status_code == 200 and r.json()["form"]["slug"] == "contact" and len(r.json()["form"]["fields"]) == 3
    for slug in ("internal", "missing"):
        r = await api.do("GET", f"/api/v1/public/forms/{slug}")
        assert r.status_code == 404 and error_code(r) == "not_found"


async def test_public_submit_and_status(api):
    await api.seed()
    r = await api.do("POST", SUBMIT.format("contact"), body={"data": valid_data()})
    assert r.status_code == 201, r.text
    sub = r.json()
    assert (sub["state"], sub["stateLabel"], sub["confirmationMessage"]) == (
        "new",
        "New",
        "Thanks! Your response has been recorded.",
    )
    assert sub["receiptToken"]
    r = await api.do("GET", f"/api/v1/public/submissions/{sub['id']}?token={sub['receiptToken']}")
    assert r.status_code == 200
    st = r.json()
    assert (st["id"], st["formTitle"], st["state"], len(st["states"])) == (sub["id"], "Contact", "new", 3)
    assert [h["label"] for h in st["history"]] == ["New"] and st["createdAt"].endswith("Z")
    for path in (
        f"/api/v1/public/submissions/{sub['id']}?token=nope",
        f"/api/v1/public/submissions/{sub['id']}",
        f"/api/v1/public/submissions/{uuid.uuid4()}?token={sub['receiptToken']}",
        f"/api/v1/public/submissions/not-a-uuid?token={sub['receiptToken']}",
    ):
        r = await api.do("GET", path)
        assert r.status_code == 404 and error_code(r) == "not_found", path


async def test_public_submit_uses_form_confirmation_message(api):
    f = sample_form("plain")
    f.settings.confirmation_message = "We'll reply within a day."
    assert (await api.do("PUT", "/api/v1/forms/plain", api.admin, f.to_dict())).status_code == 200
    sub = (await api.do("POST", SUBMIT.format("plain"), body={"data": valid_data()})).json()
    assert sub["confirmationMessage"] == "We'll reply within a day." and sub["state"] == "submitted"


async def test_public_submit_validation(api):
    await api.seed()
    data = valid_data()
    del data["email"]
    r = await api.do("POST", SUBMIT.format("contact"), body={"data": data})
    assert r.status_code == 422 and "data.email" in [d["path"] for d in r.json()["error"]["details"]]
    assert (await api.do("POST", SUBMIT.format("contact"), body={})).status_code == 422


async def test_public_submit_private_or_unknown_form(api):
    await api.seed()
    for slug in ("internal", "missing"):
        assert (await api.do("POST", SUBMIT.format(slug), body={"data": valid_data()})).status_code == 404


async def test_public_submit_bad_bodies(api):
    await api.seed()
    r = await api.do("POST", SUBMIT.format("contact"), body=json.dumps("not an object"))
    assert r.status_code == 400 and error_code(r) == "bad_request"
    r = await api.do("POST", SUBMIT.format("contact"), body={"data": {**valid_data(), "name": "a" * (2 << 20)}})
    assert r.status_code == 413 and error_code(r) == "bad_request"
    r = await api.do("POST", SUBMIT.format("contact"), body={"data": "x"})
    assert r.status_code == 400


async def test_public_submit_rate_limited(api):
    await api.seed()
    for i in range(20):
        r = await api.do("POST", SUBMIT.format("plain"), body={"data": valid_data()})
        assert r.status_code == 201, (i, r.text)
    r = await api.do("POST", SUBMIT.format("plain"), body={"data": valid_data()})
    assert r.status_code == 429 and error_code(r) == "rate_limited" and r.headers["retry-after"] == "60"


async def test_submissions_require_auth(api):
    for path in ("/api/v1/submissions", f"/api/v1/submissions/{uuid.uuid4()}", "/api/v1/forms/contact/submissions.csv"):
        r = await api.do("GET", path)
        assert r.status_code == 401 and error_code(r) == "unauthenticated", path


async def test_list_submissions_paginates(api):
    await api.seed()
    for _ in range(3):
        await api.create("contact")
    key = await api.api_key("reviewer")
    p1 = (await api.do("GET", "/api/v1/submissions?limit=2", key)).json()
    assert len(p1["items"]) == 2 and p1["nextCursor"]
    assert p1["items"][0]["stateLabel"] == "New" and p1["items"][0]["form"] == "contact"
    r = await api.do("GET", f"/api/v1/submissions?limit=2&cursor={p1['nextCursor']}", key)
    assert len(r.json()["items"]) == 1 and '"nextCursor":null' in r.text


async def test_list_submissions_filters(api):
    await api.seed()
    assigned, _ = await api.create("contact")
    await api.create("contact")
    await api.create("plain")
    rev = await api.user("rev@example.com", "reviewer")
    async with api.db.transaction() as s:
        await s.execute(
            sa.text("UPDATE submissions SET assignee_id = :u, state = 'review' WHERE id = :id"),
            {"u": rev.id, "id": assigned.id},
        )
    key = await api.api_key("reviewer")
    cases = {
        "form=plain": 1,
        "state=review": 1,
        "form=contact&state=new": 1,
        "assignee=none": 2,
        f"assignee={rev.id}": 1,
        "assignee=me": 0,
    }
    for q, want in cases.items():
        r = await api.do("GET", f"/api/v1/submissions?{q}", key)
        assert r.status_code == 200 and len(r.json()["items"]) == want, q
    item = (await api.do("GET", f"/api/v1/submissions?assignee={rev.id}", key)).json()["items"][0]
    assert item["assignee"]["email"] == "rev@example.com" and item["stateLabel"] == "In review"
    for q in ("assignee=bogus", "limit=abc", "limit=0", "cursor=%21%21garbage"):
        r = await api.do("GET", f"/api/v1/submissions?{q}", key)
        assert r.status_code == 400 and error_code(r) == "bad_request", q


async def test_get_submission_detail(api):
    await api.seed()
    sub, _ = await api.create("contact")
    key = await api.api_key("reviewer")
    got = (await api.do("GET", f"/api/v1/submissions/{sub.id}", key)).json()
    assert got["submission"]["id"] == str(sub.id) and got["submission"]["data"]["name"] == "Ada Lovelace"
    assert got["form"]["slug"] == "contact" and got["workflow"]["slug"] == "review"
    assert [(e["type"], e["toState"]) for e in got["events"]] == [("created", "new")]
    assert got["transitions"] == []
    plain, _ = await api.create("plain")
    assert '"workflow":null' in (await api.do("GET", f"/api/v1/submissions/{plain.id}", key)).text
    for sid in (uuid.uuid4(), "abc"):
        assert (await api.do("GET", f"/api/v1/submissions/{sid}", key)).status_code == 404


async def test_create_submission_authenticated(api):
    await api.seed()
    key = await api.api_key("integrator")
    r = await api.do("POST", "/api/v1/submissions", key, {"form": "internal", "data": valid_data()})
    assert r.status_code == 201, r.text
    got = r.json()
    assert got["submission"]["form"] == "internal" and got["submission"]["state"] == "new" and got["receiptToken"]
    detail = await api.do("GET", f"/api/v1/submissions/{got['submission']['id']}", key)
    assert '"type":"api_key"' in detail.text
    assert (
        await api.do("POST", "/api/v1/submissions", key, {"form": "missing", "data": valid_data()})
    ).status_code == 404
    assert (await api.do("POST", "/api/v1/submissions", key, {"form": "contact", "data": {}})).status_code == 422


async def test_export_csv_endpoint(api):
    await api.seed()
    await api.create("contact")
    key = await api.api_key("reviewer")
    r = await api.do("GET", "/api/v1/forms/contact/submissions.csv", key)
    assert r.status_code == 200
    assert r.headers["content-type"] == "text/csv; charset=utf-8"
    assert r.headers["content-disposition"] == 'attachment; filename="contact-submissions.csv"'
    records = list(csv.reader(io.StringIO(r.text)))
    assert len(records) == 2 and records[0][0] == "id"
    r = await api.do("GET", "/api/v1/forms/missing/submissions.csv", key)
    assert r.status_code == 404 and error_code(r) == "not_found"


def test_rate_limiter_window():
    clock = [0.0]
    lim = RateLimiter(20, 60.0, now=lambda: clock[0])
    assert all(lim.allow("198.51.100.7") for _ in range(20))
    assert not lim.allow("198.51.100.7")
    assert lim.allow("203.0.113.9")
    clock[0] += 59
    assert not lim.allow("198.51.100.7")
    clock[0] += 1
    assert lim.allow("198.51.100.7")


def test_rate_limiter_sweeps_expired_entries():
    clock = [0.0]
    lim = RateLimiter(1, 60.0, now=lambda: clock[0], max_keys=3)
    for k in "abc":
        lim.allow(k)
    clock[0] += 120
    lim.allow("d")
    assert len(lim._hits) == 1


T = dt.datetime(2026, 9, 23, 11, 0, tzinfo=dt.timezone(dt.timedelta(hours=2)))
ID = uuid.UUID("11111111-1111-4111-8111-111111111111")
USER = uuid.UUID("22222222-2222-4222-8222-222222222222")


def compact(v) -> str:
    return json.dumps(v, separators=(",", ":"))


def test_submission_json_shape():
    s = Submission(
        id=ID,
        org_id=ID,
        form_id=ID,
        form_version_id=ID,
        form_slug="contact",
        form_version=2,
        workflow_version_id=None,
        state="new",
        state_label="New",
        created_at=T,
        updated_at=T,
    )
    assert compact(submission_json(s)) == (
        '{"id":"11111111-1111-4111-8111-111111111111","form":"contact","formVersion":2,"state":"new","stateLabel":"New",'
        '"terminal":false,"data":{},"fields":{},"assignee":null,"createdAt":"2026-09-23T09:00:00Z","updatedAt":"2026-09-23T09:00:00Z"}'
    )
    s.assignee_id, s.assignee_name, s.assignee_email, s.data = USER, "Rita", "rita@example.com", {"name": "Ada"}
    assert (
        '"assignee":{"id":"22222222-2222-4222-8222-222222222222","name":"Rita","email":"rita@example.com"}'
        in compact(submission_json(s))
    )


def test_event_json_shape():
    e = Event(
        submission_id=ID,
        org_id=ID,
        id=7,
        type="created",
        to_state="new",
        actor_type="respondent",
        actor_name="Respondent",
        created_at=T,
    )
    assert compact(event_json(e)) == (
        '{"id":7,"type":"created","fromState":null,"toState":"new","transition":null,'
        '"actor":{"type":"respondent","id":null,"name":"Respondent"},"payload":{},"createdAt":"2026-09-23T09:00:00Z"}'
    )


def test_version_json_and_list_shapes():
    v = VersionInfo(id=ID, version=3, hash="abc", source="cli", created_by="dev", created_at=T)
    assert (
        compact(version_json(v))
        == '{"version":3,"hash":"abc","source":"cli","createdBy":"dev","createdAt":"2026-09-23T09:00:00Z"}'
    )
    assert compact(list_json([], "")) == '{"items":[],"nextCursor":null}'
    assert compact(list_json([1], "abc")) == '{"items":[1],"nextCursor":"abc"}'

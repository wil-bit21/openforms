import asyncio
import csv
import io
import uuid

import pytest
import sqlalchemy as sa

from openforms.definition import ValidationError
from openforms.server.exceptions import FormNotPublic, InvalidCursor, NotFound
from openforms.server.models import definitions as defs
from openforms.server.models import submissions as subs
from openforms.server.models.definitions import ApplyInput
from openforms.server.models.submissions import Event, ListFilter
from openforms.server.schemas.core import Principal
from tests.samples import sample_form, sample_workflow


def valid_data():
    return {"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}


@pytest.fixture
async def seeded(database, org_id):
    """Workflow "review"; forms "contact" (public, review), "plain" (public, no workflow),
    "internal" (private, review)."""
    internal = sample_form("internal", "review")
    internal.settings.public = False
    async with database.transaction() as s:
        await defs.apply(
            s,
            org_id,
            ApplyInput(
                workflows=[sample_workflow("review")],
                forms=[sample_form("contact", "review"), sample_form("plain"), internal],
            ),
        )
    return database


async def create(db, org_id, slug="contact", data=None, require_public=True, **kw):
    async with db.transaction() as s:
        return await subs.create(
            s, org_id, slug, valid_data() if data is None else data, require_public=require_public, **kw
        )


async def scalar(db, sql, **params):
    async with db.session() as s:
        return (await s.execute(sa.text(sql), params)).first()


async def other_org(db) -> uuid.UUID:
    oid = uuid.uuid4()
    async with db.transaction() as s:
        await s.execute(sa.text("INSERT INTO orgs (id, name) VALUES (:id, 'Other')"), {"id": oid})
    return oid


async def test_create_starts_in_initial_state(seeded, org_id):
    sub, token = await create(seeded, org_id)
    assert (sub.state, sub.state_label, sub.terminal) == ("new", "New", False)
    assert sub.form_slug == "contact" and sub.form_version == 1 and sub.workflow_version_id is not None
    assert len(token) == 32
    assert sub.data == valid_data() and sub.fields == {}
    async with seeded.session() as s:
        got = await subs.get(s, org_id, sub.id)
    assert (got.state, got.state_label, got.form_slug, got.created_at) == ("new", "New", "contact", sub.created_at)
    ev = await scalar(
        seeded,
        "SELECT type, to_state, actor_type, actor_name FROM submission_events WHERE submission_id = :id",
        id=sub.id,
    )
    assert tuple(ev) == ("created", "new", "respondent", "Respondent")
    stored = (await scalar(seeded, "SELECT receipt_token_hash FROM submissions WHERE id = :id", id=sub.id))[0]
    assert stored != token and len(stored) == 64


async def test_create_strips_unknown_keys(seeded, org_id):
    sub, _ = await create(seeded, org_id, data={**valid_data(), "isAdmin": True})
    assert "isAdmin" not in sub.data


async def test_create_validation_error(seeded, org_id):
    data = valid_data()
    del data["email"]
    with pytest.raises(ValidationError) as ei:
        await create(seeded, org_id, data=data)
    assert "data.email" in [p.path for p in ei.value.problems]
    with pytest.raises(ValidationError):
        async with seeded.transaction() as s:
            await subs.create(s, org_id, "contact", None, require_public=True)


async def test_create_require_public(seeded, org_id):
    with pytest.raises(FormNotPublic):
        await create(seeded, org_id, "internal")
    await create(seeded, org_id, "internal", require_public=False)


async def test_create_unknown_form(seeded, org_id):
    with pytest.raises(NotFound):
        await create(seeded, org_id, "missing")


async def test_create_without_workflow(seeded, org_id):
    sub, _ = await create(seeded, org_id, "plain")
    assert (sub.state, sub.state_label, sub.terminal, sub.workflow_version_id) == ("submitted", "Submitted", True, None)


async def test_create_records_principal_actor(seeded, org_id):
    p = Principal(org_id=org_id, kind="api_key", id=org_id, name="ci-key")
    sub, _ = await create(seeded, org_id, require_public=False, principal=p)
    ev = await scalar(
        seeded, "SELECT actor_type, actor_name FROM submission_events WHERE submission_id = :id", id=sub.id
    )
    assert tuple(ev) == ("api_key", "ci-key")


async def test_created_hook_receives_workflow_in_transaction(seeded, org_id):
    got = {}

    async def hook(session, sub, wf):
        got["sub"], got["wf"] = sub, wf
        n = (await session.execute(sa.text("SELECT count(*) FROM submissions WHERE id = :id"), {"id": sub.id})).scalar()
        got["visible"] = n == 1

    sub, _ = await create(seeded, org_id, on_created=hook)
    assert got["wf"].slug == "review" and got["sub"].id == sub.id and got["visible"]
    await create(seeded, org_id, "plain", on_created=hook)
    assert got["wf"] is None


async def test_created_hook_error_rolls_back(seeded, org_id):
    async def hook(session, sub, wf):
        raise RuntimeError("boom")

    with pytest.raises(RuntimeError, match="boom"):
        await create(seeded, org_id, on_created=hook)
    assert (await scalar(seeded, "SELECT count(*) FROM submissions"))[0] == 0


async def test_get_is_org_scoped(seeded, org_id):
    sub, _ = await create(seeded, org_id)
    other = await other_org(seeded)
    with pytest.raises(NotFound):
        async with seeded.session() as s:
            await subs.get(s, other, sub.id)


async def test_get_for_update_inside_tx(seeded, org_id):
    sub, _ = await create(seeded, org_id)
    async with seeded.transaction() as s:
        got = await subs.get(s, org_id, sub.id, for_update=True)
    assert got.id == sub.id and got.state_label == "New"


async def test_submission_keeps_pinned_versions(seeded, org_id):
    sub, _ = await create(seeded, org_id)
    wf = sample_workflow("review")
    wf.states[0].label = "Fresh"
    async with seeded.transaction() as s:
        await defs.apply(s, org_id, ApplyInput(workflows=[wf]))
    async with seeded.session() as s:
        got = await subs.get(s, org_id, sub.id)
    assert (got.state_label, got.form_version) == ("New", 1)
    newer, _ = await create(seeded, org_id)
    assert (newer.state_label, newer.form_version) == ("Fresh", 2)


async def list_(db, org_id, **kw):
    async with db.session() as s:
        return await subs.list_submissions(s, org_id, ListFilter(**kw))


async def test_list_paginates_without_duplicates(seeded, org_id):
    want = {(await create(seeded, org_id))[0].id for _ in range(5)}
    async with seeded.transaction() as s:
        await s.execute(sa.text("UPDATE submissions SET created_at = '2026-09-23T10:00:00Z'"))
    seen, cursor, pages = set(), "", 0
    while True:
        items, nxt = await list_(seeded, org_id, limit=2, cursor=cursor)
        pages += 1
        for it in items:
            assert it.id not in seen
            seen.add(it.id)
            assert it.state_label == "New"
        if not nxt:
            break
        cursor = nxt
    assert pages == 3 and seen == want


async def test_list_newest_first(seeded, org_id):
    older, _ = await create(seeded, org_id)
    newer, _ = await create(seeded, org_id)
    async with seeded.transaction() as s:
        await s.execute(
            sa.text("UPDATE submissions SET created_at = created_at - interval '1 hour' WHERE id = :id"),
            {"id": older.id},
        )
    items, nxt = await list_(seeded, org_id)
    assert [i.id for i in items] == [newer.id, older.id] and nxt == ""


async def test_list_filters(seeded, org_id):
    a, _ = await create(seeded, org_id)
    await create(seeded, org_id)
    await create(seeded, org_id, "plain")
    from openforms.server.models import auth

    async with seeded.transaction() as s:
        reviewer = await auth.create_user(s, org_id, "rev@example.com", "Rev", "password123", ["reviewer"])
        await s.execute(
            sa.text("UPDATE submissions SET state = 'review', assignee_id = :u WHERE id = :id"),
            {"u": reviewer.id, "id": a.id},
        )
    assert len((await list_(seeded, org_id, form_slug="plain"))[0]) == 1
    by_state = (await list_(seeded, org_id, state="review"))[0]
    assert len(by_state) == 1 and by_state[0].id == a.id and by_state[0].state_label == "In review"
    assigned = (await list_(seeded, org_id, assignee_id=reviewer.id))[0]
    assert len(assigned) == 1 and assigned[0].assignee_name and assigned[0].assignee_email == "rev@example.com"
    unassigned = (await list_(seeded, org_id, unassigned=True))[0]
    assert len(unassigned) == 2 and a.id not in {u.id for u in unassigned}
    assert len((await list_(seeded, org_id, form_slug="contact", state="new"))[0]) == 1
    assert len((await list_(seeded, org_id))[0]) == 3
    assert (await list_(seeded, await other_org(seeded)))[0] == []


async def test_list_limit_bounds(seeded, org_id):
    for _ in range(3):
        await create(seeded, org_id, "plain")
    items, nxt = await list_(seeded, org_id, limit=1000)
    assert len(items) == 3 and nxt == ""


@pytest.mark.parametrize("cursor", ["garbage!!", "bm90LWEtY3Vyc29y"])
async def test_list_invalid_cursor(seeded, org_id, cursor):
    with pytest.raises(InvalidCursor):
        await list_(seeded, org_id, cursor=cursor)


def test_cursor_accepts_go_nanosecond_cursors():
    import base64

    c = base64.urlsafe_b64encode(f"2026-09-23T10:00:00.123456789Z|{uuid.UUID(int=1)}".encode()).rstrip(b"=").decode()
    t, i = subs.decode_cursor(c)
    assert t.microsecond == 123456 and i == uuid.UUID(int=1)


async def test_events(seeded, org_id):
    sub, _ = await create(seeded, org_id)
    async with seeded.transaction() as s:
        await subs.insert_event(
            s,
            Event(
                submission_id=sub.id,
                org_id=org_id,
                type="comment",
                actor_type="system",
                actor_name="System",
                payload={"body": "hello"},
            ),
        )
    async with seeded.session() as s:
        evs = await subs.events(s, org_id, sub.id)
    assert len(evs) == 2
    assert (evs[0].type, evs[0].to_state, evs[0].from_state, evs[0].actor_type) == ("created", "new", "", "respondent")
    assert (evs[1].type, evs[1].payload["body"], evs[1].to_state) == ("comment", "hello", "")
    other = await other_org(seeded)
    with pytest.raises(NotFound):
        async with seeded.session() as s:
            await subs.events(s, other, sub.id)


async def test_count_by_form(seeded, org_id):
    await create(seeded, org_id)
    await create(seeded, org_id)
    await create(seeded, org_id, "plain")
    async with seeded.session() as s:
        counts = await subs.count_by_form(s, org_id)
        contact = await defs.get_form(s, org_id, "contact")
        plain = await defs.get_form(s, org_id, "plain")
    assert counts[contact.id] == 2 and counts[plain.id] == 1


async def status(db, sub_id, token):
    async with db.session() as s:
        return await subs.public_status(s, sub_id, token)


async def test_public_status(seeded, org_id):
    sub, token = await create(seeded, org_id)
    st = await status(seeded, sub.id, token)
    assert (st.id, st.form_title, st.state, st.state_label, st.terminal) == (sub.id, "Contact", "new", "New", False)
    assert len(st.states) == 3 and st.states[2].key == "done" and st.states[2].terminal
    assert [(h.state, h.label) for h in st.history] == [("new", "New")]
    async with seeded.transaction() as s:
        await s.execute(sa.text("UPDATE submissions SET state = 'review' WHERE id = :id"), {"id": sub.id})
        await subs.insert_event(
            s,
            Event(
                submission_id=sub.id,
                org_id=org_id,
                type="transition",
                from_state="new",
                to_state="review",
                transition="start",
                actor_type="system",
            ),
        )
        await subs.insert_event(
            s,
            Event(
                submission_id=sub.id,
                org_id=org_id,
                type="comment",
                actor_type="system",
                payload={"body": "internal note"},
            ),
        )
    st = await status(seeded, sub.id, token)
    assert st.state_label == "In review" and len(st.history) == 2 and st.history[1].label == "In review"


async def test_public_status_without_workflow(seeded, org_id):
    sub, token = await create(seeded, org_id, "plain")
    st = await status(seeded, sub.id, token)
    assert st.terminal and [(s.key, s.label) for s in st.states] == [("submitted", "Submitted")]


async def test_public_status_rejects_bad_token(seeded, org_id):
    sub, token = await create(seeded, org_id)
    for sid, tok in ((sub.id, "A" * len(token)), (sub.id, ""), (uuid.uuid4(), token), (sub.id, "x")):
        with pytest.raises(NotFound):
            await status(seeded, sid, tok)


async def test_export_csv_escapes_formulas(seeded, org_id):
    sub, _ = await create(seeded, org_id, data={**valid_data(), "name": '=HYPERLINK("http://evil")'})
    async with seeded.transaction() as s:
        await s.execute(
            sa.text("""UPDATE submissions SET fields = '{"note": "+1 looks good"}' WHERE id = :id"""), {"id": sub.id}
        )
        text = await subs.export_csv(s, org_id, "contact")
    records = list(csv.reader(io.StringIO(text)))
    assert records[0] == ["id", "createdAt", "state", "formVersion", "name", "email", "topic", "fields.note"]
    assert len(records) == 2
    row = records[1]
    assert (row[0], row[2], row[3]) == (str(sub.id), "new", "1")
    assert row[4] == '\'=HYPERLINK("http://evil")' and row[7] == "'+1 looks good"
    assert text.endswith("\n") and "\r\n" not in text


async def test_export_csv_unknown_form(seeded, org_id):
    with pytest.raises(NotFound):
        async with seeded.session() as s:
            await subs.export_csv(s, org_id, "missing")


def test_csv_values():
    assert subs.csv_value(["go", "ts"]) == "go;ts"
    assert subs.csv_value(3.5) == "3.5" and subs.csv_value(42) == "42" and subs.csv_value(True) == "true"
    assert subs.csv_value({"b": 1, "a": 2}) == '{"a":2,"b":1}'
    assert subs._csv_line([" lead", "a,b", 'q"']) == '" lead","a,b","q"""\n'


async def test_set_assignee(seeded, org_id):
    sub, _ = await create(seeded, org_id)
    from openforms.server.models import auth

    async with seeded.transaction() as s:
        u = await auth.create_user(s, org_id, "rev@example.com", "Rev", "password123", ["reviewer"])
        await subs.set_assignee(s, org_id, sub.id, u.id)
    async with seeded.session() as s:
        got = await subs.get(s, org_id, sub.id)
    assert got.assignee_id == u.id and got.assignee_email == "rev@example.com"
    async with seeded.transaction() as s:
        await subs.set_assignee(s, org_id, sub.id, None)
        assert (await subs.get(s, org_id, sub.id)).assignee_id is None
    with pytest.raises(NotFound):
        async with seeded.transaction() as s:
            await subs.set_assignee(s, org_id, uuid.uuid4(), None)


async def test_concurrent_creates(seeded, org_id):
    results = await asyncio.gather(*(create(seeded, org_id) for _ in range(5)))
    assert len({r[0].id for r in results}) == 5

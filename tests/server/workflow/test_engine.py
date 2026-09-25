import asyncio
import uuid

import pytest

from openforms.definition import ValidationError
from openforms.server.actions import TRIGGER_SUBMIT
from openforms.server.exceptions import (
    Forbidden,
    InvalidState,
    NotFound,
    NoWorkflow,
    StateConflict,
    UnknownTransition,
)
from openforms.server.models import submissions as subs
from openforms.server.workflow import Engine, TransitionInput


@pytest.fixture
def engine(fx):
    return Engine(fx.db, fx.queue)


async def available(fx, engine, p, sub):
    async with fx.db.session() as s:
        return await engine.available(s, p, sub)


async def transition_events(fx, sub_id):
    return await fx.events_of_type(sub_id, "transition")


async def test_available(fx, engine):
    sub = await fx.applicant()
    av = {a.key: a for a in await available(fx, engine, await fx.principal_with_roles("reviewer"), sub)}
    assert set(av) == {"screen", "reject"} and av["screen"].allowed and av["reject"].allowed
    assert av["screen"].to_label == "Screening" and av["screen"].require_fields == []
    assert av["reject"].require_fields == ["rejectionReason"]
    for a in await available(fx, engine, await fx.principal_with_roles(), sub):
        assert not a.allowed and a.reason == "role"
    admin = await fx.principal_with_roles("admin")
    assert all(a.allowed for a in await available(fx, engine, admin, sub))
    plain = await fx.submit("feedback", {"message": "hi"})
    assert await available(fx, engine, admin, plain) == []


async def test_transition_success(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    got = await engine.transition(p, sub.id, TransitionInput("screen", comment="  looks good  "))
    assert (got.state, got.state_label) == ("screening", "Screening")
    evs = await transition_events(fx, sub.id)
    assert len(evs) == 1
    ev = evs[0]
    assert (ev.from_state, ev.to_state, ev.transition, ev.actor_type, ev.actor_id, ev.actor_name) == (
        "new",
        "screening",
        "screen",
        "user",
        p.id,
        p.name,
    )
    assert ev.payload["comment"] == "looks good"
    assert await fx.jobs() == []


async def test_transition_errors(fx, engine):
    reviewer = await fx.principal_with_roles("reviewer")
    sub = await fx.applicant()
    plain = await fx.submit("feedback", {"message": "hi"})
    cases = [
        (reviewer, sub.id, TransitionInput("teleport"), UnknownTransition),
        (reviewer, sub.id, TransitionInput("invite", fields={"score": 5}), InvalidState),
        (await fx.principal_with_roles("hiring-manager"), sub.id, TransitionInput("screen"), Forbidden),
        (reviewer, sub.id, TransitionInput("screen", expected_state="interview"), StateConflict),
        (reviewer, plain.id, TransitionInput("screen"), NoWorkflow),
        (reviewer, uuid.uuid4(), TransitionInput("screen"), NotFound),
    ]
    for p, sid, inp, want in cases:
        with pytest.raises(want):
            await engine.transition(p, sid, inp)
    assert (await fx.reload(sub.id)).state == "new"


async def test_transition_require_fields(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    with pytest.raises(ValidationError) as ei:
        await engine.transition(p, sub.id, TransitionInput("reject", fields={"rejectionReason": "   "}))
    probs = ei.value.problems
    assert (
        len(probs) == 1
        and probs[0].path == "fields.rejectionReason"
        and "required for transition reject" in probs[0].message
    )
    got = await fx.reload(sub.id)
    assert got.state == "new" and got.fields == {}
    assert await fx.jobs() == []
    with pytest.raises(ValidationError) as ei:
        await engine.transition(
            p, sub.id, TransitionInput("reject", fields={"score": "not a number", "rejectionReason": "x"})
        )
    assert ei.value.problems[0].path == "fields.score"
    with pytest.raises(ValidationError) as ei:
        await engine.transition(p, sub.id, TransitionInput("reject", fields={"nope": 1, "rejectionReason": "x"}))
    assert ei.value.problems[0].path == "fields.nope"


async def test_transition_enqueues_actions_with_event_id(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    got = await engine.transition(
        p, sub.id, TransitionInput("reject", fields={"rejectionReason": "Not a fit"}, comment="sorry")
    )
    assert got.state == "rejected" and got.terminal and got.fields["rejectionReason"] == "Not a fit"
    ev = (await transition_events(fx, sub.id))[0]
    assert ev.payload["fields"]["rejectionReason"] == "Not a fit"
    jobs = await fx.jobs()
    assert [j.kind for j in jobs] == ["action.email", "action.assign"]
    for j in jobs:
        assert (
            j.payload["trigger"] == "reject"
            and j.payload["eventId"] == ev.id
            and j.payload["submissionId"] == str(sub.id)
        )


async def test_fields_already_set_satisfy_guard(fx, engine):
    sub = await fx.applicant()
    admin = await fx.principal_with_roles("admin")
    await engine.transition(admin, sub.id, TransitionInput("screen", fields={"score": 4}))
    got = await engine.transition(admin, sub.id, TransitionInput("invite"))
    assert got.state == "interview" and got.fields["score"] == 4


async def test_rollback_leaves_no_jobs(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")

    async def boom():
        raise RuntimeError("commit exploded")

    engine.before_commit = boom
    with pytest.raises(RuntimeError, match="commit exploded"):
        await engine.transition(p, sub.id, TransitionInput("reject", fields={"rejectionReason": "x"}))
    assert await fx.jobs() == []
    got = await fx.reload(sub.id)
    assert got.state == "new" and got.fields == {}
    assert await transition_events(fx, sub.id) == []


@pytest.mark.parametrize("expected", ["", "new"])
async def test_concurrent_only_one_wins(fx, engine, expected):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    results = await asyncio.gather(
        *(engine.transition(p, sub.id, TransitionInput("screen", expected_state=expected)) for _ in range(2)),
        return_exceptions=True,
    )
    wins = [r for r in results if not isinstance(r, BaseException)]
    losses = [r for r in results if isinstance(r, (InvalidState, StateConflict))]
    assert len(wins) == 1 and len(losses) == 1, results
    assert len(await transition_events(fx, sub.id)) == 1


async def test_on_created_enqueues_on_submit_actions(fx, engine):
    sub = await fx.applicant(on_created=engine.on_created)
    jobs = await fx.jobs()
    assert len(jobs) == 1
    p = jobs[0].payload
    assert p["trigger"] == TRIGGER_SUBMIT and p["submissionId"] == str(sub.id) and p["action"]["type"] == "email"
    created = next(e for e in await fx.events(sub.id) if e.type == "created")
    assert p["eventId"] == created.id
    await fx.submit("feedback", {"message": "hi"}, on_created=engine.on_created)
    assert len(await fx.jobs()) == 1


async def test_update_fields(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    got = await engine.update_fields(p, sub.id, {"score": 3, "priority": "high"})
    assert got.fields == {"score": 3, "priority": "high"} and got.state == "new"
    got = await engine.update_fields(p, sub.id, {"priority": None})
    assert got.fields == {"score": 3}
    assert len(await fx.events_of_type(sub.id, "fields_updated")) == 2
    await engine.update_fields(p, sub.id, {"score": 3})
    await engine.update_fields(p, sub.id, {"score": 3.0})
    assert len(await fx.events_of_type(sub.id, "fields_updated")) == 2
    with pytest.raises(ValidationError) as ei:
        await engine.update_fields(p, sub.id, {"priority": "urgent"})
    assert ei.value.problems[0].path == "fields.priority"
    with pytest.raises(ValidationError) as ei:
        await engine.update_fields(p, sub.id, {"ghost": None})
    assert ei.value.problems[0].path == "fields.ghost"
    plain = await fx.submit("feedback", {"message": "hi"})
    with pytest.raises(NoWorkflow):
        await engine.update_fields(p, plain.id, {"score": 1})


async def test_blank_string_unsets_a_field(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    await engine.update_fields(p, sub.id, {"rejectionReason": "x"})
    got = await engine.update_fields(p, sub.id, {"rejectionReason": "   "})
    assert "rejectionReason" not in got.fields


async def test_comment(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles()
    ev = await engine.comment(p, sub.id, "  Strong portfolio.  ")
    assert ev.type == "comment" and ev.payload["body"] == "Strong portfolio." and ev.actor_name == p.name and ev.id
    for body in ("   ", "x" * 10001):
        with pytest.raises(ValidationError) as ei:
            await engine.comment(p, sub.id, body)
        assert ei.value.problems[0].path == "body"
    with pytest.raises(NotFound):
        await engine.comment(p, uuid.uuid4(), "hi")


async def test_assign(fx, engine):
    sub = await fx.applicant()
    p = await fx.principal_with_roles("reviewer")
    u = await fx.user("assignee@example.com", "reviewer")
    got = await engine.assign(p, sub.id, u.id)
    assert got.assignee_id == u.id and got.assignee_email == "assignee@example.com"
    await engine.assign(p, sub.id, u.id)
    assert len(await fx.events_of_type(sub.id, "assigned")) == 1
    got = await engine.assign(p, sub.id, None)
    assert got.assignee_id is None
    last = (await fx.events_of_type(sub.id, "assigned"))[-1]
    assert last.payload["assigneeId"] is None and last.actor_name == p.name
    with pytest.raises(ValidationError) as ei:
        await engine.assign(p, sub.id, uuid.uuid4())
    assert ei.value.problems[0].path == "userId"


def test_subs_constants_exist():
    assert subs.EVENT_TRANSITION == "transition"

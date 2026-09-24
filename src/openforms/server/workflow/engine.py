"""Moves submissions through their workflow's state machine (port of the Go ``internal/workflow``).

Every write runs in one transaction: the state change, its event and the action
jobs commit together or not at all (transactional outbox).
"""

from __future__ import annotations

import json
import uuid
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from typing import Any

import sqlalchemy as sa
from sqlalchemy.ext.asyncio import AsyncSession

from ...definition import Problem, ValidationError, Workflow, validate_workflow_fields
from ...definition.common import trim
from ...definition.values import json_equal
from ..actions import TRIGGER_SUBMIT, Payload, enqueue
from ..database.engine import Database
from ..exceptions import Forbidden, InvalidState, NoWorkflow, StateConflict, UnknownTransition
from ..models import definitions as defs
from ..models import submissions as subs
from ..schemas.core import Principal
from ..services.jobs import Queue

MAX_COMMENT_CHARS = 10000


@dataclass
class AvailableTransition:
    key: str
    label: str
    to: str
    to_label: str = ""
    require_fields: list[str] = field(default_factory=list)
    allowed: bool = True
    reason: str = ""  # "" or "role"

    def to_dict(self) -> dict[str, Any]:
        return {
            "key": self.key,
            "label": self.label,
            "to": self.to,
            "toLabel": self.to_label,
            "requireFields": list(self.require_fields),
            "allowed": self.allowed,
            "reason": self.reason,
        }


@dataclass
class TransitionInput:
    transition: str
    fields: dict[str, Any] = field(default_factory=dict)
    comment: str = ""
    expected_state: str = ""


def is_empty(v: Any) -> bool:
    """Whether a workflow field value counts as "not provided"."""
    if v is None:
        return True
    if isinstance(v, str):
        return trim(v) == ""
    if isinstance(v, bool):
        return not v
    if isinstance(v, list):
        return len(v) == 0
    return False


def merge_fields(wf: Workflow, current: dict[str, Any], patch: dict[str, Any]) -> tuple[dict[str, Any], dict[str, Any]]:
    """Validate ``patch`` against the workflow's fields and apply it to ``current``. A None
    value unsets that field. Returns the merged map and the changed subset (removed keys → None)."""
    merged = dict(current)
    changed: dict[str, Any] = {}
    sets: dict[str, Any] = {}
    unset: list[str] = []
    problems: list[Problem] = []
    for k, v in (patch or {}).items():
        if v is not None:
            sets[k] = v
        elif wf.field(k) is None:
            problems.append(Problem("fields." + k, "unknown workflow field"))
        else:
            unset.append(k)
    clean: dict[str, Any] = {}
    try:
        clean = validate_workflow_fields(wf, sets)
    except ValidationError as e:
        problems += e.problems
    if problems:
        raise ValidationError(sorted(problems, key=lambda p: p.path))
    for k in unset:
        if k in merged:
            del merged[k]
            changed[k] = None
    for k, v in clean.items():
        if v is None:  # a blank string also unsets
            if k in merged:
                del merged[k]
                changed[k] = None
            continue
        if k not in merged or not json_equal(merged[k], v):
            merged[k] = v
            changed[k] = v
    return merged, changed


class Engine:
    def __init__(self, db: Database, queue: Queue) -> None:
        self.db = db
        self.queue = queue
        # Runs at the end of a transition transaction; tests only.
        self.before_commit: Callable[[], Awaitable[None]] | None = None

    async def _workflow_for(self, session: AsyncSession, sub: subs.Submission) -> Workflow:
        if sub.workflow_version_id is None:
            raise NoWorkflow()
        return (await defs.get_workflow_version(session, sub.workflow_version_id)).definition

    async def available(self, session: AsyncSession, p: Principal, sub: subs.Submission) -> list[AvailableTransition]:
        """Transitions leaving the current state and whether ``p`` may perform each.
        Forms without a workflow yield an empty list."""
        if sub.workflow_version_id is None:
            return []
        wf = await self._workflow_for(session, sub)
        out = []
        for t in wf.transitions or []:
            if sub.state not in t.sources():
                continue
            at = AvailableTransition(key=t.key, label=t.label, to=t.to, require_fields=list(t.guard.require_fields))
            st = wf.state(t.to)
            if st is not None:
                at.to_label = st.label
            if not p.has_any_role(*t.guard.roles):
                at.allowed, at.reason = False, "role"
            out.append(at)
        return out

    async def on_created(self, session: AsyncSession, sub: subs.Submission, wf: Workflow | None) -> None:
        """Enqueue the workflow's onSubmit actions in the creating transaction."""
        if wf is None or not wf.on_submit:
            return
        event_id = (
            await session.execute(
                sa.text(
                    "SELECT id FROM submission_events WHERE submission_id = :id AND type = 'created' ORDER BY id LIMIT 1"
                ),
                {"id": sub.id},
            )
        ).scalar() or 0
        for a in wf.on_submit:
            await enqueue(
                self.queue,
                session,
                sub.org_id,
                Payload(submission_id=sub.id, trigger=TRIGGER_SUBMIT, event_id=event_id, action=a),
            )

    async def transition(self, p: Principal, sub_id: uuid.UUID, inp: TransitionInput) -> subs.Submission:
        """The spec §6.10 algorithm in one transaction."""
        async with self.db.transaction() as s:
            sub = await subs.get(s, p.org_id, sub_id, for_update=True)
            wf = await self._workflow_for(s, sub)
            t = wf.transition(inp.transition)
            if t is None:
                raise UnknownTransition()
            if inp.expected_state and inp.expected_state != sub.state:
                raise StateConflict()
            if sub.state not in t.sources():
                raise InvalidState()
            if not p.has_any_role(*t.guard.roles):
                raise Forbidden()
            merged, changed = merge_fields(wf, sub.fields, inp.fields)
            problems = [
                Problem("fields." + k, "required for transition " + t.key)
                for k in t.guard.require_fields
                if is_empty(merged.get(k))
            ]
            if problems:
                raise ValidationError(problems)
            await s.execute(
                sa.text("""UPDATE submissions SET state = :to, fields = CAST(:fields AS jsonb), updated_at = now()
                WHERE org_id = :org AND id = :id"""),
                {"to": t.to, "fields": json.dumps(merged), "org": p.org_id, "id": sub_id},
            )
            actor = subs.actor_from_principal(p)
            ev = await subs.insert_event(
                s,
                subs.Event(
                    submission_id=sub_id,
                    org_id=p.org_id,
                    type=subs.EVENT_TRANSITION,
                    from_state=sub.state,
                    to_state=t.to,
                    transition=t.key,
                    actor_type=actor.type,
                    actor_id=actor.id,
                    actor_name=actor.name,
                    payload={"comment": inp.comment.strip(), "fields": changed},
                ),
            )
            for a in t.actions:
                await enqueue(
                    self.queue, s, p.org_id, Payload(submission_id=sub_id, trigger=t.key, event_id=ev.id, action=a)
                )
            if self.before_commit is not None:
                await self.before_commit()
        async with self.db.session() as s:
            return await subs.get(s, p.org_id, sub_id)

    async def update_fields(self, p: Principal, sub_id: uuid.UUID, fields: dict[str, Any]) -> subs.Submission:
        """Edit workflow fields without changing state. No-op edits write no event."""
        async with self.db.transaction() as s:
            sub = await subs.get(s, p.org_id, sub_id, for_update=True)
            wf = await self._workflow_for(s, sub)
            merged, changed = merge_fields(wf, sub.fields, fields)
            if changed:
                await s.execute(
                    sa.text("""UPDATE submissions SET fields = CAST(:fields AS jsonb), updated_at = now()
                    WHERE org_id = :org AND id = :id"""),
                    {"fields": json.dumps(merged), "org": p.org_id, "id": sub_id},
                )
                actor = subs.actor_from_principal(p)
                await subs.insert_event(
                    s,
                    subs.Event(
                        submission_id=sub_id,
                        org_id=p.org_id,
                        type=subs.EVENT_FIELDS_UPDATED,
                        actor_type=actor.type,
                        actor_id=actor.id,
                        actor_name=actor.name,
                        payload={"fields": changed},
                    ),
                )
        async with self.db.session() as s:
            return await subs.get(s, p.org_id, sub_id)

    async def comment(self, p: Principal, sub_id: uuid.UUID, body: str) -> subs.Event:
        body = body.strip()
        if not body:
            raise ValidationError([Problem("body", "comment cannot be empty")])
        if len(body) > MAX_COMMENT_CHARS:
            raise ValidationError([Problem("body", "comment is too long (max 10000 characters)")])
        async with self.db.transaction() as s:
            await subs.get(s, p.org_id, sub_id)
            actor = subs.actor_from_principal(p)
            return await subs.insert_event(
                s,
                subs.Event(
                    submission_id=sub_id,
                    org_id=p.org_id,
                    type=subs.EVENT_COMMENT,
                    actor_type=actor.type,
                    actor_id=actor.id,
                    actor_name=actor.name,
                    payload={"body": body},
                ),
            )

    async def assign(self, p: Principal, sub_id: uuid.UUID, user_id: uuid.UUID | None) -> subs.Submission:
        """Set or clear (``user_id`` None) the assignee. Re-assigning the same user is a no-op."""
        async with self.db.transaction() as s:
            sub = await subs.get(s, p.org_id, sub_id, for_update=True)
            payload: dict[str, Any] = {"assigneeId": None, "assigneeName": ""}
            if user_id is not None:
                name = (
                    await s.execute(
                        sa.text("SELECT name FROM users WHERE org_id = :org AND id = :id"),
                        {"org": p.org_id, "id": user_id},
                    )
                ).scalar()
                if name is None:
                    raise ValidationError([Problem("userId", "unknown user")])
                payload = {"assigneeId": str(user_id), "assigneeName": name}
            if sub.assignee_id != user_id:
                await subs.set_assignee(s, p.org_id, sub_id, user_id)
                actor = subs.actor_from_principal(p)
                await subs.insert_event(
                    s,
                    subs.Event(
                        submission_id=sub_id,
                        org_id=p.org_id,
                        type=subs.EVENT_ASSIGNED,
                        actor_type=actor.type,
                        actor_id=actor.id,
                        actor_name=actor.name,
                        payload=payload,
                    ),
                )
        async with self.db.session() as s:
            return await subs.get(s, p.org_id, sub_id)

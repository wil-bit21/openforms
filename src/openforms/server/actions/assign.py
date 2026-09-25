"""The assign action: a named user, or the least-loaded user with a role."""

from __future__ import annotations

import uuid
from typing import TYPE_CHECKING

import sqlalchemy as sa

from ..exceptions import NotFound
from ..models import auth as auth_models
from ..models import definitions as defs
from ..models import submissions as subs
from ..schemas.core import ROLE_ADMIN, User
from ..services.jobs import Job, Permanent
from .payload import SYSTEM_ACTOR_NAME

if TYPE_CHECKING:
    from sqlalchemy.ext.asyncio import AsyncSession

    from .runner import Runner


async def run_assign(r: Runner, job: Job) -> None:
    ac = await r.load(job)
    a = ac.payload.action
    async with r.d.db.session() as s:
        if a.user:
            try:
                target = await auth_models.get_user_by_email(s, job.org_id, a.user)
            except NotFound:
                raise Permanent(ValueError(f'assign: no user with email "{a.user}"')) from None
        elif a.role:
            target = await least_loaded(s, job.org_id, a.role)
        else:
            raise Permanent(ValueError("assign: action needs user or role"))
    async with r.d.db.transaction() as s:
        await subs.set_assignee(s, job.org_id, ac.sub.id, target.id)
        await subs.insert_event(
            s,
            subs.Event(
                submission_id=ac.sub.id,
                org_id=job.org_id,
                type=subs.EVENT_ASSIGNED,
                actor_type="system",
                actor_name=SYSTEM_ACTOR_NAME,
                payload={"assigneeId": str(target.id), "assigneeName": target.name, "trigger": ac.payload.trigger},
            ),
        )
    await r.record_success(ac)


async def least_loaded(s: AsyncSession, org_id: uuid.UUID, role: str) -> User:
    """Among users with ``role`` (or admins if none), the one with the fewest assigned
    non-terminal submissions; ties go to the earliest created."""
    cands = await auth_models.users_with_role(s, org_id, role) or await auth_models.users_with_role(
        s, org_id, ROLE_ADMIN
    )
    if not cands:
        raise Permanent(ValueError(f'assign: no users with role "{role}" or admin'))
    rows = await s.execute(
        sa.text("""SELECT assignee_id, workflow_version_id, state FROM submissions
        WHERE org_id = :org AND assignee_id = ANY(:ids)"""),
        {"org": org_id, "ids": [c.id for c in cands]},
    )
    load: dict[uuid.UUID, int] = {}
    for assignee, wv, state in rows:
        if not await _is_terminal(s, wv, state):
            load[assignee] = load.get(assignee, 0) + 1
    order = {c.id: i for i, c in enumerate(cands)}  # already ordered by created_at, id
    return min(cands, key=lambda c: (load.get(c.id, 0), c.created_at, order[c.id]))


async def _is_terminal(s: AsyncSession, workflow_version_id: uuid.UUID | None, state: str) -> bool:
    if workflow_version_id is None:
        return True  # forms without a workflow end in "submitted", which is terminal
    st = (await defs.get_workflow_version(s, workflow_version_id)).definition.state(state)
    return st is not None and st.terminal

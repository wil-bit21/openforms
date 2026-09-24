"""Action job handlers and the success/failure events they record."""

from __future__ import annotations

import logging
from dataclasses import dataclass, field
from typing import Any

import httpx
import sqlalchemy as sa

from ...definition import Form, Transition
from ...settings import Settings
from ..database.engine import Database
from ..exceptions import NotFound
from ..models import definitions as defs
from ..models import submissions as subs
from ..services.jobs import Job, Permanent, Queue
from .mailer import Mailer, new_mailer
from .payload import KIND_ASSIGN, KIND_EMAIL, KIND_WEBHOOK, SYSTEM_ACTOR_NAME, TRIGGER_SUBMIT, Payload

log = logging.getLogger("openforms.actions")


@dataclass
class ActionDeps:
    settings: Settings
    db: Database
    mailer: Mailer | None = None
    http_client: httpx.AsyncClient | None = None


@dataclass
class ActionContext:
    job: Job
    payload: Payload
    sub: subs.Submission
    form: Form
    transition: Transition | None = None
    from_state: str = ""
    vars: dict[str, Any] = field(default_factory=dict)


def decode_payload(job: Job) -> Payload:
    try:
        return Payload.from_dict(job.payload)
    except (KeyError, ValueError, TypeError) as e:
        raise Permanent(ValueError(f"decode action payload: {e}")) from None


class Runner:
    def __init__(self, d: ActionDeps) -> None:
        self.d = d
        self.mailer: Mailer = d.mailer or new_mailer(d.settings)
        self.http = d.http_client or httpx.AsyncClient(timeout=10.0)

    async def load(self, job: Job) -> ActionContext:
        from .vars import template_vars

        p = decode_payload(job)
        async with self.d.db.session() as s:
            try:
                sub = await subs.get(s, job.org_id, p.submission_id)
            except NotFound as e:
                raise Permanent(e) from None
            form = (await defs.get_form_version(s, sub.form_version_id)).definition
            ac = ActionContext(job=job, payload=p, sub=sub, form=form)
            if p.trigger != TRIGGER_SUBMIT and sub.workflow_version_id is not None:
                wf = (await defs.get_workflow_version(s, sub.workflow_version_id)).definition
                ac.transition = wf.transition(p.trigger)
                if p.event_id:
                    frm = (
                        await s.execute(
                            sa.text("SELECT from_state FROM submission_events WHERE id = :id"), {"id": p.event_id}
                        )
                    ).scalar()
                    ac.from_state = frm or ""
        ac.vars = template_vars(self.d.settings, ac.form, sub, ac.transition)
        return ac

    async def record_success(self, ac: ActionContext) -> None:
        async with self.d.db.transaction() as s:
            await subs.insert_event(
                s,
                subs.Event(
                    submission_id=ac.sub.id,
                    org_id=ac.sub.org_id,
                    type=subs.EVENT_ACTION_SUCCEEDED,
                    actor_type="system",
                    actor_name=SYSTEM_ACTOR_NAME,
                    payload={"action": ac.payload.action.to_dict(), "trigger": ac.payload.trigger, "jobId": ac.job.id},
                ),
            )

    async def on_failed(self, job: Job, err: BaseException) -> None:
        """Write an ``action_failed`` event when an action job ends failed."""
        if not job.kind.startswith("action."):
            return
        try:
            p = decode_payload(job)
        except Permanent:
            log.error("actions: failed job %s has an undecodable payload", job.id)
            return
        async with self.d.db.transaction() as s:
            await subs.insert_event(
                s,
                subs.Event(
                    submission_id=p.submission_id,
                    org_id=job.org_id,
                    type=subs.EVENT_ACTION_FAILED,
                    actor_type="system",
                    actor_name=SYSTEM_ACTOR_NAME,
                    payload={"error": str(err), "action": p.action.to_dict(), "trigger": p.trigger, "jobId": job.id},
                ),
            )

    async def webhook(self, job: Job) -> None:
        from .webhook import run_webhook

        await run_webhook(self, job)

    async def email(self, job: Job) -> None:
        from .email import run_email

        await run_email(self, job)

    async def assign(self, job: Job) -> None:
        from .assign import run_assign

        await run_assign(self, job)


def register(queue: Queue, d: ActionDeps) -> Runner:
    """Install the action job handlers and the failure hook on ``queue``."""
    r = Runner(d)
    queue.register(KIND_WEBHOOK, r.webhook)
    queue.register(KIND_EMAIL, r.email)
    queue.register(KIND_ASSIGN, r.assign)
    queue.set_failed_hook(r.on_failed)
    return r

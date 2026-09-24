"""Port of the Go ``testutil/fixture``: a hiring workflow, a form that uses it ("apply")
and a form without a workflow ("feedback"), shared by the actions, workflow and API tests."""

from __future__ import annotations

import uuid

from openforms.definition import parse_form, parse_workflow
from openforms.server.models import auth
from openforms.server.models import definitions as defs
from openforms.server.models import submissions as subs
from openforms.server.schemas.core import PRINCIPAL_USER, Principal
from openforms.server.services.jobs import Queue

WORKFLOW_YAML = """
slug: hiring
title: Hiring
initial: new
states:
  - { key: new, label: New }
  - { key: screening, label: Screening }
  - { key: interview, label: Interview }
  - { key: hired, label: Hired, terminal: true }
  - { key: rejected, label: Rejected, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
  - key: priority
    type: select
    label: Priority
    options:
      - { value: low, label: Low }
      - { value: high, label: High }
onSubmit:
  - { type: email, to: "{{submission.data.email}}", subject: "Thanks {{submission.data.name}}", body: "We received your application." }
transitions:
  - { key: screen, label: Start screening, from: [new], to: screening, guard: { roles: [reviewer] } }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "http://127.0.0.1:9/hook" }
  - { key: hire, label: Hire, from: [interview], to: hired, guard: { roles: [hiring-manager] } }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
      - { type: assign, role: hiring-manager }
"""

FORM_YAML = """
slug: apply
title: Apply
workflow: hiring
settings: { public: true }
fields:
  - { key: name, type: text, label: Name, required: true }
  - { key: email, type: email, label: Email, required: true }
"""

PLAIN_FORM_YAML = """
slug: feedback
title: Feedback
settings: { public: true }
fields:
  - { key: message, type: textarea, label: Message, required: true }
"""


class Fixture:
    def __init__(self, database, org_id, settings):
        self.db, self.org_id, self.settings = database, org_id, settings
        self.queue = Queue(database)

    async def setup(self) -> Fixture:
        async with self.db.transaction() as s:
            await defs.apply(
                s,
                self.org_id,
                defs.ApplyInput(
                    forms=[parse_form(FORM_YAML), parse_form(PLAIN_FORM_YAML)],
                    workflows=[parse_workflow(WORKFLOW_YAML)],
                    source="api",
                    actor="fixture",
                ),
            )
        return self

    async def submit(self, form_slug, data, on_created=None, principal=None) -> subs.Submission:
        async with self.db.transaction() as s:
            sub, _ = await subs.create(
                s, self.org_id, form_slug, data, require_public=False, on_created=on_created, principal=principal
            )
        return sub

    async def applicant(self, on_created=None) -> subs.Submission:
        """A submission on the "apply" form in state "new"."""
        return await self.submit("apply", {"name": "Ada", "email": "ada@example.com"}, on_created=on_created)

    async def user(self, email, *roles):
        async with self.db.transaction() as s:
            return await auth.create_user(s, self.org_id, email, email, "password123", list(roles))

    def principal(self, u) -> Principal:
        return Principal(
            org_id=self.org_id, kind=PRINCIPAL_USER, id=u.id, name=u.name, email=u.email, roles=tuple(u.roles)
        )

    async def principal_with_roles(self, *roles) -> Principal:
        return self.principal(await self.user(f"{uuid.uuid4()}@example.com", *roles))

    async def jobs(self):
        """Every job of the org, oldest first."""
        return list(reversed(await self.queue.list(self.org_id, "", 1000)))

    async def events(self, sub_id):
        async with self.db.session() as s:
            return await subs.events(s, self.org_id, sub_id)

    async def events_of_type(self, sub_id, typ):
        return [e for e in await self.events(sub_id) if e.type == typ]

    async def reload(self, sub_id) -> subs.Submission:
        async with self.db.session() as s:
            return await subs.get(s, self.org_id, sub_id)

    async def job(self, job_id):
        return next(j for j in await self.jobs() if j.id == job_id)

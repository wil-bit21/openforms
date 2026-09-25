"""The email action: render recipient/subject/body and send."""

from __future__ import annotations

from typing import TYPE_CHECKING

from ...definition.common import is_bare_email
from ..services.jobs import Job, Permanent
from .render import render

if TYPE_CHECKING:
    from .runner import Runner


async def run_email(r: Runner, job: Job) -> None:
    ac = await r.load(job)
    a = ac.payload.action
    to = render(a.to, ac.vars).strip()
    if not to:
        raise Permanent(ValueError(f"email: recipient template {a.to!r} rendered empty"))
    if "\r" in to or "\n" in to or not is_bare_email(to):
        raise Permanent(ValueError(f"email: invalid recipient {to!r}"))
    try:
        await r.mailer.send(to, render(a.subject, ac.vars), render(a.body, ac.vars))
    except Exception as e:
        raise RuntimeError(f"email: {e}") from e
    await r.record_success(ac)

"""Webhook delivery: signed JSON POST, 10 s timeout; 4xx (except 408/429) is permanent."""

from __future__ import annotations

import hashlib
import hmac
import json
from typing import TYPE_CHECKING, Any

import httpx

from ...definition.common import is_http_url
from ..api.serialize import ts
from ..models.submissions import Submission
from ..services.jobs import Job, Permanent
from .payload import TRIGGER_SUBMIT

if TYPE_CHECKING:
    from .runner import Runner


def sign(secret: str, body: bytes) -> str:
    """``sha256=<hex HMAC-SHA256(secret, body)>``."""
    return "sha256=" + hmac.new(secret.encode(), body, hashlib.sha256).hexdigest()


def webhook_submission(s: Submission) -> dict[str, Any]:
    return {
        "id": str(s.id),
        "form": s.form_slug,
        "formVersion": s.form_version,
        "state": s.state,
        "stateLabel": s.state_label,
        "terminal": s.terminal,
        "data": s.data or {},
        "fields": s.fields or {},
        "assignee": None
        if s.assignee_id is None
        else {"id": str(s.assignee_id), "name": s.assignee_name, "email": s.assignee_email},
        "createdAt": ts(s.created_at),
        "updatedAt": ts(s.updated_at),
    }


async def run_webhook(r: Runner, job: Job) -> None:
    ac = await r.load(job)
    target = ac.payload.action.url
    if not is_http_url(target):
        raise Permanent(ValueError(f"webhook: invalid url {target!r}"))
    body: dict[str, Any] = {
        "event": "submission.transitioned",
        "submission": webhook_submission(ac.sub),
        "transition": None,
        "form": {"slug": ac.form.slug, "title": ac.form.title},
    }
    if ac.payload.trigger == TRIGGER_SUBMIT:
        body["event"] = "submission.created"
    elif ac.transition is not None:
        body["transition"] = {
            "key": ac.transition.key,
            "label": ac.transition.label,
            "from": ac.from_state,
            "to": ac.transition.to,
        }
    raw = json.dumps(body, separators=(",", ":"), ensure_ascii=False).encode()
    headers = {
        "Content-Type": "application/json",
        "User-Agent": "openforms-webhook/1",
        "X-OpenForms-Event": body["event"],
        "X-OpenForms-Delivery": str(job.id),
    }
    if r.d.settings.webhook_secret:
        headers["X-OpenForms-Signature"] = sign(r.d.settings.webhook_secret, raw)
    try:
        resp = await r.http.post(target, content=raw, headers=headers, timeout=10.0)
    except httpx.HTTPError as e:
        raise RuntimeError(f"webhook {target}: {e!r}") from e
    if not 200 <= resp.status_code <= 299:
        err = RuntimeError(f"webhook {target} responded {resp.status_code}")
        if 400 <= resp.status_code < 500 and resp.status_code not in (408, 429):
            raise Permanent(err)
        raise err
    await r.record_success(ac)

"""Action job payloads and enqueueing."""

from __future__ import annotations

import uuid
from dataclasses import dataclass
from typing import Any

from sqlalchemy.ext.asyncio import AsyncSession

from ...definition import Action
from ..services.jobs import Queue

KIND_WEBHOOK = "action.webhook"
KIND_EMAIL = "action.email"
KIND_ASSIGN = "action.assign"
TRIGGER_SUBMIT = "submit"  # otherwise the trigger is the transition key
SYSTEM_ACTOR_NAME = "openforms"  # actor_name on events written by actions

_KINDS = {"webhook": KIND_WEBHOOK, "email": KIND_EMAIL, "assign": KIND_ASSIGN}


@dataclass
class Payload:
    submission_id: uuid.UUID
    trigger: str
    event_id: int
    action: Action

    def to_dict(self) -> dict[str, Any]:
        return {
            "submissionId": str(self.submission_id),
            "trigger": self.trigger,
            "eventId": self.event_id,
            "action": self.action.to_dict(),
        }

    @classmethod
    def from_dict(cls, d: Any) -> Payload:
        if not isinstance(d, dict):
            raise ValueError("payload is not an object")
        return cls(
            submission_id=uuid.UUID(str(d["submissionId"])),
            trigger=str(d.get("trigger", "")),
            event_id=int(d.get("eventId") or 0),
            action=Action.from_dict(d.get("action") or {}),
        )


def kind_for(action_type: str) -> str:
    try:
        return _KINDS[action_type]
    except KeyError:
        raise ValueError(f'actions: unknown action type "{action_type}"') from None


async def enqueue(queue: Queue, session: AsyncSession, org_id: uuid.UUID, p: Payload) -> None:
    """Schedule ``p`` as a job inside the caller's transaction (transactional outbox)."""
    await queue.enqueue(session, org_id, kind_for(p.action.type), p.to_dict())

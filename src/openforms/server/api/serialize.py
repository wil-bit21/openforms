"""JSON shapes shared across routers (spec §7.3). Timestamps are RFC 3339 UTC
with Go's formatting (``2026-09-24T13:42:01.123456Z``, trailing zeros trimmed)."""

from __future__ import annotations

import datetime as dt
from typing import Any

from ..schemas.core import ApiKey, Principal, User


def ts(t: dt.datetime | None) -> str | None:
    if t is None:
        return None
    if t.tzinfo is None:
        t = t.replace(tzinfo=dt.UTC)
    t = t.astimezone(dt.UTC)
    out = t.strftime("%Y-%m-%dT%H:%M:%S")
    if t.microsecond:
        out += ("." + f"{t.microsecond:06d}").rstrip("0")
    return out + "Z"


def user_json(u: User) -> dict[str, Any]:
    return {"id": str(u.id), "email": u.email, "name": u.name, "roles": list(u.roles), "createdAt": ts(u.created_at)}


def principal_json(p: Principal) -> dict[str, Any]:
    return {"kind": p.kind, "id": str(p.id), "name": p.name, "email": p.email, "roles": list(p.roles)}


def api_key_json(k: ApiKey) -> dict[str, Any]:
    return {
        "id": str(k.id),
        "name": k.name,
        "prefix": k.prefix,
        "roles": list(k.roles),
        "createdAt": ts(k.created_at),
        "lastUsedAt": ts(k.last_used_at),
        "revokedAt": ts(k.revoked_at),
    }


def submission_json(s) -> dict[str, Any]:
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


def event_json(e) -> dict[str, Any]:
    return {
        "id": e.id,
        "type": e.type,
        "fromState": e.from_state or None,
        "toState": e.to_state or None,
        "transition": e.transition or None,
        "actor": {"type": e.actor_type, "id": None if e.actor_id is None else str(e.actor_id), "name": e.actor_name},
        "payload": e.payload or {},
        "createdAt": ts(e.created_at),
    }


def version_json(v) -> dict[str, Any]:
    return {
        "version": v.version,
        "hash": v.hash,
        "source": v.source,
        "createdBy": v.created_by,
        "createdAt": ts(v.created_at),
    }


def list_json(items: list[Any], next_cursor: str) -> dict[str, Any]:
    return {"items": items, "nextCursor": next_cursor or None}

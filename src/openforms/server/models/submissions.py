"""Submissions and their event history (port of the Go ``internal/submissions``)."""

from __future__ import annotations

import base64
import datetime as dt
import hashlib
import hmac
import json
import secrets
import uuid
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from typing import Any

import sqlalchemy as sa
from sqlalchemy.ext.asyncio import AsyncSession

from ...definition import State, Workflow, validate_submission
from ...definition.values import format_number
from ..exceptions import FormNotPublic, InvalidCursor, NotFound
from ..schemas.core import PRINCIPAL_API_KEY, Principal
from . import definitions as defs

NO_WORKFLOW_STATE = "submitted"
NO_WORKFLOW_STATE_LABEL = "Submitted"

EVENT_CREATED = "created"
EVENT_TRANSITION = "transition"
EVENT_FIELDS_UPDATED = "fields_updated"
EVENT_ASSIGNED = "assigned"
EVENT_COMMENT = "comment"
EVENT_ACTION_SUCCEEDED = "action_succeeded"
EVENT_ACTION_FAILED = "action_failed"


@dataclass
class Submission:
    id: uuid.UUID
    org_id: uuid.UUID
    form_id: uuid.UUID
    form_version_id: uuid.UUID
    form_slug: str
    form_version: int
    workflow_version_id: uuid.UUID | None
    state: str
    data: dict[str, Any] = field(default_factory=dict)
    fields: dict[str, Any] = field(default_factory=dict)
    state_label: str = ""
    terminal: bool = False
    assignee_id: uuid.UUID | None = None
    assignee_name: str = ""
    assignee_email: str = ""
    created_at: dt.datetime | None = None
    updated_at: dt.datetime | None = None


@dataclass
class Actor:
    type: str  # user | api_key | system | respondent
    id: uuid.UUID | None = None
    name: str = ""


RESPONDENT = Actor(type="respondent", name="Respondent")


def actor_from_principal(p: Principal | None) -> Actor:
    if p is None:
        return RESPONDENT
    return Actor(type="api_key" if p.kind == PRINCIPAL_API_KEY else "user", id=p.id, name=p.name or p.email)


@dataclass
class Event:
    submission_id: uuid.UUID
    org_id: uuid.UUID
    type: str
    from_state: str = ""
    to_state: str = ""
    transition: str = ""
    actor_type: str = ""
    actor_id: uuid.UUID | None = None
    actor_name: str = ""
    payload: dict[str, Any] = field(default_factory=dict)
    id: int = 0
    created_at: dt.datetime | None = None


# (session, submission, workflow or None) — runs inside the insert transaction.
CreatedHook = Callable[[AsyncSession, Submission, Workflow | None], Awaitable[None]]

_SUBMISSION_COLS = """SELECT s.id, s.org_id, s.form_id, s.form_version_id, f.slug AS form_slug, fv.version AS form_version,
    s.workflow_version_id, s.state, s.data, s.fields, s.assignee_id,
    COALESCE(u.name, '') AS assignee_name, COALESCE(u.email, '') AS assignee_email, s.created_at, s.updated_at
FROM submissions s
JOIN forms f ON f.id = s.form_id
JOIN form_versions fv ON fv.id = s.form_version_id
LEFT JOIN users u ON u.id = s.assignee_id"""


def _row(r: Any) -> Submission:
    return Submission(
        id=r.id,
        org_id=r.org_id,
        form_id=r.form_id,
        form_version_id=r.form_version_id,
        form_slug=r.form_slug,
        form_version=r.form_version,
        workflow_version_id=r.workflow_version_id,
        state=r.state,
        data=r.data or {},
        fields=r.fields or {},
        assignee_id=r.assignee_id,
        assignee_name=r.assignee_name,
        assignee_email=r.assignee_email,
        created_at=r.created_at,
        updated_at=r.updated_at,
    )


def state_label(wf: Workflow | None, state: str) -> tuple[str, bool]:
    """Label and terminal flag of ``state`` in ``wf`` (None = no workflow)."""
    if wf is None:
        return NO_WORKFLOW_STATE_LABEL, True
    st = wf.state(state)
    if st is None:
        return state, False
    return st.label, st.terminal


async def resolve_state(session: AsyncSession, sub: Submission) -> None:
    """Fill ``state_label``/``terminal`` from the pinned workflow version."""
    if sub.workflow_version_id is None:
        sub.state_label, sub.terminal = NO_WORKFLOW_STATE_LABEL, True
        return
    wf = await defs.get_workflow_version(session, sub.workflow_version_id)
    sub.state_label, sub.terminal = state_label(wf.definition, sub.state)


async def get(session: AsyncSession, org_id: uuid.UUID, sub_id: uuid.UUID, *, for_update: bool = False) -> Submission:
    """``for_update`` locks the submission row for the rest of the transaction."""
    sql = _SUBMISSION_COLS + " WHERE s.org_id = :org AND s.id = :id" + (" FOR UPDATE OF s" if for_update else "")
    r = (await session.execute(sa.text(sql), {"org": org_id, "id": sub_id})).first()
    if r is None:
        raise NotFound()
    sub = _row(r)
    await resolve_state(session, sub)
    return sub


def hash_token(token: str) -> str:
    return hashlib.sha256(token.encode()).hexdigest()


def new_receipt_token() -> tuple[str, str]:
    """A 24-byte base64url token and its sha256 hex digest."""
    token = base64.urlsafe_b64encode(secrets.token_bytes(24)).rstrip(b"=").decode()
    return token, hash_token(token)


async def insert_event(session: AsyncSession, e: Event) -> Event:
    """Append an event; returns it with ``id`` and ``created_at`` set."""
    row = (
        await session.execute(
            sa.text("""INSERT INTO submission_events
        (submission_id, org_id, type, from_state, to_state, transition, actor_type, actor_id, actor_name, payload)
        VALUES (:sid, :org, :type, :from_state, :to_state, :transition, :actor_type, :actor_id, :actor_name, CAST(:payload AS jsonb))
        RETURNING id, created_at"""),
            {
                "sid": e.submission_id,
                "org": e.org_id,
                "type": e.type,
                "from_state": e.from_state or None,
                "to_state": e.to_state or None,
                "transition": e.transition or None,
                "actor_type": e.actor_type,
                "actor_id": e.actor_id,
                "actor_name": e.actor_name,
                "payload": json.dumps(e.payload or {}),
            },
        )
    ).one()
    e.id, e.created_at = row.id, row.created_at
    return e


async def create(
    session: AsyncSession,
    org_id: uuid.UUID,
    form_slug: str,
    data: dict[str, Any] | None,
    *,
    require_public: bool,
    principal: Principal | None = None,
    on_created: CreatedHook | None = None,
) -> tuple[Submission, str]:
    """Validate ``data`` against the form's current version and store it in the workflow's
    initial state ("submitted" without a workflow). Runs in the caller's transaction, so a
    failing ``on_created`` hook rolls the submission back. Returns the submission and its
    plaintext receipt token (shown once)."""
    form = await defs.get_form(session, org_id, form_slug)
    if require_public and not form.definition.settings.public:
        raise FormNotPublic()
    clean = validate_submission(form.definition, data or {})
    wf: Workflow | None = None
    state = NO_WORKFLOW_STATE
    if form.workflow_version_id is not None:
        wf = (await defs.get_workflow_version(session, form.workflow_version_id)).definition
        state = wf.initial
    sub = Submission(
        id=uuid.uuid4(),
        org_id=org_id,
        form_id=form.id,
        form_version_id=form.current.id,
        form_slug=form.slug,
        form_version=form.current.version,
        workflow_version_id=form.workflow_version_id,
        state=state,
        data=clean,
        fields={},
    )
    sub.state_label, sub.terminal = state_label(wf, state)
    token, token_hash = new_receipt_token()
    actor = actor_from_principal(principal)
    row = (
        await session.execute(
            sa.text("""INSERT INTO submissions
        (id, org_id, form_id, form_version_id, workflow_version_id, state, data, fields, receipt_token_hash)
        VALUES (:id, :org, :fid, :fvid, :wvid, :state, CAST(:data AS jsonb), '{}', :hash)
        RETURNING created_at, updated_at"""),
            {
                "id": sub.id,
                "org": org_id,
                "fid": sub.form_id,
                "fvid": sub.form_version_id,
                "wvid": sub.workflow_version_id,
                "state": state,
                "data": json.dumps(clean),
                "hash": token_hash,
            },
        )
    ).one()
    sub.created_at, sub.updated_at = row.created_at, row.updated_at
    await insert_event(
        session,
        Event(
            submission_id=sub.id,
            org_id=org_id,
            type=EVENT_CREATED,
            to_state=state,
            actor_type=actor.type,
            actor_id=actor.id,
            actor_name=actor.name,
        ),
    )
    if on_created is not None:
        await on_created(session, sub, wf)
    return sub, token


async def events(session: AsyncSession, org_id: uuid.UUID, sub_id: uuid.UUID) -> list[Event]:
    """The submission's events, oldest first."""
    exists = (
        await session.execute(
            sa.text("SELECT EXISTS (SELECT 1 FROM submissions WHERE org_id = :org AND id = :id)"),
            {"org": org_id, "id": sub_id},
        )
    ).scalar()
    if not exists:
        raise NotFound()
    rows = await session.execute(
        sa.text("""SELECT id, submission_id, org_id, type,
        COALESCE(from_state, '') AS from_state, COALESCE(to_state, '') AS to_state, COALESCE(transition, '') AS transition,
        actor_type, actor_id, actor_name, payload, created_at
        FROM submission_events WHERE submission_id = :id ORDER BY id"""),
        {"id": sub_id},
    )
    return [
        Event(
            id=r.id,
            submission_id=r.submission_id,
            org_id=r.org_id,
            type=r.type,
            from_state=r.from_state,
            to_state=r.to_state,
            transition=r.transition,
            actor_type=r.actor_type,
            actor_id=r.actor_id,
            actor_name=r.actor_name,
            payload=r.payload or {},
            created_at=r.created_at,
        )
        for r in rows
    ]


async def set_assignee(session: AsyncSession, org_id: uuid.UUID, sub_id: uuid.UUID, user_id: uuid.UUID | None) -> None:
    """Set (or clear) the assignee. Writes no event; callers record "assigned" themselves."""
    res = await session.execute(
        sa.text("UPDATE submissions SET assignee_id = :uid, updated_at = now() WHERE org_id = :org AND id = :id"),
        {"uid": user_id, "org": org_id, "id": sub_id},
    )
    if res.rowcount == 0:  # type: ignore[attr-defined]
        raise NotFound()


# --- listing ------------------------------------------------------------------


@dataclass
class ListFilter:
    form_slug: str = ""
    state: str = ""
    assignee_id: uuid.UUID | None = None
    unassigned: bool = False
    cursor: str = ""
    limit: int = 50  # default 50, max 200


def _rfc3339nano(t: dt.datetime) -> str:
    t = t.astimezone(dt.UTC)
    out = t.strftime("%Y-%m-%dT%H:%M:%S")
    if t.microsecond:
        out += ("." + f"{t.microsecond:06d}").rstrip("0")
    return out + "Z"


def encode_cursor(t: dt.datetime, sub_id: uuid.UUID) -> str:
    return base64.urlsafe_b64encode(f"{_rfc3339nano(t)}|{sub_id}".encode()).rstrip(b"=").decode()


def decode_cursor(c: str) -> tuple[dt.datetime, uuid.UUID]:
    try:
        raw = base64.urlsafe_b64decode(c + "=" * (-len(c) % 4)).decode()
        ts, sep, id_str = raw.partition("|")
        if not sep:
            raise ValueError
        # Go's RFC3339Nano may carry up to 9 fractional digits; Python keeps 6.
        head, dot, frac = ts.partition(".")
        if dot:
            digits = "".join(ch for ch in frac if ch.isdigit())
            tz = frac[len(digits) :]
            ts = f"{head}.{digits[:6].ljust(6, '0')}{tz}"
        t = dt.datetime.fromisoformat(ts.replace("Z", "+00:00"))
        if t.tzinfo is None:
            raise ValueError
        return t, uuid.UUID(id_str)
    except (ValueError, UnicodeDecodeError):
        raise InvalidCursor() from None


async def list_submissions(session: AsyncSession, org_id: uuid.UUID, f: ListFilter) -> tuple[list[Submission], str]:
    """Newest first (created_at DESC, id DESC) plus an opaque cursor for the next page ("" on the last)."""
    limit = 50 if f.limit <= 0 else min(f.limit, 200)
    cur_at, cur_id = decode_cursor(f.cursor) if f.cursor else (None, None)
    rows = await session.execute(
        sa.text(
            _SUBMISSION_COLS
            + """
        WHERE s.org_id = :org
          AND (CAST(:form AS text) = '' OR f.slug = :form)
          AND (CAST(:state AS text) = '' OR s.state = :state)
          AND (CAST(:assignee AS uuid) IS NULL OR s.assignee_id = :assignee)
          AND (NOT CAST(:unassigned AS bool) OR s.assignee_id IS NULL)
          AND (CAST(:cur_at AS timestamptz) IS NULL OR (s.created_at, s.id) < (CAST(:cur_at AS timestamptz), CAST(:cur_id AS uuid)))
        ORDER BY s.created_at DESC, s.id DESC
        LIMIT :lim"""
        ),
        {
            "org": org_id,
            "form": f.form_slug,
            "state": f.state,
            "assignee": f.assignee_id,
            "unassigned": f.unassigned,
            "cur_at": cur_at,
            "cur_id": cur_id,
            "lim": limit + 1,
        },
    )
    items = [_row(r) for r in rows]
    nxt = ""
    if len(items) > limit:
        items = items[:limit]
        last = items[-1]
        assert last.created_at is not None
        nxt = encode_cursor(last.created_at, last.id)
    for it in items:
        await resolve_state(session, it)
    return items, nxt


async def count_by_form(session: AsyncSession, org_id: uuid.UUID) -> dict[uuid.UUID, int]:
    rows = await session.execute(
        sa.text("SELECT form_id, count(*) FROM submissions WHERE org_id = :org GROUP BY form_id"), {"org": org_id}
    )
    return {r[0]: r[1] for r in rows}


# --- public status --------------------------------------------------------------


@dataclass
class PublicHistoryItem:
    state: str
    label: str
    at: dt.datetime


@dataclass
class PublicStatus:
    id: uuid.UUID
    form_title: str
    state: str
    state_label: str
    terminal: bool
    states: list[State]
    history: list[PublicHistoryItem]
    created_at: dt.datetime | None


async def public_status(session: AsyncSession, sub_id: uuid.UUID, token: str) -> PublicStatus:
    """What a respondent may see. Any unknown id or token mismatch is NotFound (constant-time compare)."""
    row = (
        await session.execute(
            sa.text("SELECT org_id, receipt_token_hash FROM submissions WHERE id = :id"), {"id": sub_id}
        )
    ).first()
    if row is None:
        raise NotFound()
    if not token or not hmac.compare_digest(hash_token(token).encode(), row.receipt_token_hash.encode()):
        raise NotFound()
    sub = await get(session, row.org_id, sub_id)
    form = await defs.get_form_version(session, sub.form_version_id)
    wf: Workflow | None = None
    states = [State(NO_WORKFLOW_STATE, NO_WORKFLOW_STATE_LABEL, terminal=True)]
    if sub.workflow_version_id is not None:
        wf = (await defs.get_workflow_version(session, sub.workflow_version_id)).definition
        states = list(wf.states or [])
    rows = await session.execute(
        sa.text("""SELECT to_state, created_at FROM submission_events
        WHERE submission_id = :id AND to_state IS NOT NULL ORDER BY id"""),
        {"id": sub_id},
    )
    history = [PublicHistoryItem(state=r.to_state, label=state_label(wf, r.to_state)[0], at=r.created_at) for r in rows]
    return PublicStatus(
        id=sub.id,
        form_title=form.definition.title,
        state=sub.state,
        state_label=sub.state_label,
        terminal=sub.terminal,
        states=states,
        history=history,
        created_at=sub.created_at,
    )


# --- CSV export -----------------------------------------------------------------


def csv_value(v: Any) -> str:
    if v is None:
        return ""
    if isinstance(v, str):
        return v
    if isinstance(v, bool):
        return "true" if v else "false"
    if isinstance(v, (int, float)):
        return format_number(v)
    if isinstance(v, list):
        return ";".join(csv_value(p) for p in v)
    return json.dumps(v, sort_keys=True, separators=(",", ":"), ensure_ascii=False)


def csv_safe(s: str) -> str:
    """Neutralise spreadsheet formula injection (OWASP CSV injection)."""
    return "'" + s if s and s[0] in "=+-@\t\r" else s


def _csv_field(s: str) -> str:
    """Go ``encoding/csv`` quoting."""
    needs = s == r"\." or any(c in s for c in ',"\r\n') or (s != "" and s[0] in " \t\n\v\f\r\x85\xa0")
    return '"' + s.replace('"', '""') + '"' if needs else s


def _csv_line(record: list[str]) -> str:
    return ",".join(_csv_field(f) for f in record) + "\n"


async def export_csv(session: AsyncSession, org_id: uuid.UUID, form_slug: str) -> str:
    """Every submission of the form, oldest first. Columns come from the form's current
    version: id, createdAt, state, formVersion, each form field key, then ``fields.<key>``."""
    form = await defs.get_form(session, org_id, form_slug)
    data_keys = [f.key for f in form.definition.field_list()]
    field_keys: list[str] = []
    if form.workflow_version_id is not None:
        wf = await defs.get_workflow_version(session, form.workflow_version_id)
        field_keys = [f.key for f in wf.definition.fields]
    out = [_csv_line(["id", "createdAt", "state", "formVersion", *data_keys, *("fields." + k for k in field_keys)])]
    rows = await session.execute(
        sa.text("""SELECT s.id, s.created_at, s.state, fv.version, s.data, s.fields
        FROM submissions s JOIN form_versions fv ON fv.id = s.form_version_id
        WHERE s.org_id = :org AND s.form_id = :fid ORDER BY s.created_at, s.id"""),
        {"org": org_id, "fid": form.id},
    )
    for r in rows:
        created = r.created_at.astimezone(dt.UTC).strftime("%Y-%m-%dT%H:%M:%SZ")
        data, fields = r.data or {}, r.fields or {}
        record = [str(r.id), created, r.state, str(r.version)]
        record += [csv_safe(csv_value(data.get(k))) for k in data_keys]
        record += [csv_safe(csv_value(fields.get(k))) for k in field_keys]
        out.append(_csv_line(record))
    return "".join(out)

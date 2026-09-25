"""Versioned form and workflow definitions (port of the Go ``internal/definitions``).

Versions are immutable and hash-deduplicated; applying a new workflow version
re-pins every form that references it (spec §5.5). Records looked up by
version id are cached for the life of the process.
"""

from __future__ import annotations

import datetime as dt
import json
import uuid
from dataclasses import dataclass, field
from typing import Any

import sqlalchemy as sa
from sqlalchemy.ext.asyncio import AsyncSession

from ...definition import (
    Form,
    Problem,
    ValidationError,
    Workflow,
    canonical,
    validate_bundle,
    validate_form,
    validate_workflow,
)
from ..exceptions import NotFound

SOURCE_CLI, SOURCE_UI, SOURCE_API, SOURCE_SEED = "cli", "ui", "api", "seed"
SOURCES = (SOURCE_CLI, SOURCE_UI, SOURCE_API, SOURCE_SEED)


def parse_source(s: str) -> str | None:
    return s if s in SOURCES else None


@dataclass
class VersionInfo:
    id: uuid.UUID
    version: int
    hash: str
    source: str
    created_by: str
    created_at: dt.datetime


@dataclass
class FormRecord:
    """A form at one version. For records from version lookups, ``updated_at``
    equals ``current.created_at``."""

    id: uuid.UUID
    slug: str
    current: VersionInfo
    workflow_version_id: uuid.UUID | None
    definition: Form
    created_at: dt.datetime
    updated_at: dt.datetime


@dataclass
class WorkflowRecord:
    id: uuid.UUID
    slug: str
    current: VersionInfo
    definition: Workflow
    created_at: dt.datetime
    updated_at: dt.datetime


@dataclass
class ApplyInput:
    forms: list[Form] = field(default_factory=list)
    workflows: list[Workflow] = field(default_factory=list)
    source: str = SOURCE_API
    actor: str = ""
    dry_run: bool = False


@dataclass
class ApplyItem:
    kind: str
    slug: str
    version: int = 0
    changed: bool = False
    created: bool = False

    def to_dict(self) -> dict[str, Any]:
        return {
            "kind": self.kind,
            "slug": self.slug,
            "version": self.version,
            "changed": self.changed,
            "created": self.created,
        }


@dataclass
class ApplyResult:
    items: list[ApplyItem] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {"items": [i.to_dict() for i in self.items]}


_form_version_cache: dict[uuid.UUID, FormRecord] = {}
_workflow_version_cache: dict[uuid.UUID, WorkflowRecord] = {}

_FORM_COLS = """SELECT f.id AS fid, f.slug, f.created_at AS fcreated, f.updated_at AS fupdated,
    v.id AS vid, v.version, v.hash, v.source, v.created_by, v.created_at AS vcreated, v.workflow_version_id, v.definition
FROM form_versions v JOIN forms f ON f.id = v.form_id"""

_WORKFLOW_COLS = """SELECT w.id AS fid, w.slug, w.created_at AS fcreated, w.updated_at AS fupdated,
    v.id AS vid, v.version, v.hash, v.source, v.created_by, v.created_at AS vcreated, v.definition
FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id"""


def _info(r: Any) -> VersionInfo:
    return VersionInfo(
        id=r.vid, version=r.version, hash=r.hash, source=r.source, created_by=r.created_by, created_at=r.vcreated
    )


def _form(r: Any) -> FormRecord:
    return FormRecord(
        id=r.fid,
        slug=r.slug,
        current=_info(r),
        workflow_version_id=r.workflow_version_id,
        definition=Form.from_dict(r.definition),
        created_at=r.fcreated,
        updated_at=r.fupdated,
    )


def _workflow(r: Any) -> WorkflowRecord:
    return WorkflowRecord(
        id=r.fid,
        slug=r.slug,
        current=_info(r),
        definition=Workflow.from_dict(r.definition),
        created_at=r.fcreated,
        updated_at=r.fupdated,
    )


async def _one(session: AsyncSession, sql: str, **params: Any) -> Any:
    row = (await session.execute(sa.text(sql), params)).first()
    if row is None:
        raise NotFound()
    return row


# --- reads --------------------------------------------------------------------


async def get_form(session: AsyncSession, org_id: uuid.UUID, slug: str) -> FormRecord:
    """The current version of a form."""
    return _form(
        await _one(
            session,
            _FORM_COLS + " WHERE f.org_id = :org AND f.slug = :slug AND f.current_version_id = v.id",
            org=org_id,
            slug=slug,
        )
    )


async def get_workflow(session: AsyncSession, org_id: uuid.UUID, slug: str) -> WorkflowRecord:
    return _workflow(
        await _one(
            session,
            _WORKFLOW_COLS + " WHERE w.org_id = :org AND w.slug = :slug AND w.current_version_id = v.id",
            org=org_id,
            slug=slug,
        )
    )


async def get_form_version(session: AsyncSession, version_id: uuid.UUID) -> FormRecord:
    """A form at a specific version id (cached: versions are immutable)."""
    if (rec := _form_version_cache.get(version_id)) is not None:
        return rec
    rec = _form(await _one(session, _FORM_COLS + " WHERE v.id = :id", id=version_id))
    rec.updated_at = rec.current.created_at
    _form_version_cache[version_id] = rec
    return rec


async def get_workflow_version(session: AsyncSession, version_id: uuid.UUID) -> WorkflowRecord:
    if (rec := _workflow_version_cache.get(version_id)) is not None:
        return rec
    rec = _workflow(await _one(session, _WORKFLOW_COLS + " WHERE v.id = :id", id=version_id))
    rec.updated_at = rec.current.created_at
    _workflow_version_cache[version_id] = rec
    return rec


async def form_version(session: AsyncSession, org_id: uuid.UUID, slug: str, n: int) -> FormRecord:
    rec = _form(
        await _one(
            session,
            _FORM_COLS + " WHERE f.org_id = :org AND f.slug = :slug AND v.version = :n",
            org=org_id,
            slug=slug,
            n=n,
        )
    )
    rec.updated_at = rec.current.created_at
    return rec


async def workflow_version(session: AsyncSession, org_id: uuid.UUID, slug: str, n: int) -> WorkflowRecord:
    rec = _workflow(
        await _one(
            session,
            _WORKFLOW_COLS + " WHERE w.org_id = :org AND w.slug = :slug AND v.version = :n",
            org=org_id,
            slug=slug,
            n=n,
        )
    )
    rec.updated_at = rec.current.created_at
    return rec


async def list_forms(session: AsyncSession, org_id: uuid.UUID) -> list[FormRecord]:
    rows = await session.execute(
        sa.text(_FORM_COLS + " WHERE f.org_id = :org AND f.current_version_id = v.id ORDER BY f.slug"), {"org": org_id}
    )
    return [_form(r) for r in rows]


async def list_workflows(session: AsyncSession, org_id: uuid.UUID) -> list[WorkflowRecord]:
    rows = await session.execute(
        sa.text(_WORKFLOW_COLS + " WHERE w.org_id = :org AND w.current_version_id = v.id ORDER BY w.slug"),
        {"org": org_id},
    )
    return [_workflow(r) for r in rows]


async def _versions(session: AsyncSession, sql: str, **params: Any) -> list[VersionInfo]:
    rows = (await session.execute(sa.text(sql), params)).all()
    if not rows:
        raise NotFound()
    return [
        VersionInfo(
            id=r.id, version=r.version, hash=r.hash, source=r.source, created_by=r.created_by, created_at=r.created_at
        )
        for r in rows
    ]


async def form_versions(session: AsyncSession, org_id: uuid.UUID, slug: str) -> list[VersionInfo]:
    """A form's versions, newest first."""
    return await _versions(
        session,
        """SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
        FROM form_versions v JOIN forms f ON f.id = v.form_id
        WHERE f.org_id = :org AND f.slug = :slug ORDER BY v.version DESC""",
        org=org_id,
        slug=slug,
    )


async def workflow_versions(session: AsyncSession, org_id: uuid.UUID, slug: str) -> list[VersionInfo]:
    return await _versions(
        session,
        """SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
        FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
        WHERE w.org_id = :org AND w.slug = :slug ORDER BY v.version DESC""",
        org=org_id,
        slug=slug,
    )


async def export(session: AsyncSession, org_id: uuid.UUID) -> tuple[list[Form], list[Workflow]]:
    """The org's current definitions, sorted by slug."""
    return (
        [f.definition for f in await list_forms(session, org_id)],
        [w.definition for w in await list_workflows(session, org_id)],
    )


# --- apply ----------------------------------------------------------------------


def prefix_problems(prefix: str, err: BaseException | None) -> list[Problem]:
    """Prefix the problems of ``err`` (``forms[2]`` + ``fields[0].key``)."""
    if err is None:
        return []
    if not isinstance(err, ValidationError):
        return [Problem(prefix, str(err))]
    out = []
    for p in err.problems:
        if p.path == "":
            path = prefix
        elif p.path.startswith("["):
            path = prefix + p.path
        else:
            path = prefix + "." + p.path
        out.append(Problem(path, p.message))
    return out


def _err(fn: Any, arg: Any) -> ValidationError | None:
    try:
        fn(arg)
    except ValidationError as e:
        return e
    return None


async def workflow_slugs(session: AsyncSession, org_id: uuid.UUID) -> list[str]:
    rows = await session.execute(
        sa.text("SELECT slug FROM workflows WHERE org_id = :org AND current_version_id IS NOT NULL ORDER BY slug"),
        {"org": org_id},
    )
    return [r[0] for r in rows]


async def _validate_input(session: AsyncSession, org_id: uuid.UUID, inp: ApplyInput) -> None:
    problems: list[Problem] = []
    seen_wf: set[str] = set()
    for i, wf in enumerate(inp.workflows):
        prefix = f"workflows[{i}]"
        if wf.slug in seen_wf:
            problems.append(Problem(prefix + ".slug", f'workflow "{wf.slug}" appears more than once in the bundle'))
        seen_wf.add(wf.slug)
        problems += prefix_problems(prefix, _err(validate_workflow, wf))
    seen_form: set[str] = set()
    for i, f in enumerate(inp.forms):
        prefix = f"forms[{i}]"
        if f.slug in seen_form:
            problems.append(Problem(prefix + ".slug", f'form "{f.slug}" appears more than once in the bundle'))
        seen_form.add(f.slug)
        problems += prefix_problems(prefix, _err(validate_form, f))
    if problems:
        raise ValidationError(problems)
    validate_bundle(inp.forms, inp.workflows, await workflow_slugs(session, org_id))


def _canon(defn: Form | Workflow) -> tuple[Any, str]:
    raw, h = canonical(defn)
    return json.loads(raw), h


async def _upsert_workflow(
    session: AsyncSession, org_id: uuid.UUID, wf: Workflow, src: str, actor: str
) -> tuple[ApplyItem, uuid.UUID]:
    """INSERT … ON CONFLICT DO NOTHING followed by SELECT … FOR UPDATE makes concurrent
    applies of a new slug serialize instead of failing."""
    doc, h = _canon(wf)
    item = ApplyItem(kind="workflow", slug=wf.slug)
    res = await session.execute(
        sa.text(
            "INSERT INTO workflows (id, org_id, slug) VALUES (:id, :org, :slug) ON CONFLICT (org_id, slug) DO NOTHING"
        ),
        {"id": uuid.uuid4(), "org": org_id, "slug": wf.slug},
    )
    item.created = res.rowcount == 1  # type: ignore[attr-defined]
    row = (
        await session.execute(
            sa.text("""
        SELECT w.id, v.id AS vid, v.version, v.hash
        FROM workflows w LEFT JOIN workflow_versions v ON v.id = w.current_version_id
        WHERE w.org_id = :org AND w.slug = :slug
        FOR UPDATE OF w"""),
            {"org": org_id, "slug": wf.slug},
        )
    ).one()
    if row.hash is not None and row.hash == h:
        item.version = row.version
        return item, row.vid
    nxt = (row.version or 0) + 1
    vid = uuid.uuid4()
    await session.execute(
        sa.text("""INSERT INTO workflow_versions (id, workflow_id, version, definition, hash, source, created_by)
            VALUES (:vid, :wid, :n, CAST(:def AS jsonb), :hash, :src, :actor)"""),
        {"vid": vid, "wid": row.id, "n": nxt, "def": json.dumps(doc), "hash": h, "src": src, "actor": actor},
    )
    await session.execute(
        sa.text("UPDATE workflows SET current_version_id = :vid, updated_at = now() WHERE id = :id"),
        {"vid": vid, "id": row.id},
    )
    item.version, item.changed = nxt, True
    return item, vid


async def _upsert_form(
    session: AsyncSession, org_id: uuid.UUID, f: Form, pin: uuid.UUID | None, src: str, actor: str
) -> ApplyItem:
    doc, h = _canon(f)
    item = ApplyItem(kind="form", slug=f.slug)
    res = await session.execute(
        sa.text("INSERT INTO forms (id, org_id, slug) VALUES (:id, :org, :slug) ON CONFLICT (org_id, slug) DO NOTHING"),
        {"id": uuid.uuid4(), "org": org_id, "slug": f.slug},
    )
    item.created = res.rowcount == 1  # type: ignore[attr-defined]
    row = (
        await session.execute(
            sa.text("""
        SELECT f.id, v.version, v.hash, v.workflow_version_id
        FROM forms f LEFT JOIN form_versions v ON v.id = f.current_version_id
        WHERE f.org_id = :org AND f.slug = :slug
        FOR UPDATE OF f"""),
            {"org": org_id, "slug": f.slug},
        )
    ).one()
    if row.hash is not None and row.hash == h and row.workflow_version_id == pin:
        item.version = row.version
        return item
    nxt = (row.version or 0) + 1
    vid = uuid.uuid4()
    await session.execute(
        sa.text("""INSERT INTO form_versions (id, form_id, version, definition, hash, workflow_version_id, source, created_by)
            VALUES (:vid, :fid, :n, CAST(:def AS jsonb), :hash, :pin, :src, :actor)"""),
        {
            "vid": vid,
            "fid": row.id,
            "n": nxt,
            "def": json.dumps(doc),
            "hash": h,
            "pin": pin,
            "src": src,
            "actor": actor,
        },
    )
    await session.execute(
        sa.text("UPDATE forms SET current_version_id = :vid, updated_at = now() WHERE id = :id"),
        {"vid": vid, "id": row.id},
    )
    item.version, item.changed = nxt, True
    return item


async def _current_workflow_version(session: AsyncSession, org_id: uuid.UUID, slug: str) -> uuid.UUID | None:
    if not slug:
        return None
    vid = (
        await session.execute(
            sa.text("SELECT current_version_id FROM workflows WHERE org_id = :org AND slug = :slug"),
            {"org": org_id, "slug": slug},
        )
    ).scalar()
    if vid is None:
        raise NotFound(f'workflow "{slug}" not found')
    return vid


async def _dependent_forms(session: AsyncSession, org_id: uuid.UUID, workflow_slug: str) -> list[Form]:
    rows = await session.execute(
        sa.text("""
        SELECT v.definition FROM forms f JOIN form_versions v ON v.id = f.current_version_id
        WHERE f.org_id = :org AND v.definition->>'workflow' = :slug ORDER BY f.slug"""),
        {"org": org_id, "slug": workflow_slug},
    )
    return [Form.from_dict(r[0]) for r in rows]


async def apply(session: AsyncSession, org_id: uuid.UUID, inp: ApplyInput) -> ApplyResult:
    """Write a bundle atomically (a savepoint inside the caller's transaction):

    1. validate every document and the bundle (cross-refs may resolve to existing workflows);
    2. upsert workflows, then forms (each form pins its workflow's current version);
    3. re-pin forms outside the bundle whose workflow just got a new version.

    Unchanged definitions create no version. ``dry_run`` reports the would-be result
    and rolls the savepoint back.
    """
    src = inp.source or SOURCE_API
    await _validate_input(session, org_id, inp)
    res = ApplyResult()
    sp = await session.begin_nested()
    try:
        new_versions: dict[str, uuid.UUID] = {}
        for wf in inp.workflows:
            item, vid = await _upsert_workflow(session, org_id, wf, src, inp.actor)
            res.items.append(item)
            if item.changed:
                new_versions[wf.slug] = vid
        in_bundle = set()
        for f in inp.forms:
            in_bundle.add(f.slug)
            pin = await _current_workflow_version(session, org_id, f.workflow)
            res.items.append(await _upsert_form(session, org_id, f, pin, src, inp.actor))
        for wf in inp.workflows:
            vid = new_versions.get(wf.slug)
            if vid is None:
                continue
            for f in await _dependent_forms(session, org_id, wf.slug):
                if f.slug in in_bundle:
                    continue
                item = await _upsert_form(session, org_id, f, vid, src, inp.actor)
                if item.changed:
                    res.items.append(item)
    except BaseException:
        await sp.rollback()
        raise
    if inp.dry_run:
        await sp.rollback()
    else:
        await sp.commit()
    return res

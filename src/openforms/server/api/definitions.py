"""``/forms``, ``/workflows`` and ``/definitions`` (authenticated; writes are admin only)."""

from __future__ import annotations

import json
from typing import Any

from fastapi import APIRouter, Depends
from starlette.requests import Request

from ...definition import Form, Problem, ValidationError, Workflow, parse_form, parse_workflow
from ..models import definitions as defs
from ..models import submissions as subs
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import read_body, read_object, require_admin, require_auth
from .errors import APIError, bad_request, not_found
from .serialize import ts, version_json

router = APIRouter(dependencies=[Depends(require_auth)])


def actor_label(p: Principal) -> str:
    """Recorded as ``created_by`` on definition versions."""
    return p.email or p.name


def source_param(request: Request) -> str:
    """``?source=``; "seed" is reserved for the seeder."""
    q = request.query_params.get("source", "")
    if q == "":
        return defs.SOURCE_API
    if defs.parse_source(q) is None or q == defs.SOURCE_SEED:
        raise bad_request("source must be one of cli, ui, api")
    return q


def version_param(v: str) -> int:
    try:
        n = int(v)
    except ValueError:
        raise not_found() from None
    if n < 1 or not v.isdigit():
        raise not_found()
    return n


async def raw_body(request: Request) -> bytes:
    try:
        return await read_body(request)
    except APIError:
        raise APIError(400, "bad_request", "request body too large or unreadable") from None


def slug_mismatch(body_slug: str, url_slug: str) -> ValidationError:
    return ValidationError([Problem("slug", f'slug "{body_slug}" does not match URL slug "{url_slug}"')])


def find_item(items: list[defs.ApplyItem], kind: str, slug: str) -> dict[str, Any]:
    for it in items:
        if it.kind == kind and it.slug == slug:
            return it.to_dict()
    return defs.ApplyItem(kind, slug).to_dict()


# --- forms --------------------------------------------------------------------


@router.get("/forms")
async def list_forms(request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        forms = await defs.list_forms(s, ctx.org_id)
        counts = await subs.count_by_form(s, ctx.org_id)
    return {
        "items": [
            {
                "slug": f.slug,
                "title": f.definition.title,
                "workflow": f.definition.workflow or None,
                "public": f.definition.settings.public,
                "version": f.current.version,
                "source": f.current.source,
                "updatedAt": ts(f.updated_at),
                "submissionCount": counts.get(f.id, 0),
            }
            for f in forms
        ]
    }


async def form_record_json(session, rec: defs.FormRecord) -> dict[str, Any]:
    wf_version = None
    if rec.workflow_version_id is not None:
        wf_version = (await defs.get_workflow_version(session, rec.workflow_version_id)).current.version
    return {
        "slug": rec.slug,
        "version": rec.current.version,
        "source": rec.current.source,
        "updatedAt": ts(rec.updated_at),
        "workflowVersion": wf_version,
        "definition": rec.definition.to_dict(),
    }


@router.get("/forms/{slug}")
async def get_form(slug: str, request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        return {"form": await form_record_json(s, await defs.get_form(s, ctx.org_id, slug))}


@router.put("/forms/{slug}")
async def put_form(slug: str, request: Request, p: Principal = Depends(require_admin)) -> dict:
    src = source_param(request)
    f = parse_form(await raw_body(request))
    if f.slug != slug:
        raise slug_mismatch(f.slug, slug)
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        res = await defs.apply(s, ctx.org_id, defs.ApplyInput(forms=[f], source=src, actor=actor_label(p)))
    return {"item": find_item(res.items, "form", slug)}


@router.get("/forms/{slug}/versions")
async def form_versions(slug: str, request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        vs = await defs.form_versions(s, ctx.org_id, slug)
    return {"items": [version_json(v) for v in vs]}


@router.get("/forms/{slug}/versions/{version}")
async def form_version(slug: str, version: str, request: Request) -> dict:
    n = version_param(version)
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        return {"form": await form_record_json(s, await defs.form_version(s, ctx.org_id, slug, n))}


# --- workflows ------------------------------------------------------------------


def workflow_record_json(rec: defs.WorkflowRecord) -> dict[str, Any]:
    return {
        "slug": rec.slug,
        "version": rec.current.version,
        "source": rec.current.source,
        "updatedAt": ts(rec.updated_at),
        "definition": rec.definition.to_dict(),
    }


@router.get("/workflows")
async def list_workflows(request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        wfs = await defs.list_workflows(s, ctx.org_id)
    return {
        "items": [
            {
                "slug": w.slug,
                "title": w.definition.title,
                "version": w.current.version,
                "source": w.current.source,
                "updatedAt": ts(w.updated_at),
                "stateCount": len(w.definition.states or []),
            }
            for w in wfs
        ]
    }


@router.get("/workflows/{slug}")
async def get_workflow(slug: str, request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        return {"workflow": workflow_record_json(await defs.get_workflow(s, ctx.org_id, slug))}


@router.put("/workflows/{slug}")
async def put_workflow(slug: str, request: Request, p: Principal = Depends(require_admin)) -> dict:
    src = source_param(request)
    wf = parse_workflow(await raw_body(request))
    if wf.slug != slug:
        raise slug_mismatch(wf.slug, slug)
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        res = await defs.apply(s, ctx.org_id, defs.ApplyInput(workflows=[wf], source=src, actor=actor_label(p)))
    return {"item": find_item(res.items, "workflow", slug)}


@router.get("/workflows/{slug}/versions")
async def workflow_versions(slug: str, request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        vs = await defs.workflow_versions(s, ctx.org_id, slug)
    return {"items": [version_json(v) for v in vs]}


@router.get("/workflows/{slug}/versions/{version}")
async def workflow_version(slug: str, version: str, request: Request) -> dict:
    n = version_param(version)
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        return {"workflow": workflow_record_json(await defs.workflow_version(s, ctx.org_id, slug, n))}


# --- bundles --------------------------------------------------------------------


async def parse_bundle(request: Request) -> tuple[list[Form], list[Workflow]]:
    """Parse every document, collecting problems prefixed with its position (``forms[1].fields[0].key``)."""
    body = await read_object(request)
    raw_forms, raw_wfs = body.get("forms") or [], body.get("workflows") or []
    if not isinstance(raw_forms, list) or not isinstance(raw_wfs, list):
        raise bad_request("invalid JSON: forms and workflows must be arrays")
    problems: list[Problem] = []
    wfs: list[Workflow] = []
    for i, raw in enumerate(raw_wfs):
        try:
            wfs.append(parse_workflow(json.dumps(raw)))
        except ValidationError as e:
            problems += defs.prefix_problems(f"workflows[{i}]", e)
    forms: list[Form] = []
    for i, raw in enumerate(raw_forms):
        try:
            forms.append(parse_form(json.dumps(raw)))
        except ValidationError as e:
            problems += defs.prefix_problems(f"forms[{i}]", e)
    if problems:
        raise ValidationError(problems)
    return forms, wfs


@router.post("/definitions/validate", dependencies=[Depends(require_admin)])
async def validate(request: Request) -> dict:
    forms, wfs = await parse_bundle(request)
    ctx = get_ctx(request)
    # A dry-run apply runs the same bundle validation (incl. existing workflows) and rolls back.
    async with ctx.db.transaction() as s:
        await defs.apply(s, ctx.org_id, defs.ApplyInput(forms=forms, workflows=wfs, dry_run=True))
    return {"valid": True}


@router.post("/definitions/apply")
async def apply(request: Request, p: Principal = Depends(require_admin)) -> dict:
    src = source_param(request)
    forms, wfs = await parse_bundle(request)
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        res = await defs.apply(
            s,
            ctx.org_id,
            defs.ApplyInput(
                forms=forms,
                workflows=wfs,
                source=src,
                actor=actor_label(p),
                dry_run=request.query_params.get("dryRun") == "true",
            ),
        )
    return res.to_dict()


@router.get("/definitions", dependencies=[Depends(require_admin)])
async def export(request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        forms, wfs = await defs.export(s, ctx.org_id)
    return {"forms": [f.to_dict() for f in forms], "workflows": [w.to_dict() for w in wfs]}

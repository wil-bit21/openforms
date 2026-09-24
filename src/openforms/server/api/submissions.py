"""``/submissions`` and the CSV export (authenticated)."""

from __future__ import annotations

import uuid

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..models import definitions as defs
from ..models import submissions as subs
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import parse_uuid, read_object, require_auth
from .errors import bad_request
from .serialize import event_json, list_json, submission_json

router = APIRouter(dependencies=[Depends(require_auth)])


@router.get("/submissions")
async def list_submissions(request: Request, p: Principal = Depends(require_auth)) -> dict:
    q = request.query_params
    f = subs.ListFilter(form_slug=q.get("form", ""), state=q.get("state", ""), cursor=q.get("cursor", ""))
    a = q.get("assignee", "")
    if a == "none":
        f.unassigned = True
    elif a == "me":
        f.assignee_id = p.id
    elif a:
        try:
            f.assignee_id = uuid.UUID(a)
        except ValueError:
            raise bad_request('assignee must be a user id, "me" or "none"') from None
    if (lim := q.get("limit", "")) != "":
        try:
            n = int(lim)
        except ValueError:
            n = 0
        if n < 1:
            raise bad_request("limit must be a positive integer")
        f.limit = n
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        items, nxt = await subs.list_submissions(s, ctx.org_id, f)
    return list_json([submission_json(x) for x in items], nxt)


@router.post("/submissions")
async def create_submission(request: Request, p: Principal = Depends(require_auth)) -> JSONResponse:
    body = await read_object(request)
    form, data = body.get("form", ""), body.get("data")
    if not isinstance(form, str) or (data is not None and not isinstance(data, dict)):
        raise bad_request("invalid JSON: form must be a string and data an object")
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        sub, token = await subs.create(
            s, ctx.org_id, form, data, require_public=False, principal=p, on_created=ctx.services.get("on_created")
        )
    return JSONResponse({"submission": submission_json(sub), "receiptToken": token}, status_code=201)


@router.get("/submissions/{sub_id}")
async def get_submission(sub_id: str, request: Request, p: Principal = Depends(require_auth)) -> dict:
    sid = parse_uuid(sub_id)
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        sub = await subs.get(s, ctx.org_id, sid)
        form = await defs.get_form_version(s, sub.form_version_id)
        wf = None
        if sub.workflow_version_id is not None:
            wf = (await defs.get_workflow_version(s, sub.workflow_version_id)).definition
        events = await subs.events(s, ctx.org_id, sid)
        available = ctx.services.get("available_transitions")
        transitions = await available(s, p, sub) if available else []
    return {
        "submission": submission_json(sub),
        "form": form.definition.to_dict(),
        "workflow": None if wf is None else wf.to_dict(),
        "events": [event_json(e) for e in events],
        "transitions": transitions,
    }


@router.get("/forms/{slug}/submissions.csv")
async def export_csv(slug: str, request: Request) -> Response:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        text = await subs.export_csv(s, ctx.org_id, slug)
    return Response(
        text.encode(),
        media_type="text/csv; charset=utf-8",
        headers={"Content-Disposition": f'attachment; filename="{slug}-submissions.csv"'},
    )

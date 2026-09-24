"""Unauthenticated ``/public`` routes: hosted form definitions, submissions and status."""

from __future__ import annotations

from fastapi import APIRouter
from starlette.requests import Request
from starlette.responses import JSONResponse

from ..models import definitions as defs
from ..models import submissions as subs
from .context import get_ctx
from .dependencies import current_principal, parse_uuid, read_object
from .errors import APIError, bad_request, not_found
from .ratelimit import RateLimiter, client_ip
from .serialize import ts

DEFAULT_CONFIRMATION_MESSAGE = "Thanks! Your response has been recorded."

router = APIRouter(prefix="/public")


def limiter(request: Request) -> RateLimiter:
    ctx = get_ctx(request)
    lim = ctx.services.get("public_limiter")
    if lim is None:
        lim = ctx.services["public_limiter"] = RateLimiter(20, 60.0)
    return lim


@router.get("/forms/{slug}")
async def get_form(slug: str, request: Request) -> dict:
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        rec = await defs.get_form(s, ctx.org_id, slug)
    if not rec.definition.settings.public:
        raise not_found()
    return {"form": rec.definition.to_dict()}


@router.post("/forms/{slug}/submissions")
async def submit(slug: str, request: Request) -> JSONResponse:
    if not limiter(request).allow(client_ip(request)):
        err = APIError(429, "rate_limited", "too many submissions; try again in a minute")
        from .errors import envelope

        resp = envelope(err.status, err.code, err.message)
        resp.headers["Retry-After"] = "60"
        return resp
    body = await read_object(request)
    data = body.get("data")
    if data is not None and not isinstance(data, dict):
        raise bad_request("invalid JSON: data must be an object")
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        sub, token = await subs.create(
            s,
            ctx.org_id,
            slug,
            data,
            require_public=True,
            principal=current_principal(request),
            on_created=ctx.services.get("on_created"),
        )
        form = await defs.get_form_version(s, sub.form_version_id)
    msg = form.definition.settings.confirmation_message or DEFAULT_CONFIRMATION_MESSAGE
    return JSONResponse(
        {
            "id": str(sub.id),
            "state": sub.state,
            "stateLabel": sub.state_label,
            "receiptToken": token,
            "confirmationMessage": msg,
        },
        status_code=201,
    )


@router.get("/submissions/{sub_id}")
async def status(sub_id: str, request: Request) -> dict:
    sid = parse_uuid(sub_id)
    ctx = get_ctx(request)
    async with ctx.db.session() as s:
        st = await subs.public_status(s, sid, request.query_params.get("token", ""))
    return {
        "id": str(st.id),
        "formTitle": st.form_title,
        "state": st.state,
        "stateLabel": st.state_label,
        "terminal": st.terminal,
        "states": [x.to_dict() for x in st.states],
        "history": [{"state": h.state, "label": h.label, "at": ts(h.at)} for h in st.history],
        "createdAt": ts(st.created_at),
    }

"""Workflow endpoints: transitions, workflow fields, comments and assignee (authenticated)."""

from __future__ import annotations

import uuid

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import JSONResponse

from ...definition import Problem, ValidationError
from ..schemas.core import Principal
from ..workflow import Engine, TransitionInput
from .context import get_ctx
from .dependencies import expect, parse_uuid, read_object, require_auth
from .errors import bad_request
from .serialize import event_json, submission_json

router = APIRouter(prefix="/submissions/{sub_id}", dependencies=[Depends(require_auth)])


def engine(request: Request) -> Engine:
    return get_ctx(request).services["engine"]


def _fields(body: dict, key: str = "fields") -> dict:
    v = body.get(key)
    if v is None:
        return {}
    if not isinstance(v, dict):
        raise bad_request(f"invalid JSON: {key} must be an object")
    return v


@router.post("/transitions")
async def transition(sub_id: str, request: Request, p: Principal = Depends(require_auth)) -> dict:
    sid = parse_uuid(sub_id)
    body = await read_object(request)
    name = expect(body, "transition", str, "")
    if not name.strip():
        raise ValidationError([Problem("transition", "transition is required")])
    sub = await engine(request).transition(
        p,
        sid,
        TransitionInput(
            transition=name,
            fields=_fields(body),
            comment=expect(body, "comment", str, ""),
            expected_state=expect(body, "expectedState", str, ""),
        ),
    )
    return {"submission": submission_json(sub)}


@router.patch("/fields")
async def update_fields(sub_id: str, request: Request, p: Principal = Depends(require_auth)) -> dict:
    sid = parse_uuid(sub_id)
    body = await read_object(request)
    return {"submission": submission_json(await engine(request).update_fields(p, sid, _fields(body)))}


@router.post("/comments")
async def comment(sub_id: str, request: Request, p: Principal = Depends(require_auth)) -> JSONResponse:
    sid = parse_uuid(sub_id)
    body = await read_object(request)
    ev = await engine(request).comment(p, sid, expect(body, "body", str, ""))
    return JSONResponse({"event": event_json(ev)}, status_code=201)


@router.put("/assignee")
async def assign(sub_id: str, request: Request, p: Principal = Depends(require_auth)) -> dict:
    sid = parse_uuid(sub_id)
    body = await read_object(request)
    raw = body.get("userId")
    user_id = None
    if raw is not None:
        try:
            user_id = uuid.UUID(str(raw)) if isinstance(raw, str) else None
        except ValueError:
            user_id = None
        if user_id is None:
            raise bad_request("invalid JSON: userId must be a UUID or null")
    return {"submission": submission_json(await engine(request).assign(p, sid, user_id))}

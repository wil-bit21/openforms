"""``/users`` (admin only)."""

from __future__ import annotations

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..models import auth as auth_models
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import expect, expect_str_list, parse_uuid, read_object, require_admin
from .serialize import user_json

router = APIRouter(prefix="/users")


@router.get("")
@router.get("/", include_in_schema=False)
async def list_users(request: Request, p: Principal = Depends(require_admin)) -> dict:
    async with get_ctx(request).db.session() as s:
        users = await auth_models.list_users(s, p.org_id)
    return {"items": [user_json(u) for u in users]}


@router.post("")
@router.post("/", include_in_schema=False)
async def create_user(request: Request, p: Principal = Depends(require_admin)) -> Response:
    body = await read_object(request)
    async with get_ctx(request).db.transaction() as s:
        u = await auth_models.create_user(
            s,
            p.org_id,
            expect(body, "email", str, ""),
            expect(body, "name", str, ""),
            expect(body, "password", str, ""),
            expect_str_list(body, "roles"),
        )
    return JSONResponse({"user": user_json(u)}, status_code=201)


@router.patch("/{user_id}")
async def update_user(user_id: str, request: Request, p: Principal = Depends(require_admin)) -> dict:
    uid = parse_uuid(user_id)
    body = await read_object(request)
    roles = expect_str_list(body, "roles")
    if "roles" in body and body["roles"] is None:
        roles = None
    async with get_ctx(request).db.transaction() as s:
        u = await auth_models.update_user(
            s,
            p.org_id,
            uid,
            name=expect(body, "name", str),
            password=expect(body, "password", str),
            roles=roles,
        )
    return {"user": user_json(u)}


@router.delete("/{user_id}")
async def delete_user(user_id: str, request: Request, p: Principal = Depends(require_admin)) -> Response:
    uid = parse_uuid(user_id)
    async with get_ctx(request).db.transaction() as s:
        await auth_models.delete_user(s, p.org_id, uid)
    return Response(status_code=204)

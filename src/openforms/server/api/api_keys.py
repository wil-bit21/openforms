"""``/api-keys`` (admin only). The plaintext key is returned once, on creation."""

from __future__ import annotations

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..models import auth as auth_models
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import expect, expect_str_list, parse_uuid, read_object, require_admin
from .serialize import api_key_json

router = APIRouter(prefix="/api-keys")


@router.get("")
@router.get("/", include_in_schema=False)
async def list_keys(request: Request, p: Principal = Depends(require_admin)) -> dict:
    async with get_ctx(request).db.session() as s:
        keys = await auth_models.list_api_keys(s, p.org_id)
    return {"items": [api_key_json(k) for k in keys]}


@router.post("")
@router.post("/", include_in_schema=False)
async def create_key(request: Request, p: Principal = Depends(require_admin)) -> Response:
    body = await read_object(request)
    async with get_ctx(request).db.transaction() as s:
        plaintext, k = await auth_models.create_api_key(
            s, p.org_id, expect(body, "name", str, ""), expect_str_list(body, "roles")
        )
    return JSONResponse({"apiKey": api_key_json(k), "key": plaintext}, status_code=201)


@router.delete("/{key_id}")
async def revoke_key(key_id: str, request: Request, p: Principal = Depends(require_admin)) -> Response:
    kid = parse_uuid(key_id)
    async with get_ctx(request).db.transaction() as s:
        await auth_models.revoke_api_key(s, p.org_id, kid)
    return Response(status_code=204)

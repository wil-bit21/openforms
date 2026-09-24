"""``/auth/login``, ``/auth/logout``, ``/auth/me``."""

from __future__ import annotations

from fastapi import APIRouter, Depends
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..models import auth as auth_models
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import SESSION_COOKIE, expect, read_object, require_auth
from .serialize import principal_json, user_json

router = APIRouter()


def _cookie(resp: Response, request: Request, value: str, max_age: int) -> None:
    resp.set_cookie(
        SESSION_COOKIE,
        value,
        max_age=max_age,
        path="/",
        httponly=True,
        secure=bool(get_ctx(request).settings.cookie_secure),
        samesite="lax",
    )


@router.post("/auth/login")
async def login(request: Request) -> Response:
    body = await read_object(request)
    email = expect(body, "email", str, "")
    password = expect(body, "password", str, "")
    async with get_ctx(request).db.transaction() as s:
        token, user = await auth_models.login(s, email, password)
    resp = JSONResponse({"user": user_json(user)})
    _cookie(resp, request, token, int(auth_models.SESSION_TTL.total_seconds()))
    return resp


@router.post("/auth/logout", dependencies=[Depends(require_auth)])
async def logout(request: Request) -> Response:
    token = request.cookies.get(SESSION_COOKIE)
    if token:
        async with get_ctx(request).db.transaction() as s:
            await auth_models.logout(s, token)
    resp = Response(status_code=204)
    resp.delete_cookie(
        SESSION_COOKIE, path="/", httponly=True, secure=bool(get_ctx(request).settings.cookie_secure), samesite="lax"
    )
    return resp


@router.get("/auth/me")
async def me(p: Principal = Depends(require_auth)) -> dict:
    return {"principal": principal_json(p)}

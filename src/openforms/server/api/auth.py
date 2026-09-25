"""``/auth/login``, ``/auth/logout``, ``/auth/me`` and password reset."""

from __future__ import annotations

import logging

from fastapi import APIRouter, Depends
from starlette.background import BackgroundTask
from starlette.requests import Request
from starlette.responses import JSONResponse, Response

from ..actions.mailer import Mailer, new_mailer
from ..models import auth as auth_models
from ..schemas.core import Principal
from .context import get_ctx
from .dependencies import SESSION_COOKIE, expect, read_object, require_auth
from .ratelimit import client_ip, shared_limiter, too_many_requests
from .serialize import principal_json, user_json

router = APIRouter()
log = logging.getLogger("openforms.auth")

# (limit, window seconds). Fixed windows per process; enough to stop online guessing.
LOGIN_PER_IP = (20, 300.0)
LOGIN_PER_EMAIL = (10, 900.0)
RESET_PER_IP = (5, 900.0)
RESET_PER_EMAIL = (3, 3600.0)
RESET_CONFIRM_PER_IP = (10, 900.0)


def _limited(request: Request, name: str, key: str, rule: tuple[int, float]) -> bool:
    return not shared_limiter(get_ctx(request).services, name, *rule).allow(key)


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
    if _limited(request, "login_ip", client_ip(request), LOGIN_PER_IP) or _limited(
        request, "login_email", email.strip().lower(), LOGIN_PER_EMAIL
    ):
        return too_many_requests("too many sign-in attempts; try again later", LOGIN_PER_IP[1])
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


RESET_SUBJECT = "Reset your openforms password"


def reset_email_body(base_url: str, email: str, token: str) -> str:
    link = f"{base_url}/admin/reset-password?token={token}"
    return (
        f"Someone asked to reset the password for {email} on {base_url}.\n\n"
        f"Open this link within {int(auth_models.RESET_TTL.total_seconds() // 60)} minutes to choose a new password:\n"
        f"{link}\n\n"
        "If you did not ask for this, ignore this email; your password stays the same.\n"
    )


def _mailer(request: Request) -> Mailer:
    ctx = get_ctx(request)
    m = ctx.services.get("mailer")
    if m is None:
        m = ctx.services["mailer"] = new_mailer(ctx.settings)
    return m


async def _send_reset(mailer: Mailer, to: str, body: str) -> None:
    try:
        await mailer.send(to, RESET_SUBJECT, body)
    except Exception:
        log.exception("password reset email to %s failed", to)


@router.post("/auth/password-reset")
async def request_password_reset(request: Request) -> Response:
    """Email a reset link. Always 202, so the response does not reveal which emails exist."""
    body = await read_object(request)
    email = expect(body, "email", str, "").strip().lower()
    if _limited(request, "reset_ip", client_ip(request), RESET_PER_IP) or _limited(
        request, "reset_email", email, RESET_PER_EMAIL
    ):
        return too_many_requests("too many reset requests; try again later", RESET_PER_IP[1])
    ctx = get_ctx(request)
    async with ctx.db.transaction() as s:
        issued = await auth_models.request_password_reset(s, email) if email else None
    task = None
    if issued is not None:
        token, user = issued
        task = BackgroundTask(
            _send_reset, _mailer(request), user.email, reset_email_body(ctx.settings.base_url, user.email, token)
        )
    return Response(status_code=202, background=task)


@router.post("/auth/password-reset/confirm")
async def confirm_password_reset(request: Request) -> Response:
    """Set a new password with the token from the email; signs the user out everywhere."""
    body = await read_object(request)
    token = expect(body, "token", str, "")
    password = expect(body, "password", str, "")
    if _limited(request, "reset_confirm_ip", client_ip(request), RESET_CONFIRM_PER_IP):
        return too_many_requests("too many attempts; try again later", RESET_CONFIRM_PER_IP[1])
    async with get_ctx(request).db.transaction() as s:
        await auth_models.reset_password(s, token, password)
    return Response(status_code=204)

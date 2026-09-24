"""Request dependencies: authentication, JSON bodies and path parameters."""

from __future__ import annotations

import json
import uuid
from typing import Any

from starlette.requests import Request

from ..exceptions import Unauthenticated
from ..models import auth as auth_models
from ..schemas.core import Principal
from .context import get_ctx
from .errors import bad_request, forbidden, not_found

SESSION_COOKIE = "of_session"
MAX_BODY_BYTES = 1 << 20


async def authenticate(request: Request) -> None:
    """Resolve a Bearer API key or the ``of_session`` cookie into a principal.
    Invalid credentials leave the request anonymous; infrastructure errors propagate (500)."""
    request.state.principal = None
    ctx = get_ctx(request)
    header = request.headers.get("authorization")
    cookie = request.cookies.get(SESSION_COOKIE)
    if not header and not cookie:
        return
    try:
        async with ctx.db.transaction() as s:
            if header:
                if not header.startswith("Bearer "):
                    raise Unauthenticated()
                request.state.principal = await auth_models.principal_from_api_key(s, header[len("Bearer ") :].strip())
            else:
                request.state.principal = await auth_models.principal_from_session(s, cookie or "")
    except Unauthenticated:
        request.state.principal = None


def current_principal(request: Request) -> Principal | None:
    return getattr(request.state, "principal", None)


def require_auth(request: Request) -> Principal:
    p = current_principal(request)
    if p is None:
        raise Unauthenticated()
    return p


def require_admin(request: Request) -> Principal:
    p = require_auth(request)
    if not p.is_admin():
        raise forbidden("admin role required")
    return p


async def read_body(request: Request) -> bytes:
    """At most 1 MiB; larger bodies are 413 ``bad_request``."""
    data = bytearray()
    async for chunk in request.stream():
        data += chunk
        if len(data) > MAX_BODY_BYTES:
            raise bad_request("request body exceeds 1 MiB", 413)
    return bytes(data)


async def read_json(request: Request) -> Any:
    data = await read_body(request)
    if not data.strip():
        raise bad_request("request body is required")
    try:
        return json.loads(data)
    except (ValueError, UnicodeDecodeError) as e:
        raise bad_request(f"invalid JSON: {e}") from None


async def read_object(request: Request) -> dict[str, Any]:
    body = await read_json(request)
    if not isinstance(body, dict):
        raise bad_request("invalid JSON: expected an object")
    return body


def expect(body: dict[str, Any], key: str, kind: type | tuple[type, ...], default: Any = None) -> Any:
    """Typed access to an optional body property; wrong types are 400 (like Go's decoder)."""
    v = body.get(key, default)
    if v is None:
        return default
    if isinstance(v, bool) and bool not in (kind if isinstance(kind, tuple) else (kind,)):
        raise bad_request(f"invalid JSON: {key} has the wrong type")
    if not isinstance(v, kind):
        raise bad_request(f"invalid JSON: {key} has the wrong type")
    return v


def expect_str_list(body: dict[str, Any], key: str) -> list[str] | None:
    v = body.get(key)
    if v is None:
        return None
    if not isinstance(v, list) or not all(isinstance(x, str) for x in v):
        raise bad_request(f"invalid JSON: {key} must be a list of strings")
    return v


def parse_uuid(value: str) -> uuid.UUID:
    """Malformed ids are 404 ``not_found``."""
    try:
        return uuid.UUID(value)
    except ValueError:
        raise not_found() from None

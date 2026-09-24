"""``/healthz``."""

from __future__ import annotations

import asyncio

from starlette.requests import Request
from starlette.responses import JSONResponse

from .context import get_ctx


async def healthz(request: Request) -> JSONResponse:
    try:
        await asyncio.wait_for(get_ctx(request).db.ping(), timeout=2)
    except Exception:
        return JSONResponse({"status": "unavailable"}, status_code=503)
    return JSONResponse({"status": "ok"})

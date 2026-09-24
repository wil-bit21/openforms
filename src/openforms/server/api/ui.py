"""Serves the web builds (admin, hosted, demo, embed.js) from ``openforms/server/ui``."""

from __future__ import annotations

import mimetypes
import posixpath
from pathlib import Path

from starlette.requests import Request
from starlette.responses import FileResponse, PlainTextResponse, RedirectResponse, Response

from .context import get_ctx

mimetypes.add_type("text/javascript", ".js")
mimetypes.add_type("text/css", ".css")


def _safe_path(root: Path, name: str) -> Path | None:
    name = posixpath.normpath("/" + name).lstrip("/")
    if not name or name == ".":
        return None
    p = root / name
    try:
        p.resolve().relative_to(root.resolve())
    except ValueError:
        return None
    return p if p.is_file() else None


def _static(request: Request, root: Path, name: str) -> Response:
    p = _safe_path(root, name)
    if p is None:
        return PlainTextResponse("404 page not found\n", status_code=404)
    cache = "public, max-age=31536000, immutable" if "/assets/" in "/" + name else "no-cache"
    ctype = mimetypes.guess_type(p.name)[0] or "application/octet-stream"
    if ctype.startswith("text/") or ctype == "application/javascript":
        ctype += "; charset=utf-8"
    return FileResponse(p, headers={"Cache-Control": cache}, media_type=ctype)


def _index(request: Request, root: Path, app: str) -> Response:
    p = root / app / "index.html"
    if not p.is_file():
        return PlainTextResponse("web UI not built; run `make web`\n", status_code=503)
    content = b"" if request.method == "HEAD" else p.read_bytes()
    return Response(content, media_type="text/html; charset=utf-8", headers={"Cache-Control": "no-cache"})


async def serve_ui(request: Request) -> Response:
    if request.method not in ("GET", "HEAD"):
        return PlainTextResponse("method not allowed\n", status_code=405, headers={"Allow": "GET, HEAD"})
    root = get_ctx(request).ui_dir or Path("/nonexistent")
    path = request.url.path
    if path == "/":
        return RedirectResponse("/admin", status_code=302)
    if path.startswith("/_app/"):
        return _static(request, root, path[len("/_app/") :])
    if path == "/embed.js":
        return _static(request, root, "embed/embed.js")
    if path == "/admin" or path.startswith("/admin/"):
        return _index(request, root, "admin")
    if path.startswith(("/f/", "/s/")):
        return _index(request, root, "hosted")
    if path == "/demo" or path.startswith("/demo/"):
        return _index(request, root, "demo")
    return PlainTextResponse("404 page not found\n", status_code=404)

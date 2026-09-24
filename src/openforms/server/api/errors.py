"""The error envelope ``{"error": {"code", "message", "details"?}}`` (spec §7.1)."""

from __future__ import annotations

import logging

from fastapi import FastAPI
from fastapi.exceptions import RequestValidationError
from starlette.exceptions import HTTPException as StarletteHTTPException
from starlette.requests import Request
from starlette.responses import JSONResponse

from ...definition import Problem, ValidationError
from ..exceptions import OpenFormsError

log = logging.getLogger("openforms.api")


class APIError(Exception):
    """An error with an explicit HTTP status and envelope code."""

    def __init__(self, status: int, code: str, message: str, details: list[Problem] | None = None) -> None:
        super().__init__(f"{code}: {message}")
        self.status, self.code, self.message, self.details = status, code, message, details or []


def not_found(msg: str = "not found") -> APIError:
    return APIError(404, "not_found", msg)


def forbidden(msg: str) -> APIError:
    return APIError(403, "forbidden", msg)


def bad_request(msg: str, status: int = 400) -> APIError:
    return APIError(status, "bad_request", msg)


def envelope(status: int, code: str, message: str, details: list[Problem] | None = None) -> JSONResponse:
    body: dict = {"code": code, "message": message}
    if details:
        body["details"] = [p.to_dict() for p in details]
    return JSONResponse({"error": body}, status_code=status)


def error_response(request: Request, err: BaseException) -> JSONResponse:
    """Order: APIError, ValidationError, domain errors, then 500 internal."""
    if isinstance(err, APIError):
        return envelope(err.status, err.code, err.message, err.details)
    if isinstance(err, ValidationError):
        return envelope(422, "validation_failed", "validation failed", err.problems)
    if isinstance(err, OpenFormsError):
        return envelope(err.status, err.code, err.message)
    log.error("internal error: %r (method=%s path=%s)", err, request.method, request.url.path, exc_info=err)
    return envelope(500, "internal", "internal server error")


def install_error_handlers(app: FastAPI) -> None:
    async def handle(request: Request, exc: Exception) -> JSONResponse:
        return error_response(request, exc)

    async def http_exc(request: Request, exc: Exception) -> JSONResponse:
        assert isinstance(exc, StarletteHTTPException)
        if exc.status_code == 404:
            return envelope(404, "not_found", "no such endpoint")
        if exc.status_code == 405:
            return envelope(405, "bad_request", "method not allowed")
        return envelope(exc.status_code, "bad_request", str(exc.detail))

    async def request_validation(request: Request, exc: Exception) -> JSONResponse:
        return envelope(400, "bad_request", "invalid request")

    app.add_exception_handler(APIError, handle)
    app.add_exception_handler(ValidationError, handle)
    app.add_exception_handler(OpenFormsError, handle)
    app.add_exception_handler(StarletteHTTPException, http_exc)
    app.add_exception_handler(RequestValidationError, request_validation)
    app.add_exception_handler(Exception, handle)

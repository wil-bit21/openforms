import uuid

import pytest
from starlette.requests import Request

from openforms.definition import Problem, ValidationError
from openforms.server.api.dependencies import read_json
from openforms.server.api.errors import APIError, error_response
from openforms.server.exceptions import EmailTaken, InvalidCredentials, NotFound, Unauthenticated
from tests.conftest import error_code


def _req() -> Request:
    return Request({"type": "http", "method": "GET", "path": "/x", "headers": []})


@pytest.mark.parametrize(
    "err,status,code",
    [
        (APIError(418, "teapot", "short and stout"), 418, "teapot"),
        (ValidationError([Problem("email", "bad")]), 422, "validation_failed"),
        (Unauthenticated(), 401, "unauthenticated"),
        (InvalidCredentials(), 401, "invalid_credentials"),
        (EmailTaken(), 409, "email_taken"),
        (NotFound(), 404, "not_found"),
        (RuntimeError("database exploded"), 500, "internal"),
    ],
)
def test_error_mapping(err, status, code):
    resp = error_response(_req(), err)
    assert resp.status_code == status
    assert resp.headers["content-type"] == "application/json"
    import json

    body = json.loads(resp.body)
    assert body["error"]["code"] == code and body["error"]["message"]
    if isinstance(err, ValidationError):
        assert body["error"]["details"] == [{"path": "email", "message": "bad"}]
    if status == 500:
        assert b"exploded" not in resp.body and b'"details"' not in resp.body


async def _body_request(body: bytes) -> Request:
    sent = False

    async def receive():
        nonlocal sent
        if sent:
            return {"type": "http.request", "body": b"", "more_body": False}
        sent = True
        return {"type": "http.request", "body": body, "more_body": False}

    return Request({"type": "http", "method": "POST", "path": "/", "headers": []}, receive)


async def test_read_json():
    assert await read_json(await _body_request(b'{"name":"ada","extra":1}')) == {"name": "ada", "extra": 1}
    for body, status in ((b'{"name":', 400), (b"", 400), (b'{"name":"' + b"a" * (1 << 20) + b'"}', 413)):
        with pytest.raises(APIError) as ei:
            await read_json(await _body_request(body))
        assert (ei.value.status, ei.value.code) == (status, "bad_request")


async def test_healthz(env):
    r = await env.do("GET", "/healthz")
    assert r.status_code == 200 and r.json() == {"status": "ok"}


async def test_unknown_api_path_is_json_404(env):
    r = await env.do("GET", "/api/v1/does-not-exist")
    assert r.status_code == 404 and error_code(r) == "not_found"


async def test_wrong_method_on_api_path_is_json_405(env):
    r = await env.do("DELETE", "/api/v1/auth/me")
    assert r.status_code == 405 and error_code(r) == "bad_request"


async def test_invalid_bearer_does_not_break_public_routes(env):
    assert (await env.do("GET", "/healthz", "ofk_bogus")).status_code == 200


async def test_require_auth_and_admin(env):
    r = await env.do("GET", "/api/v1/auth/me")
    assert r.status_code == 401 and error_code(r) == "unauthenticated"
    reviewer = await env.api_key("reviewer")
    r = await env.do("GET", "/api/v1/users", reviewer)
    assert r.status_code == 403 and error_code(r) == "forbidden"
    assert (await env.do("GET", "/api/v1/users")).status_code == 401
    assert (await env.do("GET", "/api/v1/users", await env.api_key("admin"))).status_code == 200


async def test_malformed_uuid_is_404(env):
    admin = await env.api_key("admin")
    r = await env.do("DELETE", "/api/v1/users/not-a-uuid", admin)
    assert r.status_code == 404 and error_code(r) == "not_found"
    r = await env.do("DELETE", f"/api/v1/users/{uuid.uuid4()}", admin)
    assert r.status_code == 404

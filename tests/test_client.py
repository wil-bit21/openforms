"""Port of ``internal/client/client_test.go`` (httpx MockTransport stands in for httptest)."""

import json
import socket

import httpx
import pytest

from openforms.client import APIError, ApplyItem, Bundle, Client, ClientError
from openforms.definition import Form


def client(handler, base="http://srv/"):
    return Client(base, "ofk_test", transport=httpx.MockTransport(handler))


async def test_apply_posts_bundle_with_source_and_dry_run():
    got = {}

    def handler(req: httpx.Request):
        got.update(method=req.method, path=req.url.path, query=dict(req.url.params), auth=req.headers.get("authorization"),
                   ct=req.headers.get("content-type"), body=req.content)  # fmt: skip
        return httpx.Response(
            200, json={"items": [{"kind": "form", "slug": "contact", "version": 2, "changed": True, "created": False}]}
        )

    res = await client(handler).apply(Bundle(forms=[Form(slug="contact", title="Contact")]), True)
    assert (got["method"], got["path"]) == ("POST", "/api/v1/definitions/apply")
    assert got["query"] == {"source": "cli", "dryRun": "true"}
    assert got["auth"] == "Bearer ofk_test" and got["ct"] == "application/json"
    body = json.loads(got["body"])
    assert body["workflows"] == [] and body["forms"][0]["slug"] == "contact"
    assert res.items == [ApplyItem("form", "contact", 2, True, False)]


async def test_apply_without_dry_run_omits_param():
    got = {}

    def handler(req):
        got["query"] = dict(req.url.params)
        return httpx.Response(200, json={"items": []})

    await client(handler).apply(Bundle(), False)
    assert got["query"] == {"source": "cli"}


async def test_validate_returns_api_error_with_details():
    def handler(req):
        assert req.url.path == "/api/v1/definitions/validate"
        return httpx.Response(422, json={"error": {"code": "validation_failed", "message": "definitions are invalid",
                                                   "details": [{"path": "fields[0].type", "message": "unknown field type"}]}})  # fmt: skip

    with pytest.raises(APIError) as e:
        await client(handler).validate(Bundle())
    err = e.value
    assert (err.status, err.code, err.message) == (422, "validation_failed", "definitions are invalid")
    assert [d.path for d in err.details] == ["fields[0].type"]
    assert "validation_failed" in str(err)


async def test_validate_ok():
    res = await client(lambda req: httpx.Response(200, json={"valid": True})).validate(Bundle())
    assert res.valid


async def test_export_decodes_bundle():
    def handler(req):
        assert (req.method, req.url.path) == ("GET", "/api/v1/definitions")
        return httpx.Response(200, json={
            "forms": [{"slug": "contact", "title": "Contact", "settings": {"public": True}, "fields": []}],
            "workflows": [{"slug": "triage", "title": "Triage", "initial": "new", "states": [{"key": "new", "label": "New"}], "transitions": []}],
        })  # fmt: skip

    b = await client(handler).export()
    assert [f.slug for f in b.forms] == ["contact"] and b.forms[0].settings.public
    assert [w.initial for w in b.workflows] == ["new"]


async def test_non_json_error_body():
    with pytest.raises(APIError) as e:
        await client(lambda req: httpx.Response(502, text="Bad gateway\n")).export()
    assert (e.value.status, e.value.code, e.value.message) == (502, "http_error", "Bad gateway")


async def test_empty_error_body_uses_status_text():
    with pytest.raises(APIError) as e:
        await client(lambda req: httpx.Response(503)).export()
    assert e.value.message == "Service Unavailable"


async def test_unreachable_server():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        base = f"http://127.0.0.1:{s.getsockname()[1]}"
    with pytest.raises(ClientError) as e:
        await Client(base, "k").export()
    assert str(e.value).startswith("cannot reach openforms server at " + base)

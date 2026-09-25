from openforms.server.models import definitions as defs
from tests.conftest import error_code
from tests.samples import sample_form, sample_workflow


def wf(slug="review"):
    return sample_workflow(slug).to_dict()


def form(slug, workflow=""):
    return sample_form(slug, workflow).to_dict()


async def test_put_definitions_require_admin(api):
    r = await api.do("PUT", "/api/v1/workflows/review", await api.api_key("reviewer"), wf())
    assert r.status_code == 403 and error_code(r) == "forbidden"
    assert (await api.do("PUT", "/api/v1/workflows/review", "", wf())).status_code == 401


async def test_put_workflow_then_form(api):
    r = await api.do("PUT", "/api/v1/workflows/review", api.admin, wf())
    assert r.status_code == 200, r.text
    assert r.json()["item"] == {"kind": "workflow", "slug": "review", "version": 1, "changed": True, "created": True}
    assert (
        await api.do("PUT", "/api/v1/forms/contact?source=ui", api.admin, form("contact", "review"))
    ).status_code == 200
    item = (await api.do("PUT", "/api/v1/forms/contact?source=ui", api.admin, form("contact", "review"))).json()["item"]
    assert not item["changed"] and item["version"] == 1
    async with api.db.session() as s:
        f = await defs.get_form(s, api.org_id, "contact")
    assert f.current.source == "ui" and f.current.created_by


async def test_put_form_accepts_yaml(api):
    yaml_doc = (
        "slug: plain\ntitle: Contact\nsettings: {public: true}\nfields:\n  - {key: name, type: text, label: Name}\n"
    )
    r = await api.do("PUT", "/api/v1/forms/plain", api.admin, yaml_doc.encode())
    assert r.status_code == 200, r.text


async def test_put_form_slug_mismatch(api):
    r = await api.do("PUT", "/api/v1/forms/other", api.admin, form("plain"))
    assert r.status_code == 422
    e = r.json()["error"]
    assert e["code"] == "validation_failed" and [d["path"] for d in e["details"]] == ["slug"]


async def test_put_form_invalid_definition(api):
    body = {
        "slug": "plain",
        "title": "Bad",
        "settings": {"public": True},
        "fields": [{"key": "1bad", "type": "nope", "label": "X"}],
    }
    r = await api.do("PUT", "/api/v1/forms/plain", api.admin, body)
    assert r.status_code == 422 and error_code(r) == "validation_failed"


async def test_put_form_missing_workflow(api):
    assert (await api.do("PUT", "/api/v1/forms/contact", api.admin, form("contact", "nope"))).status_code == 422


async def test_put_form_unknown_source(api):
    r = await api.do("PUT", "/api/v1/forms/plain?source=seed", api.admin, form("plain"))
    assert r.status_code == 400 and error_code(r) == "bad_request"


async def test_get_form_and_list(api):
    await api.seed()
    await api.create("contact")
    await api.create("contact")
    reviewer = await api.api_key("reviewer")
    r = await api.do("GET", "/api/v1/forms/contact", reviewer)
    assert r.status_code == 200
    f = r.json()["form"]
    assert (f["slug"], f["version"], f["source"], f["workflowVersion"], f["definition"]["title"]) == (
        "contact",
        1,
        "api",
        1,
        "Contact",
    )
    assert f["updatedAt"].endswith("Z")
    r = await api.do("GET", "/api/v1/forms/missing", reviewer)
    assert r.status_code == 404 and error_code(r) == "not_found"
    items = (await api.do("GET", "/api/v1/forms", reviewer)).json()["items"]
    assert [i["slug"] for i in items] == ["contact", "internal", "plain"]
    assert items[0]["submissionCount"] == 2 and items[0]["workflow"] == "review" and items[0]["public"]
    assert not items[1]["public"] and items[2]["workflow"] is None and items[2]["submissionCount"] == 0


async def test_form_version_endpoints(api):
    await api.do("PUT", "/api/v1/forms/plain", api.admin, form("plain"))
    f = sample_form("plain")
    f.title = "Contact v2"
    await api.do("PUT", "/api/v1/forms/plain", api.admin, f.to_dict())
    items = (await api.do("GET", "/api/v1/forms/plain/versions", api.admin)).json()["items"]
    assert len(items) == 2 and items[0]["version"] == 2 and len(items[0]["hash"]) == 64
    v1 = (await api.do("GET", "/api/v1/forms/plain/versions/1", api.admin)).json()["form"]
    assert v1["version"] == 1 and v1["definition"]["title"] == "Contact"
    for path in (
        "/api/v1/forms/plain/versions/9",
        "/api/v1/forms/plain/versions/abc",
        "/api/v1/forms/missing/versions",
        "/api/v1/forms/plain/versions/0",
        "/api/v1/forms/plain/versions/-1",
    ):
        assert (await api.do("GET", path, api.admin)).status_code == 404, path


async def test_workflow_endpoints(api):
    await api.seed()
    items = (await api.do("GET", "/api/v1/workflows", api.admin)).json()["items"]
    assert len(items) == 1 and (items[0]["slug"], items[0]["stateCount"], items[0]["title"]) == ("review", 3, "Review")
    w = (await api.do("GET", "/api/v1/workflows/review", api.admin)).json()["workflow"]
    assert w["slug"] == "review" and w["definition"]["initial"] == "new"
    assert (await api.do("GET", "/api/v1/workflows/review/versions", api.admin)).status_code == 200
    assert (await api.do("GET", "/api/v1/workflows/review/versions/1", api.admin)).status_code == 200
    assert (await api.do("PUT", "/api/v1/workflows/other", api.admin, wf())).status_code == 422


async def test_validate_bundle_endpoint(api):
    ok = {"workflows": [wf()], "forms": [form("contact", "review")]}
    r = await api.do("POST", "/api/v1/definitions/validate", api.admin, ok)
    assert r.status_code == 200 and r.json() == {"valid": True}
    assert (await api.do("GET", "/api/v1/forms/contact", api.admin)).status_code == 404
    bad = {"forms": [form("plain"), {"slug": "Bad Slug", "title": "x", "settings": {"public": True}, "fields": []}]}
    r = await api.do("POST", "/api/v1/definitions/validate", api.admin, bad)
    assert r.status_code == 422
    assert all(d["path"].startswith("forms[1]") for d in r.json()["error"]["details"])
    r = await api.do("POST", "/api/v1/definitions/validate", api.admin, {"forms": [form("contact", "nope")]})
    assert r.status_code == 422
    assert (await api.do("POST", "/api/v1/definitions/validate", await api.api_key("reviewer"), ok)).status_code == 403


async def test_apply_and_export_endpoints(api):
    bundle = {"workflows": [wf()], "forms": [form("contact", "review")]}
    r = await api.do("POST", "/api/v1/definitions/apply?dryRun=true", api.admin, bundle)
    assert r.status_code == 200 and len(r.json()["items"]) == 2 and r.json()["items"][0]["created"]
    assert (await api.do("GET", "/api/v1/forms/contact", api.admin)).status_code == 404
    assert (await api.do("POST", "/api/v1/definitions/apply?source=cli", api.admin, bundle)).status_code == 200
    async with api.db.session() as s:
        assert (await defs.get_form(s, api.org_id, "contact")).current.source == "cli"
    exp = (await api.do("GET", "/api/v1/definitions", api.admin)).json()
    assert [f["slug"] for f in exp["forms"]] == ["contact"] and len(exp["workflows"]) == 1


async def test_export_empty_returns_arrays(api):
    r = await api.do("GET", "/api/v1/definitions", api.admin)
    assert r.json() == {"forms": [], "workflows": []}


async def test_put_body_too_large(api):
    r = await api.do("PUT", "/api/v1/forms/plain", api.admin, b"x" * ((1 << 20) + 10))
    assert r.status_code == 400 and error_code(r) == "bad_request"

import asyncio
import uuid

import pytest
import sqlalchemy as sa

from openforms.definition import Problem, ValidationError
from openforms.server.exceptions import NotFound
from openforms.server.models import definitions as defs
from openforms.server.models.definitions import ApplyInput, ApplyItem
from tests.samples import sample_form, sample_workflow


async def apply(database, org_id, **kw):
    async with database.transaction() as s:
        return await defs.apply(s, org_id, ApplyInput(**kw))


async def read(database, fn, *args):
    async with database.session() as s:
        return await fn(s, *args)


async def test_apply_creates_version_one(database, org_id):
    res = await apply(
        database,
        org_id,
        workflows=[sample_workflow("review")],
        forms=[sample_form("contact", "review")],
        source="cli",
        actor="dev@example.com",
    )
    assert res.items == [ApplyItem("workflow", "review", 1, True, True), ApplyItem("form", "contact", 1, True, True)]
    f = await read(database, defs.get_form, org_id, "contact")
    assert (f.current.version, f.current.source, f.current.created_by) == (1, "cli", "dev@example.com")
    assert f.definition.title == "Contact" and len(f.definition.fields) == 3
    assert len(f.current.hash) == 64
    wf = await read(database, defs.get_workflow, org_id, "review")
    assert f.workflow_version_id == wf.current.id
    assert wf.definition.initial == "new" and len(wf.definition.states) == 3


async def test_apply_unchanged_is_noop(database, org_id):
    kw = dict(workflows=[sample_workflow("review")], forms=[sample_form("contact", "review")])
    await apply(database, org_id, **kw)
    res = await apply(database, org_id, **kw)
    assert res.items == [ApplyItem("workflow", "review", 1), ApplyItem("form", "contact", 1)]


async def test_apply_changed_form_creates_new_version(database, org_id):
    await apply(database, org_id, forms=[sample_form("plain")])
    changed = sample_form("plain")
    changed.title = "Contact us"
    res = await apply(database, org_id, forms=[changed], source="ui")
    assert res.items == [ApplyItem("form", "plain", 2, True)]
    f = await read(database, defs.get_form, org_id, "plain")
    assert (f.current.version, f.definition.title, f.current.source, f.workflow_version_id) == (
        2,
        "Contact us",
        "ui",
        None,
    )


async def test_apply_defaults_source_to_api(database, org_id):
    await apply(database, org_id, forms=[sample_form("plain")], source="")
    assert (await read(database, defs.get_form, org_id, "plain")).current.source == "api"


async def test_get_unknown_raises_not_found(database, org_id):
    for fn in (defs.get_form, defs.get_workflow):
        with pytest.raises(NotFound):
            await read(database, fn, org_id, "missing")


def test_parse_source():
    for s in ("cli", "ui", "api", "seed"):
        assert defs.parse_source(s) == s
    assert defs.parse_source("git") is None


async def test_apply_repins_dependent_forms(database, org_id):
    await apply(
        database,
        org_id,
        workflows=[sample_workflow("review")],
        forms=[sample_form("contact", "review"), sample_form("plain")],
    )
    wf = sample_workflow("review")
    wf.title = "Review v2"
    res = await apply(database, org_id, workflows=[wf], source="ui")
    assert res.items == [ApplyItem("workflow", "review", 2, True), ApplyItem("form", "contact", 2, True)]
    f = await read(database, defs.get_form, org_id, "contact")
    w = await read(database, defs.get_workflow, org_id, "review")
    assert f.workflow_version_id == w.current.id and f.current.source == "ui"
    assert (await read(database, defs.get_form, org_id, "plain")).current.version == 1


async def test_apply_repins_form_in_bundle_with_unchanged_definition(database, org_id):
    await apply(database, org_id, workflows=[sample_workflow("review")], forms=[sample_form("contact", "review")])
    wf = sample_workflow("review")
    wf.title = "Review v2"
    res = await apply(database, org_id, workflows=[wf], forms=[sample_form("contact", "review")])
    assert res.items == [ApplyItem("workflow", "review", 2, True), ApplyItem("form", "contact", 2, True)]


async def test_apply_form_against_existing_workflow(database, org_id):
    await apply(database, org_id, workflows=[sample_workflow("review")])
    res = await apply(database, org_id, forms=[sample_form("contact", "review")])
    assert len(res.items) == 1 and res.items[0].created


async def test_apply_dry_run_persists_nothing(database, org_id):
    res = await apply(
        database, org_id, workflows=[sample_workflow("review")], forms=[sample_form("contact", "review")], dry_run=True
    )
    assert len(res.items) == 2 and res.items[0].created and res.items[1].created
    for fn, slug in ((defs.get_form, "contact"), (defs.get_workflow, "review")):
        with pytest.raises(NotFound):
            await read(database, fn, org_id, slug)


async def test_apply_rejects_missing_workflow(database, org_id):
    with pytest.raises(ValidationError):
        await apply(database, org_id, forms=[sample_form("contact", "nope")])
    with pytest.raises(NotFound):
        await read(database, defs.get_form, org_id, "contact")


async def test_apply_rejects_duplicate_slugs(database, org_id):
    with pytest.raises(ValidationError) as ei:
        await apply(database, org_id, forms=[sample_form("plain"), sample_form("plain")])
    assert ei.value.problems[0].path == "forms[1].slug"


async def test_apply_prefixes_document_problems(database, org_id):
    wf = sample_workflow("review")
    wf.initial = "missing"
    with pytest.raises(ValidationError) as ei:
        await apply(database, org_id, workflows=[wf])
    assert all(p.path.startswith("workflows[0]") for p in ei.value.problems)


async def test_apply_concurrent_same_new_slug(database, org_id):
    results = await asyncio.gather(
        *(apply(database, org_id, forms=[sample_form("plain")]) for _ in range(4)), return_exceptions=True
    )
    assert not [r for r in results if isinstance(r, BaseException)], results
    assert len(await read(database, defs.form_versions, org_id, "plain")) == 1


def test_prefix_problems():
    err = ValidationError(
        [Problem("fields[0].key", "bad key"), Problem("", "whole document"), Problem("[2]", "indexed")]
    )
    assert defs.prefix_problems("forms[3]", err) == [
        Problem("forms[3].fields[0].key", "bad key"),
        Problem("forms[3]", "whole document"),
        Problem("forms[3][2]", "indexed"),
    ]
    assert defs.prefix_problems("forms[0]", RuntimeError("boom")) == [Problem("forms[0]", "boom")]
    assert defs.prefix_problems("forms[0]", None) == []


async def seed_two_versions(database, org_id):
    await apply(database, org_id, workflows=[sample_workflow("review")], forms=[sample_form("contact", "review")])
    f = sample_form("contact", "review")
    f.title = "Contact v2"
    await apply(database, org_id, forms=[f], source="ui", actor="ui@example.com")


async def test_form_versions_newest_first(database, org_id):
    await seed_two_versions(database, org_id)
    vs = await read(database, defs.form_versions, org_id, "contact")
    assert [v.version for v in vs] == [2, 1]
    assert (vs[0].source, vs[0].created_by) == ("ui", "ui@example.com")
    with pytest.raises(NotFound):
        await read(database, defs.form_versions, org_id, "missing")


async def test_form_version_returns_historic_definition(database, org_id):
    await seed_two_versions(database, org_id)
    v1 = await read(database, defs.form_version, org_id, "contact", 1)
    assert v1.definition.title == "Contact" and v1.current.version == 1
    with pytest.raises(NotFound):
        await read(database, defs.form_version, org_id, "contact", 9)
    by_id = await read(database, defs.get_form_version, v1.current.id)
    assert by_id.definition.title == "Contact" and by_id.slug == "contact"


async def test_workflow_versions_and_lookups(database, org_id):
    await apply(database, org_id, workflows=[sample_workflow("review")])
    wf = sample_workflow("review")
    wf.title = "Review v2"
    await apply(database, org_id, workflows=[wf])
    vs = await read(database, defs.workflow_versions, org_id, "review")
    assert len(vs) == 2 and vs[0].version == 2
    assert (await read(database, defs.workflow_version, org_id, "review", 1)).definition.title == "Review"
    with pytest.raises(NotFound):
        await read(database, defs.workflow_version, org_id, "review", 3)


async def test_get_workflow_version_is_cached(database, org_id):
    await apply(database, org_id, workflows=[sample_workflow("review")])
    cur = await read(database, defs.get_workflow, org_id, "review")
    first = await read(database, defs.get_workflow_version, cur.current.id)
    async with database.transaction() as s:
        await s.execute(
            sa.text(
                "UPDATE workflow_versions SET definition = jsonb_set(definition, '{title}', '\"tampered\"') WHERE id = :id"
            ),
            {"id": cur.current.id},
        )
    second = await read(database, defs.get_workflow_version, cur.current.id)
    assert first.definition.title == second.definition.title == "Review"


async def test_list_and_export_sorted_by_slug(database, org_id):
    await apply(
        database,
        org_id,
        workflows=[sample_workflow("zeta"), sample_workflow("alpha")],
        forms=[sample_form("b-form", "zeta"), sample_form("a-form")],
    )
    assert [f.slug for f in await read(database, defs.list_forms, org_id)] == ["a-form", "b-form"]
    assert [w.slug for w in await read(database, defs.list_workflows, org_id)] == ["alpha", "zeta"]
    ef, ew = await read(database, defs.export, org_id)
    assert [f.slug for f in ef] == ["a-form", "b-form"] and [w.slug for w in ew] == ["alpha", "zeta"]


async def test_export_empty_org(database, org_id):
    assert await read(database, defs.export, org_id) == ([], [])


async def test_list_is_org_scoped(database, org_id):
    await apply(database, org_id, forms=[sample_form("plain")])
    other = uuid.uuid4()
    async with database.transaction() as s:
        await s.execute(sa.text("INSERT INTO orgs (id, name) VALUES (:id, 'Other')"), {"id": other})
    assert await read(database, defs.list_forms, other) == []


async def test_stored_definition_hash_matches_canonical_bytes(database, org_id):
    """The JSONB round trip must reproduce the stored hash (dedupe relies on it)."""
    await apply(database, org_id, workflows=[sample_workflow("review")], forms=[sample_form("contact", "review")])
    from openforms.definition import canonical

    f = await read(database, defs.get_form, org_id, "contact")
    assert canonical(f.definition)[1] == f.current.hash

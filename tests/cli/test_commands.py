"""Ports of ``init_validate_test.go``, ``push_diff_test.go`` and ``pull_test.go``."""

import os
import socket
from pathlib import Path

import pytest

from openforms.cli import commands
from openforms.cli.deps import CLIError, DriftError, ProblemsError
from openforms.cli.local import load_local
from openforms.cli.yamlout import canonical_yaml
from openforms.client import APIError, ApplyItem, ApplyResult, Bundle, Client
from openforms.definition import Field, Form, FormSettings, Problem, State, Workflow, parse_form, parse_workflow
from tests.cli.helpers import REMOTE_ENV, FakeRemote, new_test_deps, put, read_rel

SCAFFOLDED = ["openforms.yaml", "openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"]
CONTACT_NO_WF = (
    "slug: contact\ntitle: Contact\nsettings:\n  public: true\nfields:\n  - {key: name, type: text, label: Name}\n"
)


# --- init / validate --------------------------------------------------------------------


async def test_init_scaffolds_project(tmp_path):
    d, tio = new_test_deps(tmp_path)
    await commands.run_init(d)
    for rel in SCAFFOLDED:
        assert (tmp_path / rel).exists()
    assert read_rel(tmp_path, "openforms.yaml") == "server: http://localhost:8080\ndir: openforms\n"
    assert "openforms push" in tio.out.getvalue()


async def test_init_into_subdirectory(tmp_path):
    d, _ = new_test_deps(tmp_path)
    await commands.run_init(d, "myproject")
    read_rel(tmp_path, "myproject/openforms/forms/contact.yaml")


async def test_init_refuses_to_overwrite(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", "mine")
    d, _ = new_test_deps(tmp_path)
    with pytest.raises(CLIError, match="openforms/forms/contact.yaml already exists"):
        await commands.run_init(d)
    assert read_rel(tmp_path, "openforms/forms/contact.yaml") == "mine"
    assert not (tmp_path / "openforms.yaml").exists()


async def test_init_writes_canonical_yaml(tmp_path):
    d, _ = new_test_deps(tmp_path)
    await commands.run_init(d)
    form_src = read_rel(tmp_path, "openforms/forms/contact.yaml")
    assert canonical_yaml(parse_form(form_src).to_dict()) == form_src
    wf_src = read_rel(tmp_path, "openforms/workflows/contact-triage.yaml")
    assert canonical_yaml(parse_workflow(wf_src).to_dict()) == wf_src


async def test_init_output_matches_go(tmp_path):
    """The scaffolded files and output are byte-identical to what the Go CLI wrote."""
    golden = Path(__file__).parent / "golden_init"
    d, tio = new_test_deps(tmp_path)
    await commands.run_init(d)
    for rel in SCAFFOLDED:
        assert read_rel(tmp_path, rel) == read_rel(golden, rel), rel
    assert tio.out.getvalue() == read_rel(golden, "stdout.txt")


async def test_validate_scaffolded_project(tmp_path):
    d, tio = new_test_deps(tmp_path)
    await commands.run_init(d)
    tio.reset()
    await commands.run_validate(d)
    assert tio.out.getvalue() == "OK: 1 form(s), 1 workflow(s) valid\n"


async def test_validate_reports_problems_and_fails(tmp_path):
    put(
        tmp_path,
        "defs/forms/feedback.yaml",
        "slug: contact2\ntitle: Feedback\nsettings: {public: true}\nfields:\n  - {key: name, type: text, label: Name}\n",
    )
    d, tio = new_test_deps(tmp_path)
    with pytest.raises(ProblemsError):
        await commands.run_validate(d, "defs")
    assert (
        tio.err.getvalue()
        == 'forms/feedback.yaml:slug: slug "contact2" must match file name "feedback"\n1 problem(s) found\n'
    )


async def test_validate_uses_dir_from_project_config(tmp_path):
    put(tmp_path, "openforms.yaml", "dir: defs\n")
    put(
        tmp_path,
        "defs/forms/contact.yaml",
        "slug: contact\ntitle: Contact\nsettings: {public: true}\nfields:\n  - {key: name, type: text, label: Name}\n",
    )
    d, tio = new_test_deps(tmp_path)
    await commands.run_validate(d)
    assert tio.out.getvalue() == "OK: 1 form(s), 0 workflow(s) valid\n"


# --- push / diff ---------------------------------------------------------------------------


async def test_push_applies_local_bundle_and_prints_table(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", CONTACT_NO_WF)
    fr = FakeRemote(apply_res=ApplyResult([ApplyItem("form", "contact", 1, True, True)]))
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, fr)
    await commands.run_push(d)
    assert len(fr.applied) == 1 and fr.dry_runs == [False]
    assert fr.applied[0].forms[0].slug == "contact"
    assert tio.out.getvalue() == "KIND  SLUG     VERSION  STATUS\nform  contact  1        created\n"


async def test_push_dry_run(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", CONTACT_NO_WF)
    fr = FakeRemote(apply_res=ApplyResult([ApplyItem("form", "contact", 3, True)]))
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, fr)
    await commands.run_push(d, dry_run=True)
    assert fr.dry_runs == [True]
    out = tio.out.getvalue()
    assert out.startswith("Dry run: nothing was applied.\n") and "updated" in out


async def test_push_stops_on_local_problems(tmp_path):
    put(tmp_path, "openforms/forms/wrong.yaml", CONTACT_NO_WF)
    fr = FakeRemote()
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, fr)
    with pytest.raises(ProblemsError):
        await commands.run_push(d)
    assert fr.applied == [] and "forms/wrong.yaml:slug:" in tio.err.getvalue()


async def test_push_prints_server_validation_problems(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", CONTACT_NO_WF)
    err = APIError(422, "validation_failed", "invalid", [Problem("forms[0].fields", "must have at least one field")])
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(apply_err=err))
    with pytest.raises(ProblemsError):
        await commands.run_push(d)
    assert "server:forms[0].fields: must have at least one field" in tio.err.getvalue()


async def test_push_unreachable_server(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", CONTACT_NO_WF)
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        base = f"http://127.0.0.1:{s.getsockname()[1]}"
    d, _ = new_test_deps(tmp_path, {"OPENFORMS_URL": base, "OPENFORMS_API_KEY": "ofk_x"})
    d.new_client = Client
    with pytest.raises(CLIError, match="cannot reach openforms server at " + base):
        await commands.run_push(d)


def test_apply_status():
    assert commands.apply_status(ApplyItem("", "", created=True, changed=True)) == "created"
    assert commands.apply_status(ApplyItem("", "", changed=True)) == "updated"
    assert commands.apply_status(ApplyItem("", "")) == "unchanged"


def test_diff_bundles():
    a = Form(slug="a", title="A", fields=[])
    a_changed = Form(slug="a", title="A2", fields=[])
    b = Form(slug="b", title="B", fields=[])
    c = Form(slug="c", title="C", fields=[])
    w = Workflow(slug="w", title="W", initial="s", states=[State("s", "S")])
    got = commands.diff_bundles(Bundle([a_changed, b], [w]), Bundle([a, b, c], []))
    assert got == ["~ form a", "- form c (remote only)", "+ workflow w (local only)"]


async def test_diff_command(tmp_path):
    put(tmp_path, "openforms/forms/contact.yaml", CONTACT_NO_WF)
    local, _ = load_local(tmp_path / "openforms")
    fr = FakeRemote(export=local.bundle)
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, fr)
    await commands.run_diff(d)
    assert tio.out.getvalue() == "No differences.\n"
    fr.export_data = Bundle()
    tio.reset()
    await commands.run_diff(d)
    assert tio.out.getvalue() == "+ form contact (local only)\n"


# --- pull ---------------------------------------------------------------------------------


def remote_bundle():
    return Bundle(
        forms=[Form(slug="contact", title="Contact", settings=FormSettings(public=True),
                    fields=[Field(key="email", type="email", label="Email", required=True)])],
        workflows=[Workflow(slug="triage", title="Triage", initial="new", states=[State("new", "New")], transitions=[])],
    )  # fmt: skip


async def test_pull_writes_canonical_files(tmp_path):
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    await commands.run_pull(d)
    assert read_rel(tmp_path, "openforms/forms/contact.yaml") == canonical_yaml(remote_bundle().forms[0].to_dict())
    assert read_rel(tmp_path, "openforms/workflows/triage.yaml") == (
        "slug: triage\ntitle: Triage\ninitial: new\nstates:\n  - key: new\n    label: New\ntransitions: []\n"
    )
    assert tio.out.getvalue() == (
        "created forms/contact.yaml\ncreated workflows/triage.yaml\n"
        "Pulled 1 form(s), 1 workflow(s): 2 created, 0 updated, 0 unchanged.\n"
    )


async def test_pull_is_stable_and_skips_unchanged(tmp_path):
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    await commands.run_pull(d)
    p = tmp_path / "openforms/forms/contact.yaml"
    os.utime(p, (1_000_000, 1_000_000))
    tio.reset()
    await commands.run_pull(d)
    assert p.stat().st_mtime == 1_000_000
    assert tio.out.getvalue() == "Pulled 1 form(s), 1 workflow(s): 0 created, 0 updated, 2 unchanged.\n"


async def test_pull_check_detects_drift_without_writing(tmp_path):
    put(
        tmp_path,
        "openforms/forms/contact.yaml",
        "slug: contact\ntitle: Old title\nsettings:\n  public: true\nfields:\n  - {key: name, type: text, label: Name}\n",
    )
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    with pytest.raises(DriftError):
        await commands.run_pull(d, check=True)
    assert "Old title" in read_rel(tmp_path, "openforms/forms/contact.yaml")
    assert not (tmp_path / "openforms/workflows/triage.yaml").exists()
    assert tio.out.getvalue() == "would update forms/contact.yaml\nwould create workflows/triage.yaml\n"


async def test_pull_check_up_to_date(tmp_path):
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    await commands.run_pull(d)
    tio.reset()
    await commands.run_pull(d, check=True)
    assert tio.out.getvalue() == "Up to date.\n"


async def test_pull_keeps_local_only_files(tmp_path):
    put(tmp_path, "openforms/forms/draft.yaml", "slug: draft\n")
    d, tio = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    await commands.run_pull(d)
    assert read_rel(tmp_path, "openforms/forms/draft.yaml") == "slug: draft\n"
    assert "note: forms/draft.yaml exists locally but not on the server (left untouched)" in tio.out.getvalue()


async def test_pull_refuses_alternate_extension(tmp_path):
    put(tmp_path, "openforms/forms/contact.json", "{}")
    d, _ = new_test_deps(tmp_path, REMOTE_ENV, FakeRemote(export=remote_bundle()))
    with pytest.raises(
        CLIError, match="forms/contact.json exists; pull only manages .yaml files, rename it to forms/contact.yaml"
    ):
        await commands.run_pull(d)
    assert not (tmp_path / "openforms/forms/contact.yaml").exists()

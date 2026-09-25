"""Ports of ``local_test.go`` and ``deps_test.go``."""

import io

import pytest

from openforms.cli.deps import (
    CONFIG_FILE,
    CLIError,
    ProblemsError,
    ProjectConfig,
    explain_remote_error,
    load_project_config,
)
from openforms.cli.local import LocalProblem, load_local, print_problems
from openforms.client import APIError
from openforms.definition import Problem
from tests.cli.helpers import new_test_deps, put

VALID_FORM = """slug: contact
title: Contact
workflow: triage
settings:
  public: true
fields:
  - key: email
    type: email
    label: Email
    required: true
"""

VALID_WORKFLOW = """slug: triage
title: Triage
initial: new
states:
  - key: new
    label: New
  - key: done
    label: Done
    terminal: true
transitions:
  - key: finish
    label: Finish
    from: [new]
    to: done
    guard: {}
"""


def test_load_local_valid(tmp_path):
    put(tmp_path, "forms/contact.yaml", VALID_FORM)
    put(tmp_path, "workflows/triage.yml", VALID_WORKFLOW)
    put(tmp_path, "forms/README.md", "not a definition")
    lb, problems = load_local(tmp_path)
    assert problems == []
    assert [f.slug for f in lb.bundle.forms] == ["contact"]
    assert [w.slug for w in lb.bundle.workflows] == ["triage"]
    assert lb.form_files == {"contact": "forms/contact.yaml"}
    assert lb.workflow_files == {"triage": "workflows/triage.yml"}


def test_load_local_missing_dirs_is_empty(tmp_path):
    lb, problems = load_local(tmp_path / "nope")
    assert problems == [] and lb.bundle.forms == [] and lb.bundle.workflows == []


def test_load_local_reports_schema_problems_per_file(tmp_path):
    put(
        tmp_path,
        "forms/broken.yaml",
        "slug: broken\ntitle: Broken\nsettings: {public: true}\nfields:\n  - key: a\n    type: colour\n    label: A\n",
    )
    _, problems = load_local(tmp_path)
    assert problems and all(p.file == "forms/broken.yaml" for p in problems)


def test_load_local_slug_must_match_file_name(tmp_path):
    put(tmp_path, "forms/feedback.yaml", VALID_FORM.replace("workflow: triage\n", ""))
    _, problems = load_local(tmp_path)
    assert [str(p) for p in problems] == ['forms/feedback.yaml:slug: slug "contact" must match file name "feedback"']


def test_load_local_duplicate_slug_across_extensions(tmp_path):
    put(tmp_path, "forms/contact.yaml", VALID_FORM.replace("workflow: triage\n", ""))
    put(
        tmp_path,
        "forms/contact.json",
        '{"slug":"contact","title":"Contact","settings":{"public":true},"fields":[{"key":"name","type":"text","label":"Name"}]}',
    )
    _, problems = load_local(tmp_path)
    assert len(problems) == 1 and "duplicate form slug" in problems[0].message
    assert (
        str(problems[0])
        == 'forms/contact.yaml:slug: duplicate form slug "contact" (also defined in forms/contact.json)'
    )


def test_load_local_unknown_workflow_reference(tmp_path):
    put(tmp_path, "forms/contact.yaml", VALID_FORM)
    _, problems = load_local(tmp_path)
    assert [str(p) for p in problems] == [
        'forms/contact.yaml:workflow: unknown workflow "triage" (not found in workflows/)'
    ]


def test_load_local_accepts_crlf_and_bom(tmp_path):
    put(tmp_path, "forms/contact.yaml", b"\xef\xbb\xbf" + VALID_FORM.replace("\n", "\r\n").encode())
    put(tmp_path, "workflows/triage.yaml", VALID_WORKFLOW.replace("\n", "\r\n"))
    lb, problems = load_local(tmp_path)
    assert problems == [] and lb.bundle.forms[0].fields[0].key == "email"


def test_print_problems():
    buf = io.StringIO()
    print_problems(buf, [
        LocalProblem("forms/a.yaml", "fields[0].type", "bad type"),
        LocalProblem("forms/b.yaml", "", "yaml: line 2: did not find expected key"),
    ])  # fmt: skip
    assert buf.getvalue() == (
        "forms/a.yaml:fields[0].type: bad type\nforms/b.yaml: yaml: line 2: did not find expected key\n2 problem(s) found\n"
    )


def test_load_project_config(tmp_path):
    assert load_project_config(tmp_path) == ProjectConfig()
    put(tmp_path, CONFIG_FILE, "server: http://file:8080\ndir: defs\n")
    assert load_project_config(tmp_path) == ProjectConfig("http://file:8080", "defs")
    put(tmp_path, CONFIG_FILE, "servr: typo\n")
    with pytest.raises(CLIError, match=CONFIG_FILE):
        load_project_config(tmp_path)


def test_resolve_dir(tmp_path):
    d, _ = new_test_deps(tmp_path)
    assert d.resolve_dir("", ProjectConfig()) == tmp_path / "openforms"
    assert d.resolve_dir("", ProjectConfig(dir="defs")) == tmp_path / "defs"
    assert d.resolve_dir("flagdir", ProjectConfig(dir="defs")) == tmp_path / "flagdir"
    absolute = tmp_path / "elsewhere" / "x"
    assert d.resolve_dir(str(absolute), ProjectConfig()) == absolute


def test_resolve_remote_precedence(tmp_path):
    pc = ProjectConfig(server="http://file")
    d, _ = new_test_deps(tmp_path, {"OPENFORMS_URL": "http://env", "OPENFORMS_API_KEY": "ofk_env"})
    assert d.resolve_remote("http://flag", "ofk_flag", pc) == ("http://flag", "ofk_flag")
    assert d.resolve_remote("", "", pc) == ("http://env", "ofk_env")
    no_env, _ = new_test_deps(tmp_path, {"OPENFORMS_API_KEY": "ofk_env"})
    assert no_env.resolve_remote("", "", pc)[0] == "http://file"
    with pytest.raises(CLIError, match="no server configured"):
        no_env.resolve_remote("", "", ProjectConfig())
    no_key, _ = new_test_deps(tmp_path)
    with pytest.raises(CLIError, match="no API key"):
        no_key.resolve_remote("", "", pc)


def test_explain_remote_error():
    buf = io.StringIO()
    e = explain_remote_error(buf, APIError(401, "unauthenticated", "missing credentials"))
    assert "check --api-key or OPENFORMS_API_KEY" in str(e)
    e = explain_remote_error(buf, APIError(403, "forbidden", "admin only"))
    assert 'needs the "admin" role' in str(e)
    e = explain_remote_error(
        buf, APIError(422, "validation_failed", "invalid", [Problem("forms[0].title", "required")])
    )
    assert isinstance(e, ProblemsError) and "server:forms[0].title: required" in buf.getvalue()
    plain = RuntimeError("boom")
    assert explain_remote_error(buf, plain) is plain

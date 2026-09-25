import pytest

from openforms.definition import ValidationError, canonical, parse_form, parse_workflow, validate_bundle

from .helpers import base_form, base_workflow, problem_paths

JOB_APPLICATION_YAML = """
slug: job-application
title: Job application
description: Apply to join the team.
workflow: hiring
settings:
  public: true
  submitLabel: Send application
  confirmationMessage: Thanks! We'll be in touch.
fields:
  - key: name
    type: text
    label: Full name
    required: true
    placeholder: Ada Lovelace
    help: As on your passport
    validation: { minLength: 2, maxLength: 100 }
  - key: role
    type: select
    label: Role
    required: true
    options:
      - { value: engineer, label: Engineer }
      - { value: designer, label: Designer }
  - key: portfolio
    type: url
    label: Portfolio URL
    showIf: { field: role, equals: designer }
"""

JOB_APPLICATION_JSON = """{
  "fields": [
    {"key": "name", "type": "text", "label": "Full name", "required": true, "placeholder": "Ada Lovelace",
     "help": "As on your passport", "validation": {"maxLength": 100, "minLength": 2}},
    {"key": "role", "type": "select", "label": "Role", "required": true,
     "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
    {"key": "portfolio", "type": "url", "label": "Portfolio URL", "showIf": {"field": "role", "equals": "designer"}}
  ],
  "settings": {"confirmationMessage": "Thanks! We'll be in touch.", "submitLabel": "Send application", "public": true},
  "workflow": "hiring",
  "description": "Apply to join the team.",
  "title": "Job application",
  "slug": "job-application"
}"""

HIRING_YAML = """
slug: hiring
title: Hiring pipeline
initial: new
states:
  - { key: new, label: New, color: gray }
  - { key: screening, label: Screening, color: blue }
  - { key: interview, label: Interview, color: purple }
  - { key: hired, label: Hired, color: green, terminal: true }
  - { key: rejected, label: Rejected, color: red, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
onSubmit:
  - { type: assign, role: reviewer }
  - { type: email, to: "{{submission.data.email}}", subject: "We got your application", body: "Hi {{submission.data.name}}" }
transitions:
  - key: screen
    label: Start screening
    from: [new]
    to: screening
    guard: { roles: [reviewer] }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "https://example.com/hooks/interview" }
  - key: hire
    label: Hire
    from: [interview]
    to: hired
    guard: { roles: [hiring-manager] }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
"""


def test_parse_form_yaml():
    f = parse_form(JOB_APPLICATION_YAML)
    assert (f.slug, f.workflow, f.settings.public, len(f.fields)) == ("job-application", "hiring", True, 3)
    assert f.fields[0].validation.min_length == 2
    assert f.fields[2].show_if.equals == "designer"


def test_yaml_and_json_agree():
    assert canonical(parse_form(JOB_APPLICATION_YAML))[1] == canonical(parse_form(JOB_APPLICATION_JSON))[1]


def test_parse_workflow_yaml():
    w = parse_workflow(HIRING_YAML)
    assert (w.initial, len(w.states), len(w.transitions), len(w.on_submit)) == ("new", 5, 4, 2)
    tr = w.transition("invite")
    assert tr.guard.require_fields[0] == "score" and tr.actions[0].type == "webhook"


PROBLEM_CASES = [
    ("empty document", parse_form, "   \n", "", "empty"),
    ("syntax error", parse_form, "slug: [unclosed", "", "invalid YAML/JSON"),
    ("top level list", parse_form, "- a\n- b\n", "", ""),
    (
        "unknown property",
        parse_form,
        "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: X, colour: red }\n",
        "fields[0]",
        "colour",
    ),
    ("wrong type", parse_form, "slug: a\ntitle: 5\nfields:\n  - { key: x, type: text, label: X }\n", "title", ""),
    (
        "semantic after schema",
        parse_form,
        "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: X }\n  - { key: x, type: text, label: Second }\n",
        "fields[1].key",
        "duplicate",
    ),
    (
        "workflow schema",
        parse_workflow,
        "slug: w\ntitle: W\ninitial: a\nstates:\n  - { key: a, label: A, color: orange }\n",
        "states[0].color",
        "",
    ),
    (
        "workflow semantic",
        parse_workflow,
        "slug: w\ntitle: W\ninitial: b\nstates:\n  - { key: a, label: A }\n",
        "initial",
        "unknown state",
    ),
    ("missing fields", parse_form, "slug: a\ntitle: A\n", "", "missing property 'fields'"),
    ("empty fields", parse_form, "slug: a\ntitle: A\nfields: []\n", "fields", "minItems: got 0, want 1"),
]


@pytest.mark.parametrize("name,parse,doc,want_path,want_msg", PROBLEM_CASES, ids=[c[0] for c in PROBLEM_CASES])
def test_parse_problems(name, parse, doc, want_path, want_msg):
    with pytest.raises(ValidationError) as ei:
        parse(doc)
    assert want_path in [p.path for p in ei.value.problems], ei.value
    assert want_msg in str(ei.value)


def test_yaml_implicit_boolean():
    doc = "slug: a\ntitle: A\nfields:\n  - key: ok\n    type: select\n    label: OK?\n    options:\n      - { value: yes, label: Yes }\n"
    assert "fields[0].options[0].value" in problem_paths(parse_form, doc)


def test_yaml_y_is_a_boolean_like_go_yaml_v2():
    doc = "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: Y }\n"
    assert "fields[0].label" in problem_paths(parse_form, doc)


def test_yaml_dates_stay_strings():
    f = parse_form("slug: a\ntitle: 2024-01-01\nfields:\n  - { key: x, type: text, label: X }\n")
    assert f.title == "2024-01-01"


def test_rejects_duplicate_keys():
    doc = "slug: a\ntitle: A\ntitle: B\nfields:\n  - { key: x, type: text, label: X }\n"
    with pytest.raises(ValidationError) as ei:
        parse_form(doc)
    assert "" in [p.path for p in ei.value.problems] and "title" in str(ei.value)


def test_schema_key_is_dropped():
    f = parse_form(
        '{"$schema": "x", "slug": "a", "title": "A", "fields": [{"key": "x", "type": "text", "label": "X"}]}'
    )
    assert "$schema" not in f.to_dict()


def test_bundle_accepts_consistent_bundle():
    validate_bundle([base_form()], [base_workflow()])


def test_bundle_resolves_existing_workflows():
    validate_bundle([base_form()], [], ["hiring"])


def test_bundle_problems():
    bad = base_workflow()
    bad.initial = "draft"
    orphan = base_form()
    orphan.slug, orphan.workflow = "orphan", "missing"
    paths = problem_paths(validate_bundle, [base_form(), base_form(), orphan], [base_workflow(), base_workflow(), bad])
    for want in (
        "forms[1].slug",
        "forms[2].workflow",
        "workflows[1].slug",
        "workflows[2].slug",
        "workflows[2].initial",
    ):
        assert want in paths

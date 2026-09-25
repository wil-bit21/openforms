"""Port of ``internal/cli/yamlout_test.go`` plus a golden corpus produced by the Go CLI
(gopkg.in/yaml.v3) so the output stays byte-identical."""

import json
from pathlib import Path

import pytest

from openforms.cli.yamlout import canonical_yaml, go_format_float
from openforms.definition import (
    Action,
    Condition,
    Field,
    Form,
    FormSettings,
    Guard,
    Option,
    State,
    Transition,
    Validation,
    Workflow,
    WorkflowField,
    parse_form,
    parse_workflow,
)

GOLDEN = json.loads((Path(__file__).parent / "yaml_golden.json").read_text())


def y(v):
    return canonical_yaml(v.to_dict())


def test_matches_go_yaml_v3():
    failures = []
    for case in GOLDEN:
        try:
            got = canonical_yaml(case["doc"])
        except ValueError as e:
            got = f"ERR {e}"
        if got != case["yaml"]:
            failures.append((case["doc"], case["yaml"], got))
    assert not failures, failures[:3]


def test_key_order():
    f = Form(
        slug="contact",
        title="Contact us",
        settings=FormSettings(public=True),
        fields=[
            Field(key="name", type="text", label="Name", required=True),
            Field(key="topic", type="select", label="Topic", options=[Option("sales", "Sales")]),
        ],
    )
    assert y(f) == (
        "slug: contact\ntitle: Contact us\nsettings:\n  public: true\nfields:\n"
        "  - key: name\n    type: text\n    label: Name\n    required: true\n"
        "  - key: topic\n    type: select\n    label: Topic\n    options:\n"
        "      - value: sales\n        label: Sales\n"
    )


def test_workflow_key_order():
    w = Workflow(
        slug="triage",
        title="Triage",
        initial="new",
        states=[State("new", "New"), State("done", "Done", terminal=True)],
        transitions=[
            Transition(key="finish", label="Finish", from_=["new"], to="done", guard=Guard(roles=["support"]))
        ],
    )
    assert y(w) == (
        "slug: triage\ntitle: Triage\ninitial: new\nstates:\n  - key: new\n    label: New\n"
        "  - key: done\n    label: Done\n    terminal: true\ntransitions:\n  - key: finish\n"
        "    label: Finish\n    from:\n      - new\n    to: done\n    guard:\n      roles:\n        - support\n"
    )


def sample_form():
    return Form(
        slug="job",
        title="Job",
        description="Apply now",
        workflow="hiring",
        settings=FormSettings(public=True, submit_label="Send", confirmation_message="Thanks"),
        fields=[
            Field(
                key="name",
                type="text",
                label="Name",
                required=True,
                validation=Validation(min_length=2, max_length=100),
            ),
            Field(key="years", type="number", label="Years", validation=Validation(min=0.0, max=50.0)),
            Field(
                key="role",
                type="select",
                label="Role",
                options=[Option("engineer", "Engineer"), Option("designer", "Designer")],
            ),
            Field(key="portfolio", type="url", label="Portfolio", show_if=Condition(field="role", equals="designer")),
        ],
    )


def sample_workflow():
    return Workflow(
        slug="hiring",
        title="Hiring",
        initial="new",
        states=[State("new", "New", color="gray"), State("rejected", "Rejected", color="red", terminal=True)],
        fields=[WorkflowField(key="reason", type="textarea", label="Reason")],
        on_submit=[Action(type="assign", role="reviewer")],
        transitions=[
            Transition(
                key="reject",
                label="Reject",
                from_=["new"],
                to="rejected",
                guard=Guard(roles=["reviewer"], require_fields=["reason"]),
                actions=[
                    Action(
                        type="email",
                        to="{{submission.data.email}}",
                        subject="Update",
                        body="Hi {{submission.data.name}}",
                    ),
                    Action(type="webhook", url="https://example.com/hook"),
                ],
            )
        ],
    )


def test_round_trips_form():
    f = sample_form()
    assert parse_form(y(f)) == f


def test_round_trips_workflow():
    w = sample_workflow()
    assert parse_workflow(y(w)) == w


def test_is_idempotent():
    first = y(sample_workflow())
    assert y(parse_workflow(first)) == first


def test_keeps_ambiguous_strings():
    f = Form(
        slug="tricky",
        title="123",
        fields=[
            Field(
                key="answer",
                type="select",
                label="true",
                placeholder="null",
                options=[Option("yes", "on"), Option("no", "off"), Option("123", "1e3"), Option("null", "~")],
            ),
            Field(
                key="detail",
                type="text",
                label="{{submission.data.email}}",
                show_if=Condition(field="answer", in_=["yes", "123"]),
            ),
        ],
    )
    assert parse_form(y(f)) == f


@pytest.mark.parametrize(
    ("f", "want"),
    [(2.5, "2.5"), (1e21, "1e+21"), (1234567.5, "1.2345675e+06"), (1e-05, "1e-05"), (0.0001, "0.0001"),
     (100000.5, "100000.5"), (1 / 3, "0.3333333333333333"), (5e-324, "5e-324"), (float("inf"), ".inf")],
)  # fmt: skip
def test_go_format_float(f, want):
    assert go_format_float(f) == want


def test_integral_floats_render_as_integers():
    assert canonical_yaml({"n": [123456789.0, -0.0, 50.0]}) == '"n":\n  - 123456789\n  - 0\n  - 50\n'

import json

from openforms.definition import Field, Form, FormSettings, Problem, Problems, ValidationError, Workflow
from openforms.definition.common import validate_options
from openforms.definition.types import Option, State, Transition, WorkflowField

from .helpers import problem_paths


def test_form_json_shape():
    f = Form(
        slug="contact",
        title="Contact",
        settings=FormSettings(public=True),
        fields=[Field(key="name", type="text", label="Name", required=True)],
    )
    assert json.dumps(f.to_dict(), separators=(",", ":")) == (
        '{"slug":"contact","title":"Contact","settings":{"public":true},'
        '"fields":[{"key":"name","type":"text","label":"Name","required":true}]}'
    )


def test_workflow_lookups():
    w = Workflow(
        states=[State("new", "New"), State("done", "Done", terminal=True)],
        fields=[WorkflowField("score", "number", "Score")],
        transitions=[Transition("finish", "Finish", ["new"], "done")],
    )
    assert w.state("done").terminal
    assert w.state("missing") is None
    assert w.transition("finish").to == "done"
    assert w.transition("nope") is None
    assert w.field("score").type == "number"
    assert w.field("nope") is None


def test_validation_error_message():
    err = ValidationError([Problem("", "document is empty"), Problem("fields[0].key", "is required")])
    for want in ("document is empty", "fields[0].key", "is required"):
        assert want in str(err)
    assert str(ValidationError()) != ""


def test_problems_merge_and_join():
    ps = Problems()
    ps.merge("forms[0]", ValidationError([Problem("slug", "bad"), Problem("", "root")]))
    ps.merge("forms[1]", None)
    assert sorted(p.path for p in ps) == ["forms[0]", "forms[0].slug"]
    assert Problems().error() is None


def test_validate_options():
    ps = Problems()
    validate_options(ps, "fields[0]", True, "", [])
    validate_options(
        ps, "fields[1]", False, "options are only allowed on select and multiselect fields", [Option("a", "A")]
    )
    validate_options(
        ps, "fields[2]", True, "", [Option("a", "A"), Option("a", "Again"), Option(" b", "B"), Option("", "")]
    )
    paths = [p.path for p in ps]
    for want in (
        "fields[0].options",
        "fields[1].options",
        "fields[2].options[1].value",
        "fields[2].options[2].value",
        "fields[2].options[3].value",
        "fields[2].options[3].label",
    ):
        assert want in paths


def test_problem_paths_helper_empty_on_success():
    assert problem_paths(lambda: None) == []

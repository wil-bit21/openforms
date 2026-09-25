import json

import pytest

from openforms._paths import schemas_dir
from openforms.definition import (
    Condition,
    Field,
    Form,
    Option,
    ValidationError,
    form_from_json,
    validate_submission,
    visible,
)
from openforms.definition.values import json_equal


def _fixture(name):
    return json.loads((schemas_dir() / "fixtures" / name).read_text(encoding="utf-8"))


VIS = _fixture("visibility.json")
SUB = _fixture("submission.json")


@pytest.mark.parametrize("case", VIS, ids=[c["name"] for c in VIS])
def test_visibility_fixtures(case):
    assert visible(form_from_json(case["form"]), case["data"]) == case["visible"]


@pytest.mark.parametrize("case", SUB, ids=[c["name"] for c in SUB])
def test_submission_fixtures(case):
    form = form_from_json(case["form"])
    if case["clean"] is not None:
        clean = validate_submission(form, case["data"])
        assert json_equal(clean, case["clean"]), clean
        return
    with pytest.raises(ValidationError) as ei:
        validate_submission(form, case["data"])
    assert sorted(p.path for p in ei.value.problems) == sorted(case["errorPaths"])


def test_visible_none_data():
    f = Form(
        slug="t",
        title="T",
        fields=[
            Field(key="a", type="text", label="A"),
            Field(key="b", type="text", label="B", show_if=Condition(field="a", equals="x")),
        ],
    )
    assert visible(f, None) == {"a": True, "b": False}


def test_does_not_alias_input():
    f = Form(
        slug="t",
        title="T",
        fields=[Field(key="skills", type="multiselect", label="Skills", options=[Option("go", "Go")])],
    )
    data = {"skills": ["go"], "extra": 1}
    clean = validate_submission(f, data)
    clean["skills"][0] = "changed"
    assert data == {"skills": ["go"], "extra": 1}


def test_empty_clean_is_a_dict():
    f = Form(slug="t", title="T", fields=[Field(key="a", type="text", label="A")])
    assert validate_submission(f, {}) == {}


def test_problems_in_field_order():
    f = Form(
        slug="t",
        title="T",
        fields=[
            Field(key="z", type="text", label="Z", required=True),
            Field(key="a", type="text", label="A", required=True),
        ],
    )
    with pytest.raises(ValidationError) as ei:
        validate_submission(f, None)
    assert [p.path for p in ei.value.problems] == ["data.z", "data.a"]
    assert ei.value.problems[0].message == "is required"

import json

import pytest

from openforms._paths import schemas_dir
from openforms.definition.parse import schema_validator

SPEC_FORM = """{
  "$schema": "https://openforms.dev/schemas/form.schema.json",
  "slug": "job-application", "title": "Job application", "description": "Apply to join the team.",
  "workflow": "hiring",
  "settings": {"public": true, "submitLabel": "Send application", "confirmationMessage": "Thanks! We'll be in touch."},
  "fields": [
    {"key": "name", "type": "text", "label": "Full name", "required": true, "placeholder": "Ada Lovelace",
     "help": "As on your passport", "validation": {"minLength": 2, "maxLength": 100}},
    {"key": "role", "type": "select", "label": "Role", "required": true,
     "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
    {"key": "portfolio", "type": "url", "label": "Portfolio URL", "showIf": {"field": "role", "equals": "designer"}}
  ]
}"""

SPEC_WORKFLOW = """{
  "slug": "hiring", "title": "Hiring pipeline", "initial": "new",
  "states": [
    {"key": "new", "label": "New", "color": "gray"},
    {"key": "screening", "label": "Screening", "color": "blue"},
    {"key": "interview", "label": "Interview", "color": "purple"},
    {"key": "hired", "label": "Hired", "color": "green", "terminal": true},
    {"key": "rejected", "label": "Rejected", "color": "red", "terminal": true}
  ],
  "fields": [
    {"key": "score", "type": "number", "label": "Score"},
    {"key": "rejectionReason", "type": "textarea", "label": "Rejection reason"}
  ],
  "onSubmit": [
    {"type": "assign", "role": "reviewer"},
    {"type": "email", "to": "{{submission.data.email}}", "subject": "We got your application", "body": "Hi {{submission.data.name}}"}
  ],
  "transitions": [
    {"key": "screen", "label": "Start screening", "from": ["new"], "to": "screening", "guard": {"roles": ["reviewer"]}},
    {"key": "invite", "label": "Invite to interview", "from": ["screening"], "to": "interview",
     "guard": {"roles": ["reviewer"], "requireFields": ["score"]},
     "actions": [{"type": "webhook", "url": "https://example.com/hooks/interview"}]},
    {"key": "hire", "label": "Hire", "from": ["interview"], "to": "hired", "guard": {"roles": ["hiring-manager"]}},
    {"key": "reject", "label": "Reject", "from": ["new", "screening", "interview"], "to": "rejected",
     "guard": {"roles": ["reviewer", "hiring-manager"], "requireFields": ["rejectionReason"]},
     "actions": [{"type": "email", "to": "{{submission.data.email}}", "subject": "Your application", "body": "{{submission.fields.rejectionReason}}"}]}
  ]
}"""

BAD = [
    (
        "form unknown property",
        "form",
        '{"slug":"a","title":"A","fields":[{"key":"x","type":"text","label":"X","colour":"red"}]}',
    ),
    ("form missing fields", "form", '{"slug":"a","title":"A"}'),
    ("form empty fields", "form", '{"slug":"a","title":"A","fields":[]}'),
    ("form bad slug", "form", '{"slug":"A B","title":"A","fields":[{"key":"x","type":"text","label":"X"}]}'),
    ("form bad field type", "form", '{"slug":"a","title":"A","fields":[{"key":"x","type":"color","label":"X"}]}'),
    (
        "form option value not string",
        "form",
        '{"slug":"a","title":"A","fields":[{"key":"x","type":"select","label":"X","options":[{"value":true,"label":"Yes"}]}]}',
    ),
    (
        "workflow bad color",
        "workflow",
        '{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A","color":"orange"}]}',
    ),
    (
        "workflow email field type",
        "workflow",
        '{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"fields":[{"key":"e","type":"email","label":"E"}]}',
    ),
    (
        "workflow bad action type",
        "workflow",
        '{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"onSubmit":[{"type":"sms"}]}',
    ),
    (
        "workflow transition without from",
        "workflow",
        '{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"transitions":[{"key":"t","label":"T","from":[],"to":"a"}]}',
    ),
]


def test_schemas_have_ids():
    for name, sid in (
        ("form", "https://openforms.dev/schemas/form.schema.json"),
        ("workflow", "https://openforms.dev/schemas/workflow.schema.json"),
    ):
        assert schema_validator(name).schema["$id"] == sid


def test_spec_examples_are_valid():
    assert not list(schema_validator("form").iter_errors(json.loads(SPEC_FORM)))
    assert not list(schema_validator("workflow").iter_errors(json.loads(SPEC_WORKFLOW)))


@pytest.mark.parametrize("name,kind,doc", BAD, ids=[b[0] for b in BAD])
def test_schemas_reject_structural_errors(name, kind, doc):
    assert list(schema_validator(kind).iter_errors(json.loads(doc)))


def test_fixtures_are_well_formed():
    names = set()
    for file in ("visibility.json", "submission.json"):
        cases = json.loads((schemas_dir() / "fixtures" / file).read_text(encoding="utf-8"))
        assert cases
        for c in cases:
            assert c["name"] and (file, c["name"]) not in names
            names.add((file, c["name"]))
            assert not list(schema_validator("form").iter_errors(c["form"])), c["name"]
            assert isinstance(c["data"], dict)
            if file == "visibility.json":
                assert c["visible"]
            else:
                assert (c["clean"] is None) != (len(c["errorPaths"]) == 0)

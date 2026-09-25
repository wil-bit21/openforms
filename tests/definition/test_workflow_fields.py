import pytest

from openforms.definition import Option, State, ValidationError, Workflow, WorkflowField, validate_workflow_fields


def fields_workflow() -> Workflow:
    return Workflow(
        slug="w",
        title="W",
        initial="new",
        states=[State("new", "New")],
        fields=[
            WorkflowField("score", "number", "Score"),
            WorkflowField("rejectionReason", "textarea", "Reason"),
            WorkflowField("source", "select", "Source", [Option("referral", "Referral")]),
            WorkflowField("flagged", "checkbox", "Flagged"),
            WorkflowField("followUp", "date", "Follow up"),
        ],
    )


def test_normalizes():
    got = validate_workflow_fields(
        fields_workflow(),
        {
            "score": 4,
            "source": " referral ",
            "flagged": False,
            "followUp": "2026-10-01",
            "rejectionReason": None,
        },
    )
    assert got == {
        "score": 4,
        "source": "referral",
        "flagged": False,
        "followUp": "2026-10-01",
        "rejectionReason": None,
    }


def test_blank_means_unset():
    assert validate_workflow_fields(fields_workflow(), {"rejectionReason": "   "}) == {"rejectionReason": None}


def test_problems_sorted_by_key():
    with pytest.raises(ValidationError) as ei:
        validate_workflow_fields(
            fields_workflow(), {"zeta": 1, "alpha": "x", "score": "4", "source": "website", "flagged": "yes"}
        )
    probs = ei.value.problems
    assert [p.path for p in probs] == ["fields.alpha", "fields.flagged", "fields.score", "fields.source", "fields.zeta"]
    assert probs[0].message == "unknown workflow field"


def test_empty_patch():
    assert validate_workflow_fields(fields_workflow(), {}) == {}

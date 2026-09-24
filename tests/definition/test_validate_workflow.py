import pytest

from openforms.definition import Action, Option, State, Workflow, validate_workflow

from .helpers import base_workflow, problem_paths


def _set(obj, **kw):
    for k, v in kw.items():
        setattr(obj, k, v)


def _replace_on_submit0(w):
    w.on_submit[0] = Action(type="assign", user="ada")


CASES = [
    ("bad slug", lambda w: _set(w, slug="Hiring"), "slug"),
    ("empty title", lambda w: _set(w, title=""), "title"),
    ("no states", lambda w: _set(w, states=None, transitions=None), "states"),
    ("bad state key", lambda w: _set(w.states[0], key="1new"), "states[0].key"),
    ("duplicate state key", lambda w: _set(w.states[1], key="new"), "states[1].key"),
    ("empty state label", lambda w: _set(w.states[0], label=" "), "states[0].label"),
    ("bad color", lambda w: _set(w.states[0], color="orange"), "states[0].color"),
    ("unknown initial", lambda w: _set(w, initial="draft"), "initial"),
    ("email workflow field", lambda w: _set(w.fields[0], type="email"), "fields[0].type"),
    ("duplicate workflow field", lambda w: _set(w.fields[1], key="score"), "fields[1].key"),
    ("bad workflow field key", lambda w: _set(w.fields[0], key="sco re"), "fields[0].key"),
    ("empty workflow field label", lambda w: _set(w.fields[0], label=""), "fields[0].label"),
    ("select field without options", lambda w: _set(w.fields[2], options=[]), "fields[2].options"),
    ("options on number field", lambda w: _set(w.fields[0], options=[Option("a", "A")]), "fields[0].options"),
    ("duplicate transition key", lambda w: _set(w.transitions[1], key="screen"), "transitions[1].key"),
    ("bad transition key", lambda w: _set(w.transitions[0], key="start-screening"), "transitions[0].key"),
    ("empty transition label", lambda w: _set(w.transitions[0], label=""), "transitions[0].label"),
    ("empty from", lambda w: _set(w.transitions[0], from_=None), "transitions[0].from"),
    ("unknown from", lambda w: _set(w.transitions[0], from_=["draft"]), "transitions[0].from[0]"),
    ("unknown to", lambda w: _set(w.transitions[0], to="draft"), "transitions[0].to"),
    ("from terminal", lambda w: _set(w.transitions[0], from_=["hired"]), "transitions[0].from[0]"),
    ("empty role", lambda w: _set(w.transitions[0].guard, roles=[" "]), "transitions[0].guard.roles[0]"),
    (
        "unknown requireField",
        lambda w: _set(w.transitions[1].guard, require_fields=["rating"]),
        "transitions[1].guard.requireFields[0]",
    ),
    ("webhook without url", lambda w: _set(w.transitions[1].actions[0], url=""), "transitions[1].actions[0].url"),
    (
        "webhook ftp url",
        lambda w: _set(w.transitions[1].actions[0], url="ftp://example.com"),
        "transitions[1].actions[0].url",
    ),
    (
        "webhook with subject",
        lambda w: _set(w.transitions[1].actions[0], subject="x"),
        "transitions[1].actions[0].subject",
    ),
    ("email without to", lambda w: _set(w.transitions[3].actions[0], to=""), "transitions[3].actions[0].to"),
    (
        "email without subject",
        lambda w: _set(w.transitions[3].actions[0], subject=""),
        "transitions[3].actions[0].subject",
    ),
    (
        "email with url",
        lambda w: _set(w.transitions[3].actions[0], url="https://x.dev"),
        "transitions[3].actions[0].url",
    ),
    ("assign with user and role", lambda w: _set(w.on_submit[0], user="ada@example.com"), "onSubmit[0]"),
    ("assign with neither", lambda w: _set(w.on_submit[0], role=""), "onSubmit[0]"),
    ("assign bad user email", _replace_on_submit0, "onSubmit[0].user"),
    ("assign with body", lambda w: _set(w.on_submit[0], body="x"), "onSubmit[0].body"),
    ("unknown action type", lambda w: _set(w.on_submit[1], type="sms"), "onSubmit[1].type"),
]


def test_accepts_base():
    validate_workflow(base_workflow())


@pytest.mark.parametrize("name,mutate,want", CASES, ids=[c[0] for c in CASES])
def test_rules(name, mutate, want):
    w = base_workflow()
    mutate(w)
    assert want in problem_paths(validate_workflow, w)


def test_allows_no_transitions():
    validate_workflow(Workflow(slug="inbox", title="Inbox", initial="open", states=[State("open", "Open")]))

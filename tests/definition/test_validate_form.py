import pytest

from openforms.definition import Condition, Option, Validation, validate_form

from .helpers import base_form, problem_paths


def _set(obj, **kw):
    for k, v in kw.items():
        setattr(obj, k, v)


CASES = [
    ("bad slug", lambda f: _set(f, slug="Bad Slug"), "slug"),
    ("empty title", lambda f: _set(f, title="  "), "title"),
    ("bad workflow slug", lambda f: _set(f, workflow="Hiring!"), "workflow"),
    ("no fields", lambda f: _set(f, fields=None), "fields"),
    ("bad key", lambda f: _set(f.fields[0], key="1name"), "fields[0].key"),
    ("duplicate key", lambda f: _set(f.fields[1], key="name"), "fields[1].key"),
    ("empty label", lambda f: _set(f.fields[0], label=""), "fields[0].label"),
    ("unknown type", lambda f: _set(f.fields[0], type="color"), "fields[0].type"),
    ("select without options", lambda f: _set(f.fields[1], options=[]), "fields[1].options"),
    ("duplicate option value", lambda f: _set(f.fields[1].options[1], value="engineer"), "fields[1].options[1].value"),
    (
        "option value whitespace",
        lambda f: _set(f.fields[1].options[0], value=" engineer"),
        "fields[1].options[0].value",
    ),
    ("options on text", lambda f: _set(f.fields[0], options=[Option("a", "A")]), "fields[0].options"),
    ("negative minLength", lambda f: _set(f.fields[0].validation, min_length=-1), "fields[0].validation.minLength"),
    (
        "minLength above maxLength",
        lambda f: _set(f.fields[0].validation, min_length=10, max_length=5),
        "fields[0].validation.maxLength",
    ),
    ("min above max", lambda f: _set(f.fields[3].validation, min=60), "fields[3].validation.max"),
    ("minLength on number", lambda f: _set(f.fields[3].validation, min_length=1), "fields[3].validation.minLength"),
    ("pattern on number", lambda f: _set(f.fields[3].validation, pattern="x"), "fields[3].validation.pattern"),
    ("min on text", lambda f: _set(f.fields[0].validation, min=1), "fields[0].validation.min"),
    ("invalid pattern", lambda f: _set(f.fields[0].validation, pattern="("), "fields[0].validation.pattern"),
    (
        "backreference pattern (RE2)",
        lambda f: _set(f.fields[0].validation, pattern=r"(a)\1"),
        "fields[0].validation.pattern",
    ),
    ("self reference showIf", lambda f: _set(f.fields[2].show_if, field="portfolio"), "fields[2].showIf.field"),
    (
        "showIf later field",
        lambda f: _set(f.fields[0], show_if=Condition(field="role", equals="x")),
        "fields[0].showIf.field",
    ),
    ("showIf unknown field", lambda f: _set(f.fields[2].show_if, field="nope"), "fields[2].showIf.field"),
    ("showIf without operator", lambda f: _set(f.fields[2], show_if=Condition(field="role")), "fields[2].showIf"),
    (
        "showIf two operators",
        lambda f: _set(f.fields[2], show_if=Condition(field="role", equals="designer", not_equals="engineer")),
        "fields[2].showIf",
    ),
    ("showIf empty in", lambda f: _set(f.fields[2], show_if=Condition(field="role", in_=[])), "fields[2].showIf.in"),
]


def test_accepts_base():
    validate_form(base_form())


@pytest.mark.parametrize("name,mutate,want", CASES, ids=[c[0] for c in CASES])
def test_rules(name, mutate, want):
    f = base_form()
    mutate(f)
    assert want in problem_paths(validate_form, f)


def test_reports_all_problems():
    f = base_form()
    f.slug, f.title = "BAD", ""
    f.fields[0].key = "1x"
    paths = problem_paths(validate_form, f)
    for want in ("slug", "title", "fields[0].key"):
        assert want in paths


def test_validation_zero_values_are_kept():
    f = base_form()
    f.fields[0].validation = Validation(min_length=0)
    validate_form(f)
    assert f.to_dict()["fields"][0]["validation"] == {"minLength": 0}

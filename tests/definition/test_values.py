import pytest

from openforms.definition.types import Option, Validation
from openforms.definition.values import check_value, json_equal

OPTS = [Option("go", "Go"), Option("ts", "TypeScript")]

CASES = [
    ("nil", "text", None, None, None, False, ""),
    ("trim", "text", None, "  hi ", "hi", True, ""),
    ("blank", "text", None, "   ", None, False, ""),
    ("nbsp is trimmed like Go", "text", None, " hi ", "hi", True, ""),
    ("not string", "text", None, 5.0, None, False, "must be a string"),
    ("object for text", "textarea", None, {"a": 1.0}, None, False, "must be a string"),
    ("min length runes", "text", Validation(min_length=3), "éé", None, False, "must be at least 3 characters"),
    ("max length runes", "text", Validation(max_length=2), "👍👍", "👍👍", True, ""),
    ("max length exceeded", "text", Validation(max_length=2), "abc", None, False, "must be at most 2 characters"),
    ("pattern anchored", "text", Validation(pattern="[A-Z]{3}"), "ABCD", None, False, "must match the required format"),
    ("pattern ok", "text", Validation(pattern="[A-Z]{3}"), "ABC", "ABC", True, ""),
    ("bad pattern ignored", "text", Validation(pattern="("), "x", "x", True, ""),
    ("email ok", "email", None, "ada@example.com", "ada@example.com", True, ""),
    ("email display name", "email", None, "Ada <ada@example.com>", None, False, "must be a valid email address"),
    ("email angle brackets", "email", None, "<ada@example.com>", None, False, "must be a valid email address"),
    ("email no at", "email", None, "ada", None, False, "must be a valid email address"),
    ("email double dot", "email", None, "ada..x@example.com", None, False, "must be a valid email address"),
    ("url ok", "url", None, "https://example.com", "https://example.com", True, ""),
    ("url no scheme", "url", None, "example.com", None, False, "must be an absolute http(s) URL"),
    ("url ftp", "url", None, "ftp://example.com", None, False, "must be an absolute http(s) URL"),
    ("url space in host", "url", None, "http://exa mple.com", None, False, "must be an absolute http(s) URL"),
    ("date ok", "date", None, "2024-02-29", "2024-02-29", True, ""),
    ("date impossible", "date", None, "2023-02-29", None, False, "must be a date in YYYY-MM-DD format"),
    ("date short", "date", None, "2024-2-3", None, False, "must be a date in YYYY-MM-DD format"),
    ("number float", "number", None, 3.5, 3.5, True, ""),
    ("number int", "number", None, 42, 42, True, ""),
    ("number integral float", "number", None, 7.0, 7, True, ""),
    ("number string", "number", None, "5", None, False, "must be a number"),
    ("number bool", "number", None, True, None, False, "must be a number"),
    ("number below min", "number", Validation(min=1), 0.5, None, False, "must be at least 1"),
    ("number above max", "number", Validation(max=10), 10.5, None, False, "must be at most 10"),
    ("number fractional bound", "number", Validation(max=2.5), 3, None, False, "must be at most 2.5"),
    ("number at max", "number", Validation(max=10), 10.0, 10, True, ""),
    ("checkbox false", "checkbox", None, False, False, True, ""),
    ("checkbox string", "checkbox", None, "true", None, False, "must be true or false"),
    ("checkbox int", "checkbox", None, 1, None, False, "must be true or false"),
    ("select ok", "select", None, "go", "go", True, ""),
    ("select trimmed", "select", None, " go ", "go", True, ""),
    ("select bad", "select", None, "rust", None, False, "must be one of the options"),
    ("multi ok", "multiselect", None, ["ts", "go"], ["ts", "go"], True, ""),
    ("multi empty", "multiselect", None, [], None, False, ""),
    ("multi dup", "multiselect", None, ["go", "go"], None, False, "must not contain duplicates"),
    ("multi bad option", "multiselect", None, ["rust"], None, False, "must only contain valid options"),
    ("multi string", "multiselect", None, "go", None, False, "must be a list of options"),
    ("multi non-string item", "multiselect", None, [1.0], None, False, "must be a list of options"),
    ("unknown type", "color", None, "x", None, False, "unsupported field type"),
]


@pytest.mark.parametrize("name,typ,v,raw,want,provided,msg", CASES, ids=[c[0] for c in CASES])
def test_check_value(name, typ, v, raw, want, provided, msg):
    got, got_provided, got_msg = check_value(typ, OPTS, v, raw)
    assert (got_msg, got_provided) == (msg, provided)
    assert got == want and type(got) is type(want)


def test_multiselect_result_is_a_copy():
    raw = ["go"]
    got, _, _ = check_value("multiselect", OPTS, None, raw)
    assert got == raw and got is not raw


def test_json_equal():
    assert json_equal(18, 18.0)
    assert not json_equal("18", 18.0)
    assert not json_equal(True, 1)
    assert json_equal({"a": [1]}, {"a": [1.0]})

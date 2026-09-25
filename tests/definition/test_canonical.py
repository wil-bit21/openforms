import hashlib

import pytest

from openforms.definition import Field, Form, canonical


def test_sorts_keys_and_compacts():
    got, h = canonical({"b": 1, "a": {"d": True, "c": "<x> & y"}})
    want = b'{"a":{"c":"<x> & y","d":true},"b":1}'
    assert got == want
    assert h == hashlib.sha256(want).hexdigest()


def test_struct_and_map_agree():
    f = Form(slug="contact", title="Contact", fields=[Field(key="name", type="text", label="Name")])
    m = {
        "title": "Contact",
        "fields": [{"label": "Name", "type": "text", "key": "name"}],
        "slug": "contact",
        "settings": {"public": False},
    }
    assert canonical(f)[1] == canonical(m)[1]


def test_preserves_numbers():
    got, _ = canonical({"n": 0.1, "i": 42, "big": 12345678901234567, "f": 50.0})
    assert got == b'{"big":12345678901234567,"f":50,"i":42,"n":0.1}'


def test_rejects_unmarshalable():
    with pytest.raises((TypeError, ValueError)):
        canonical({"f": lambda: None})


def test_escapes_line_separators_like_go():
    assert canonical({"s": "a b"})[0] == b'{"s":"a\\u2028b"}'

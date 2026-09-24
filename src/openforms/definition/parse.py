"""Parsing YAML/JSON definitions: YAML 1.1 → JSON Schema → semantic rules.

The YAML loader mirrors ``sigs.k8s.io/yaml`` (go-yaml v2) as used by the Go
server: YAML 1.1 implicit booleans (``yes``, ``on``, ``y``…) become booleans,
timestamps stay strings, and duplicate mapping keys are rejected.
"""

from __future__ import annotations

import json
import re
from functools import lru_cache
from typing import Any

import yaml
from jsonschema import Draft202012Validator
from jsonschema.exceptions import ValidationError as SchemaError

from .._paths import schemas_dir
from .problems import Problem, ValidationError
from .types import Form, Workflow
from .validate_form import validate_form
from .validate_workflow import validate_workflow

FORM_SCHEMA_ID = "https://openforms.dev/schemas/form.schema.json"
WORKFLOW_SCHEMA_ID = "https://openforms.dev/schemas/workflow.schema.json"


class _Loader(yaml.SafeLoader):
    pass


# Drop the timestamp resolver (go-yaml v2 keeps timestamps as strings) and the
# default bool resolver, then add go-yaml v2's YAML 1.1 bool set (includes y/n).
_Loader.yaml_implicit_resolvers = {
    ch: [(tag, rx) for tag, rx in resolvers if tag not in ("tag:yaml.org,2002:timestamp", "tag:yaml.org,2002:bool")]
    for ch, resolvers in yaml.SafeLoader.yaml_implicit_resolvers.items()
}
_Loader.add_implicit_resolver(
    "tag:yaml.org,2002:bool",
    re.compile(r"^(?:y|Y|yes|Yes|YES|n|N|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF)$"),
    list("yYnNtTfFoO"),
)


def _bool(loader: yaml.SafeLoader, node: yaml.ScalarNode) -> bool:
    return str(loader.construct_scalar(node)).lower() in ("y", "yes", "true", "on")


_Loader.add_constructor("tag:yaml.org,2002:bool", _bool)


class _DuplicateKey(Exception):
    pass


def _mapping(loader: yaml.SafeLoader, node: yaml.MappingNode, deep: bool = False) -> dict[str, Any]:
    loader.flatten_mapping(node)
    out: dict[str, Any] = {}
    for key_node, value_node in node.value:
        key = loader.construct_object(key_node, deep=True)
        skey = _json_key(key)
        if skey in out:
            raise _DuplicateKey(f'line {key_node.start_mark.line + 1}: key "{skey}" already set in map')
        out[skey] = loader.construct_object(value_node, deep=True)
    return out


def _json_key(k: Any) -> str:
    if isinstance(k, bool):
        return "true" if k else "false"
    if k is None:
        return "null"
    return str(k)


_Loader.add_constructor("tag:yaml.org,2002:map", _mapping)


def load_yaml(raw: bytes | str) -> Any:
    """Parse one YAML (or JSON) document the way the Go server did. Raises ValueError."""
    text = raw.decode("utf-8") if isinstance(raw, bytes) else raw
    try:
        return yaml.load(text, Loader=_Loader)  # noqa: S506 - SafeLoader subclass
    except _DuplicateKey as e:
        raise ValueError(str(e)) from None
    except yaml.YAMLError as e:
        raise ValueError(" ".join(str(e).split())) from None


@lru_cache(maxsize=2)
def _validator(schema_id: str) -> Draft202012Validator:
    name = "form.schema.json" if schema_id == FORM_SCHEMA_ID else "workflow.schema.json"
    schema = json.loads((schemas_dir() / name).read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    return Draft202012Validator(schema)


def schema_validator(kind: str) -> Draft202012Validator:
    return _validator(FORM_SCHEMA_ID if kind == "form" else WORKFLOW_SCHEMA_ID)


def instance_path(tokens: Any) -> str:
    """``["fields", 0, "showIf"]`` → ``fields[0].showIf``."""
    out = ""
    for t in tokens:
        if isinstance(t, int):
            out += f"[{t}]"
        else:
            out += ("." if out else "") + str(t)
    return out


_JSON_TYPES = {bool: "boolean", int: "integer", float: "number", str: "string", list: "array", dict: "object"}


def _json_type(v: Any) -> str:
    if v is None:
        return "null"
    return _JSON_TYPES.get(type(v), type(v).__name__)


def _quote(v: Any) -> str:
    return "'" + str(v) + "'"


def _message(e: SchemaError) -> str:
    v = e.validator
    vv: Any = e.validator_value
    inst: Any = e.instance
    sch: Any = e.schema
    if v == "type":
        want = vv if isinstance(vv, str) else " or ".join(vv)
        return f"got {_json_type(inst)}, want {want}"
    if v == "required":
        missing = [p for p in vv if isinstance(inst, dict) and p not in inst]
        return ("missing property " if len(missing) == 1 else "missing properties ") + ", ".join(map(_quote, missing))
    if v == "additionalProperties" and isinstance(inst, dict):
        allowed = set((sch.get("properties") or {}).keys())
        extra = [k for k in inst if k not in allowed]
        return (
            ("additional property " if len(extra) == 1 else "additional properties ")
            + ", ".join(map(_quote, extra))
            + " not allowed"
        )
    if v == "enum":
        return "value must be one of " + ", ".join(_quote(x) for x in vv)
    if v in ("minItems", "maxItems", "minLength", "maxLength"):
        got = len(inst) if hasattr(inst, "__len__") else inst
        return f"{v}: got {got}, want {vv}"
    if v in ("minimum", "maximum"):
        return f"{v}: got {inst}, want {vv}"
    if v == "pattern":
        return f"{_quote(inst)} does not match pattern {_quote(vv)}"
    return e.message


def schema_problems(kind: str, instance: Any) -> list[Problem]:
    seen: set[Problem] = set()
    out: list[Problem] = []
    for e in schema_validator(kind).iter_errors(instance):
        leaves = [e] if not e.context else list(e.context)
        for leaf in leaves:
            p = Problem(instance_path(leaf.absolute_path), _message(leaf))
            if p not in seen:
                seen.add(p)
                out.append(p)
    out.sort(key=lambda p: p.path)
    return out


def _root(msg: str) -> ValidationError:
    return ValidationError([Problem("", msg)])


def decode_document(raw: bytes | str, kind: str) -> dict[str, Any]:
    """Load ``raw`` and validate it against the form or workflow schema.
    Returns the plain JSON object; raises ValidationError."""
    text = raw.decode("utf-8") if isinstance(raw, bytes) else raw
    if text.strip() == "":
        raise _root("document is empty")
    try:
        doc = load_yaml(text)
    except ValueError as e:
        raise _root(f"invalid YAML/JSON: {e}") from None
    try:
        json.dumps(doc, allow_nan=False)
    except (TypeError, ValueError) as e:
        raise _root(f"invalid JSON: {e}") from None
    problems = schema_problems(kind, doc)
    if problems:
        raise ValidationError(problems)
    assert isinstance(doc, dict)
    doc.pop("$schema", None)
    return doc


def parse_form(raw: bytes | str) -> Form:
    """Parse a YAML or JSON form, validate it against form.schema.json and the
    semantic rules. Raises ValidationError."""
    f = Form.from_dict(decode_document(raw, "form"))
    validate_form(f)
    return f


def parse_workflow(raw: bytes | str) -> Workflow:
    w = Workflow.from_dict(decode_document(raw, "workflow"))
    validate_workflow(w)
    return w


def form_from_json(obj: Any) -> Form:
    """Validate an already-decoded JSON value as a form (API request bodies)."""
    return parse_form(json.dumps(obj))


def workflow_from_json(obj: Any) -> Workflow:
    return parse_workflow(json.dumps(obj))

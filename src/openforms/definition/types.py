"""Form and workflow definition types (spec §5).

The ``to_dict`` methods reproduce the Go server's JSON encoding exactly
(field order and ``omitempty`` rules), so canonical hashes stay stable across
the Go → Python migration.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any

FIELD_TEXT = "text"
FIELD_TEXTAREA = "textarea"
FIELD_EMAIL = "email"
FIELD_NUMBER = "number"
FIELD_SELECT = "select"
FIELD_MULTISELECT = "multiselect"
FIELD_CHECKBOX = "checkbox"
FIELD_DATE = "date"
FIELD_URL = "url"

ACTION_WEBHOOK = "webhook"
ACTION_EMAIL = "email"
ACTION_ASSIGN = "assign"


def _num(v: Any) -> Any:
    """Go marshals float64 50.0 as ``50``: integral floats become ints."""
    if isinstance(v, float) and v.is_integer() and abs(v) < 1e21:
        return int(v)
    return v


def _put(d: dict[str, Any], key: str, value: Any) -> None:
    """``omitempty``: skip "", False, None, empty lists and dicts (0 is kept for pointers)."""
    if value is None or value is False or value == "" or (isinstance(value, (list, dict)) and not value):
        return
    d[key] = value


def _str(d: dict[str, Any], key: str) -> str:
    v = d.get(key)
    return v if isinstance(v, str) else ""


def _list(d: dict[str, Any], key: str) -> list[Any] | None:
    v = d.get(key)
    return list(v) if isinstance(v, list) else None


@dataclass
class Option:
    value: str = ""
    label: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Option:
        return cls(value=_str(d, "value"), label=_str(d, "label"))

    def to_dict(self) -> dict[str, Any]:
        return {"value": self.value, "label": self.label}


@dataclass
class Validation:
    min_length: int | None = None
    max_length: int | None = None
    pattern: str = ""
    min: float | None = None
    max: float | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Validation:
        return cls(
            min_length=d.get("minLength"),
            max_length=d.get("maxLength"),
            pattern=_str(d, "pattern"),
            min=d.get("min"),
            max=d.get("max"),
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {}
        if self.min_length is not None:
            out["minLength"] = self.min_length
        if self.max_length is not None:
            out["maxLength"] = self.max_length
        _put(out, "pattern", self.pattern)
        if self.min is not None:
            out["min"] = _num(self.min)
        if self.max is not None:
            out["max"] = _num(self.max)
        return out


@dataclass
class Condition:
    """Controls field visibility. Exactly one of equals, not_equals, in_ is set
    (``None`` means unset, like a nil ``any`` in Go)."""

    field: str = ""
    equals: Any = None
    not_equals: Any = None
    in_: list[Any] | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Condition:
        return cls(field=_str(d, "field"), equals=d.get("equals"), not_equals=d.get("notEquals"), in_=_list(d, "in"))

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"field": self.field}
        if self.equals is not None:
            out["equals"] = _num(self.equals)
        if self.not_equals is not None:
            out["notEquals"] = _num(self.not_equals)
        if self.in_:
            out["in"] = [_num(v) for v in self.in_]
        return out


@dataclass
class Field:
    key: str = ""
    type: str = ""
    label: str = ""
    help: str = ""
    placeholder: str = ""
    required: bool = False
    options: list[Option] = field(default_factory=list)
    validation: Validation | None = None
    show_if: Condition | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Field:
        v = d.get("validation")
        s = d.get("showIf")
        return cls(
            key=_str(d, "key"),
            type=_str(d, "type"),
            label=_str(d, "label"),
            help=_str(d, "help"),
            placeholder=_str(d, "placeholder"),
            required=d.get("required") is True,
            options=[Option.from_dict(o) for o in d.get("options") or [] if isinstance(o, dict)],
            validation=Validation.from_dict(v) if isinstance(v, dict) else None,
            show_if=Condition.from_dict(s) if isinstance(s, dict) else None,
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"key": self.key, "type": self.type, "label": self.label}
        _put(out, "help", self.help)
        _put(out, "placeholder", self.placeholder)
        _put(out, "required", self.required)
        _put(out, "options", [o.to_dict() for o in self.options])
        if self.validation is not None:
            out["validation"] = self.validation.to_dict()
        if self.show_if is not None:
            out["showIf"] = self.show_if.to_dict()
        return out


@dataclass
class FormSettings:
    public: bool = False
    submit_label: str = ""
    confirmation_message: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> FormSettings:
        return cls(
            public=d.get("public") is True,
            submit_label=_str(d, "submitLabel"),
            confirmation_message=_str(d, "confirmationMessage"),
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"public": self.public}
        _put(out, "submitLabel", self.submit_label)
        _put(out, "confirmationMessage", self.confirmation_message)
        return out


@dataclass
class Form:
    slug: str = ""
    title: str = ""
    description: str = ""
    workflow: str = ""
    settings: FormSettings = field(default_factory=FormSettings)
    fields: list[Field] | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Form:
        s = d.get("settings")
        fs = d.get("fields")
        return cls(
            slug=_str(d, "slug"),
            title=_str(d, "title"),
            description=_str(d, "description"),
            workflow=_str(d, "workflow"),
            settings=FormSettings.from_dict(s) if isinstance(s, dict) else FormSettings(),
            fields=[Field.from_dict(f) for f in fs if isinstance(f, dict)] if isinstance(fs, list) else None,
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"slug": self.slug, "title": self.title}
        _put(out, "description", self.description)
        _put(out, "workflow", self.workflow)
        out["settings"] = self.settings.to_dict()
        out["fields"] = None if self.fields is None else [f.to_dict() for f in self.fields]
        return out

    def field_list(self) -> list[Field]:
        return self.fields or []


@dataclass
class State:
    key: str = ""
    label: str = ""
    color: str = ""
    terminal: bool = False

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> State:
        return cls(
            key=_str(d, "key"), label=_str(d, "label"), color=_str(d, "color"), terminal=d.get("terminal") is True
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"key": self.key, "label": self.label}
        _put(out, "color", self.color)
        _put(out, "terminal", self.terminal)
        return out


@dataclass
class WorkflowField:
    key: str = ""
    type: str = ""
    label: str = ""
    options: list[Option] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> WorkflowField:
        return cls(
            key=_str(d, "key"),
            type=_str(d, "type"),
            label=_str(d, "label"),
            options=[Option.from_dict(o) for o in d.get("options") or [] if isinstance(o, dict)],
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"key": self.key, "type": self.type, "label": self.label}
        _put(out, "options", [o.to_dict() for o in self.options])
        return out


@dataclass
class Guard:
    roles: list[str] = field(default_factory=list)
    require_fields: list[str] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Guard:
        return cls(roles=list(d.get("roles") or []), require_fields=list(d.get("requireFields") or []))

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {}
        _put(out, "roles", list(self.roles))
        _put(out, "requireFields", list(self.require_fields))
        return out


@dataclass
class Action:
    type: str = ""
    url: str = ""
    to: str = ""
    subject: str = ""
    body: str = ""
    user: str = ""
    role: str = ""

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Action:
        return cls(**{k: _str(d, k) for k in ("type", "url", "to", "subject", "body", "user", "role")})

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"type": self.type}
        for k in ("url", "to", "subject", "body", "user", "role"):
            _put(out, k, getattr(self, k))
        return out


@dataclass
class Transition:
    key: str = ""
    label: str = ""
    from_: list[str] | None = None
    to: str = ""
    guard: Guard = field(default_factory=Guard)
    actions: list[Action] = field(default_factory=list)

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Transition:
        g = d.get("guard")
        return cls(
            key=_str(d, "key"),
            label=_str(d, "label"),
            from_=_list(d, "from"),
            to=_str(d, "to"),
            guard=Guard.from_dict(g) if isinstance(g, dict) else Guard(),
            actions=[Action.from_dict(a) for a in d.get("actions") or [] if isinstance(a, dict)],
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"key": self.key, "label": self.label, "from": self.from_, "to": self.to}
        out["guard"] = self.guard.to_dict()
        _put(out, "actions", [a.to_dict() for a in self.actions])
        return out

    def sources(self) -> list[str]:
        return self.from_ or []


@dataclass
class Workflow:
    slug: str = ""
    title: str = ""
    initial: str = ""
    states: list[State] | None = None
    fields: list[WorkflowField] = field(default_factory=list)
    on_submit: list[Action] = field(default_factory=list)
    transitions: list[Transition] | None = None

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Workflow:
        st = d.get("states")
        tr = d.get("transitions")
        return cls(
            slug=_str(d, "slug"),
            title=_str(d, "title"),
            initial=_str(d, "initial"),
            states=[State.from_dict(s) for s in st if isinstance(s, dict)] if isinstance(st, list) else None,
            fields=[WorkflowField.from_dict(f) for f in d.get("fields") or [] if isinstance(f, dict)],
            on_submit=[Action.from_dict(a) for a in d.get("onSubmit") or [] if isinstance(a, dict)],
            transitions=[Transition.from_dict(t) for t in tr if isinstance(t, dict)] if isinstance(tr, list) else None,
        )

    def to_dict(self) -> dict[str, Any]:
        out: dict[str, Any] = {"slug": self.slug, "title": self.title, "initial": self.initial}
        out["states"] = None if self.states is None else [s.to_dict() for s in self.states]
        _put(out, "fields", [f.to_dict() for f in self.fields])
        _put(out, "onSubmit", [a.to_dict() for a in self.on_submit])
        out["transitions"] = None if self.transitions is None else [t.to_dict() for t in self.transitions]
        return out

    def state(self, key: str) -> State | None:
        return next((s for s in self.states or [] if s.key == key), None)

    def transition(self, key: str) -> Transition | None:
        return next((t for t in self.transitions or [] if t.key == key), None)

    def field(self, key: str) -> WorkflowField | None:
        return next((f for f in self.fields if f.key == key), None)

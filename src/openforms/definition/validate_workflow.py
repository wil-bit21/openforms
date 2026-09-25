"""Semantic validation of workflows (spec §5.3)."""

from __future__ import annotations

from .common import KEY_RULE, SLUG_RULE, blank, is_bare_email, is_http_url, is_key, is_slug, trim, validate_options
from .problems import Problems
from .types import ACTION_ASSIGN, ACTION_EMAIL, ACTION_WEBHOOK, FIELD_SELECT, Action, State, Workflow

WORKFLOW_FIELD_TYPES = {"text", "textarea", "number", "select", "checkbox", "date"}
STATE_COLORS = {"gray", "blue", "green", "yellow", "red", "purple"}


def validate_workflow(w: Workflow) -> None:
    """Raise ValidationError listing every problem in ``w``."""
    ps = Problems()
    if not is_slug(w.slug):
        ps.add("slug", SLUG_RULE)
    if blank(w.title):
        ps.add("title", "is required")

    states_list = w.states or []
    if not states_list:
        ps.add("states", "at least one state is required")
    states: dict[str, State] = {}
    for i, s in enumerate(states_list):
        p = f"states[{i}]"
        if not is_key(s.key):
            ps.add(p + ".key", KEY_RULE)
        elif s.key in states:
            ps.add(p + ".key", f'duplicate state key "{s.key}"')
        else:
            states[s.key] = s
        if blank(s.label):
            ps.add(p + ".label", "is required")
        if s.color and s.color not in STATE_COLORS:
            ps.add(p + ".color", "must be one of gray, blue, green, yellow, red, purple")
    if w.initial not in states:
        ps.add("initial", f'unknown state "{w.initial}"')

    fields: set[str] = set()
    for i, f in enumerate(w.fields):
        p = f"fields[{i}]"
        if not is_key(f.key):
            ps.add(p + ".key", KEY_RULE)
        elif f.key in fields:
            ps.add(p + ".key", f'duplicate workflow field key "{f.key}"')
        else:
            fields.add(f.key)
        if f.type not in WORKFLOW_FIELD_TYPES:
            ps.add(
                p + ".type",
                f'type "{f.type}" is not allowed for workflow fields (use text, textarea, number, select, checkbox or date)',
            )
        if blank(f.label):
            ps.add(p + ".label", "is required")
        validate_options(ps, p, f.type == FIELD_SELECT, "options are only allowed on select fields", f.options)

    for j, a in enumerate(w.on_submit):
        validate_action(ps, f"onSubmit[{j}]", a)

    seen: set[str] = set()
    for i, t in enumerate(w.transitions or []):
        p = f"transitions[{i}]"
        if not is_key(t.key):
            ps.add(p + ".key", KEY_RULE)
        elif t.key in seen:
            ps.add(p + ".key", f'duplicate transition key "{t.key}"')
        else:
            seen.add(t.key)
        if blank(t.label):
            ps.add(p + ".label", "is required")
        if not t.sources():
            ps.add(p + ".from", "at least one source state is required")
        for j, src in enumerate(t.sources()):
            fp = f"{p}.from[{j}]"
            s = states.get(src)
            if s is None:
                ps.add(fp, f'unknown state "{src}"')
            elif s.terminal:
                ps.add(fp, f'cannot transition out of terminal state "{src}"')
        if t.to not in states:
            ps.add(p + ".to", f'unknown state "{t.to}"')
        for j, r in enumerate(t.guard.roles):
            if blank(r):
                ps.add(f"{p}.guard.roles[{j}]", "must not be empty")
        for j, rf in enumerate(t.guard.require_fields):
            if rf not in fields:
                ps.add(f"{p}.guard.requireFields[{j}]", f'unknown workflow field "{rf}"')
        for j, a in enumerate(t.actions):
            validate_action(ps, f"{p}.actions[{j}]", a)
    ps.raise_if_any()


def validate_action(ps: Problems, p: str, a: Action) -> None:
    """Per-type action rules (spec §5.2)."""

    def not_allowed(name: str) -> None:
        if getattr(a, name):
            ps.add(f"{p}.{name}", f"not allowed on {a.type} actions")

    if a.type == ACTION_WEBHOOK:
        if not a.url:
            ps.add(p + ".url", "is required")
        elif not is_http_url(a.url):
            ps.add(p + ".url", "must be an absolute http(s) URL")
        for n in ("to", "subject", "body", "user", "role"):
            not_allowed(n)
    elif a.type == ACTION_EMAIL:
        if not trim(a.to):
            ps.add(p + ".to", "is required")
        if not trim(a.subject):
            ps.add(p + ".subject", "is required")
        for n in ("url", "user", "role"):
            not_allowed(n)
    elif a.type == ACTION_ASSIGN:
        if (a.user == "") == (a.role == ""):
            ps.add(p, "exactly one of user or role is required")
        if a.user and not is_bare_email(a.user):
            ps.add(p + ".user", "must be an email address")
        for n in ("url", "to", "subject", "body"):
            not_allowed(n)
    else:
        ps.add(p + ".type", f'unknown action type "{a.type}"')

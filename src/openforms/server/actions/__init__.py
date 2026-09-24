"""Workflow side effects (webhook, email, assign) run as jobs (port of the Go ``internal/actions``)."""

from .payload import (
    KIND_ASSIGN,
    KIND_EMAIL,
    KIND_WEBHOOK,
    SYSTEM_ACTOR_NAME,
    TRIGGER_SUBMIT,
    Payload,
    enqueue,
    kind_for,
)
from .render import render
from .runner import ActionDeps, register
from .vars import template_vars

__all__ = [
    "KIND_ASSIGN",
    "KIND_EMAIL",
    "KIND_WEBHOOK",
    "SYSTEM_ACTOR_NAME",
    "TRIGGER_SUBMIT",
    "ActionDeps",
    "Payload",
    "enqueue",
    "kind_for",
    "register",
    "render",
    "template_vars",
]

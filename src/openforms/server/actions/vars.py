"""Variables available to action templates (spec §6.9)."""

from __future__ import annotations

from typing import Any

from ...definition import Form, Transition
from ...settings import Settings
from ..models.submissions import Submission


def template_vars(settings: Settings, form: Form, sub: Submission, transition: Transition | None) -> dict[str, Any]:
    base = settings.base_url.rstrip("/")
    tr: dict[str, Any] = {}
    if transition is not None:
        tr = {"key": transition.key, "label": transition.label}
    return {
        "baseUrl": base,
        "form": {"slug": form.slug, "title": form.title},
        "transition": tr,
        "submission": {
            "id": str(sub.id),
            "state": sub.state,
            "stateLabel": sub.state_label,
            "data": sub.data or {},
            "fields": sub.fields or {},
            "url": f"{base}/admin/submissions/{sub.id}",
        },
    }

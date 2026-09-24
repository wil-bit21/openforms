"""Per-process application state shared by the routers."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

from starlette.requests import Request

from ...settings import Settings
from ..database.engine import Database


@dataclass
class AppContext:
    settings: Settings
    db: Database
    org_id: uuid.UUID
    ui_dir: Path | None = None
    # Filled in by later layers (definitions cache, job queue, workflow engine…).
    services: dict[str, Any] = field(default_factory=dict)


def get_ctx(request: Request) -> AppContext:
    return request.app.state.ctx

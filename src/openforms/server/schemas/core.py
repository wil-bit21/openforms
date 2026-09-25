"""Domain objects returned by the model layer."""

from __future__ import annotations

import datetime as dt
import uuid
from dataclasses import dataclass, field

ROLE_ADMIN = "admin"
PRINCIPAL_USER = "user"
PRINCIPAL_API_KEY = "api_key"


@dataclass(frozen=True)
class Principal:
    """The authenticated caller of a request."""

    org_id: uuid.UUID
    kind: str
    id: uuid.UUID
    name: str = ""
    email: str = ""
    roles: tuple[str, ...] = ()

    def is_admin(self) -> bool:
        return ROLE_ADMIN in self.roles

    def has_any_role(self, *roles: str) -> bool:
        """True if ``roles`` is empty, the principal is admin, or any role matches."""
        return not roles or self.is_admin() or any(r in self.roles for r in roles)


@dataclass
class User:
    id: uuid.UUID
    org_id: uuid.UUID
    email: str
    name: str
    roles: list[str] = field(default_factory=list)
    created_at: dt.datetime | None = None


@dataclass
class ApiKey:
    id: uuid.UUID
    org_id: uuid.UUID
    name: str
    prefix: str
    roles: list[str] = field(default_factory=list)
    created_at: dt.datetime | None = None
    last_used_at: dt.datetime | None = None
    revoked_at: dt.datetime | None = None

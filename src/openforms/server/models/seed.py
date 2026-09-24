"""The demo bundle and demo users (port of Go ``internal/seed``)."""

from __future__ import annotations

import uuid
from dataclasses import dataclass, field

from ...examples import bundle
from ..database.engine import Database
from ..exceptions import EmailTaken, NotFound
from ..schemas.core import ROLE_ADMIN
from . import auth
from . import definitions as defs

DEMO_PASSWORD = "demo1234"


@dataclass(frozen=True)
class DemoUser:
    email: str
    name: str
    roles: tuple[str, ...]


DEMO_USERS = (
    DemoUser("admin@demo.local", "Avery Admin", (ROLE_ADMIN,)),
    DemoUser("reviewer@demo.local", "Riley Reviewer", ("reviewer",)),
    DemoUser("manager@demo.local", "Morgan Manager", ("hiring-manager",)),
)


@dataclass
class SeedResult:
    apply: defs.ApplyResult
    users_created: list[str] = field(default_factory=list)


async def seed_demo(db: Database, org_id: uuid.UUID) -> SeedResult:
    """Apply the example bundle (source "seed") and create missing demo users.

    Idempotent: unchanged definitions create no versions and existing users are left untouched."""
    forms, workflows = bundle()
    async with db.transaction() as s:
        res = SeedResult(
            apply=await defs.apply(
                s, org_id, defs.ApplyInput(forms=forms, workflows=workflows, source=defs.SOURCE_SEED, actor="seed")
            )
        )
    for du in DEMO_USERS:
        async with db.transaction() as s:
            try:
                await auth.get_user_by_email(s, org_id, du.email)
                continue
            except NotFound:
                pass
            try:
                await auth.create_user(s, org_id, du.email, du.name, DEMO_PASSWORD, list(du.roles))
            except EmailTaken:
                continue  # created concurrently by another instance
        res.users_created.append(du.email)
    return res

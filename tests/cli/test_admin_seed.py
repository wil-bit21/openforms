"""Ports of ``admin_test.go`` and ``cli/seed_test.go``."""

from contextlib import asynccontextmanager

import pytest

from openforms.cli import commands
from openforms.cli.deps import CLIError
from openforms.server.models import auth
from tests.cli.helpers import new_test_deps


@pytest.fixture
def admin_deps(database, org_id, tmp_path):
    d, tio = new_test_deps(tmp_path)

    @asynccontextmanager
    async def open_auth():
        yield database, org_id

    d.open_auth = open_auth
    return d, tio


def test_split_roles():
    assert commands.split_roles(" admin, reviewer ,,") == ["admin", "reviewer"]
    assert commands.split_roles("") == []


async def test_create_user(admin_deps, database, org_id):
    d, tio = admin_deps
    await commands.admin_create_user(d, "Ada@Example.com", "", "correct-horse", "admin,reviewer")
    async with database.session() as s:
        u = await auth.get_user_by_email(s, org_id, "ada@example.com")
    assert u.name == "Ada" and list(u.roles) == ["admin", "reviewer"]
    assert tio.out.getvalue() == f"Created user ada@example.com ({u.id}) with roles [admin, reviewer]\n"


async def test_create_user_duplicate(admin_deps):
    d, _ = admin_deps
    await commands.admin_create_user(d, "bob@example.com", "", "password123", "")
    with pytest.raises(CLIError, match="a user with email bob@example.com already exists"):
        await commands.admin_create_user(d, "bob@example.com", "", "password123", "")


async def test_create_user_requires_flags(tmp_path):
    d, _ = new_test_deps(tmp_path)  # open_auth fails if reached
    with pytest.raises(CLIError, match="--email and --password are required"):
        await commands.admin_create_user(d, "x@example.com", "", "", "")


async def test_create_api_key(admin_deps, database):
    d, tio = admin_deps
    await commands.admin_create_api_key(d, "ci", "admin")
    lines = tio.out.getvalue().splitlines()
    key = next(line for line in lines if line.startswith("ofk_"))
    assert len(key) == 44
    async with database.session() as s:
        p = await auth.principal_from_api_key(s, key)
    assert p.is_admin() and p.name == "ci"
    assert lines[0].startswith('Created API key "ci" (prefix ') and lines[0].endswith(") with roles [admin]")
    assert "it will not be shown again" in tio.out.getvalue()


async def test_create_api_key_requires_name(tmp_path):
    d, _ = new_test_deps(tmp_path)
    with pytest.raises(CLIError, match="--name is required"):
        await commands.admin_create_api_key(d, "", "admin")


async def test_seed_demo_is_idempotent(admin_deps):
    d, tio = admin_deps
    await commands.run_seed_demo(d)
    out = tio.out.getvalue()
    for want in ("job-application", "hiring", "contact-triage", "created", "reviewer@demo.local", "demo1234"):
        assert want in out
    assert "workflow  hiring               v1    created\n" in out
    assert "user      reviewer@demo.local  password demo1234\n" in out
    tio.reset()
    await commands.run_seed_demo(d)
    out = tio.out.getvalue()
    assert "created" not in out and "updated" not in out
    assert "demo users already exist" in out

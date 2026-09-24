import re
import uuid

import pytest
import sqlalchemy as sa

from openforms.definition import ValidationError
from openforms.server.exceptions import EmailTaken, InvalidCredentials, NotFound, Unauthenticated
from openforms.server.models import auth
from openforms.server.schemas.core import Principal


def paths(ei) -> list[str]:
    return [p.path for p in ei.value.problems]


async def test_ensure_default_org_is_idempotent(database, org_id):
    async with database.transaction() as s:
        assert await auth.ensure_default_org(s) == org_id


async def test_create_user_normalises_and_rejects_duplicates(database, org_id):
    async with database.transaction() as s:
        u = await auth.create_user(
            s, org_id, "  Ada@Example.com ", " Ada ", "password123", ["reviewer", "admin", "reviewer"]
        )
    assert (u.email, u.name, u.org_id, u.roles) == ("ada@example.com", "Ada", org_id, ["reviewer", "admin"])
    with pytest.raises(EmailTaken):
        async with database.transaction() as s:
            await auth.create_user(s, org_id, "ADA@example.com", "Other", "password123", None)


async def test_duplicate_does_not_poison_the_transaction(database, org_id):
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "a@example.com", "A", "password123", None)
        with pytest.raises(EmailTaken):
            await auth.create_user(s, org_id, "A@example.com", "A2", "password123", None)
        await auth.create_user(s, org_id, "b@example.com", "B", "password123", None)
    async with database.session() as s:
        assert len(await auth.list_users(s, org_id)) == 2


async def test_create_user_validation(database, org_id):
    with pytest.raises(ValidationError) as ei:
        async with database.transaction() as s:
            await auth.create_user(s, org_id, "not-an-email", " ", "short", ["Bad Role"])
    assert paths(ei) == ["email", "name", "password", "roles[0]"]


async def test_list_get_and_users_with_role(database, org_id):
    async with database.transaction() as s:
        a = await auth.create_user(s, org_id, "a@example.com", "A", "password123", ["reviewer"])
        await auth.create_user(s, org_id, "b@example.com", "B", "password123", ["hiring-manager"])
    async with database.session() as s:
        assert len(await auth.list_users(s, org_id)) == 2
        assert (await auth.get_user_by_email(s, org_id, "A@EXAMPLE.COM")).id == a.id
        with pytest.raises(NotFound):
            await auth.get_user_by_email(s, org_id, "nobody@example.com")
        assert [u.id for u in await auth.users_with_role(s, org_id, "reviewer")] == [a.id]


async def test_update_user(database, org_id):
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "root@example.com", "Root", "password123", ["admin"])
        u = await auth.create_user(s, org_id, "ada@example.com", "Ada", "password123", ["reviewer"])
    async with database.transaction() as s:
        got = await auth.update_user(s, org_id, u.id, name="Ada L.", password="newpassword1", roles=["hiring-manager"])
    assert got.name == "Ada L." and got.roles == ["hiring-manager"]
    async with database.transaction() as s:
        got = await auth.update_user(s, org_id, u.id)
    assert got.roles == ["hiring-manager"] and got.name == "Ada L."
    with pytest.raises(NotFound):
        async with database.transaction() as s:
            await auth.update_user(s, org_id, uuid.uuid4(), name="x")
    with pytest.raises(ValidationError) as ei:
        async with database.transaction() as s:
            await auth.update_user(s, org_id, u.id, name=" ")
    assert paths(ei)[0] == "name"


async def test_last_admin_cannot_be_demoted_or_deleted(database, org_id):
    async with database.transaction() as s:
        root = await auth.create_user(s, org_id, "root@example.com", "Root", "password123", ["admin"])
    with pytest.raises(ValidationError) as ei:
        async with database.transaction() as s:
            await auth.update_user(s, org_id, root.id, roles=["reviewer"])
    assert paths(ei)[0] == "roles"
    with pytest.raises(ValidationError) as ei:
        async with database.transaction() as s:
            await auth.delete_user(s, org_id, root.id)
    assert paths(ei)[0] == "id"
    async with database.session() as s:
        assert (await auth.get_user_by_email(s, org_id, "root@example.com")).roles == ["admin"]
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "second@example.com", "Second", "password123", ["admin"])
    async with database.transaction() as s:
        await auth.update_user(s, org_id, root.id, roles=["reviewer"])
    async with database.transaction() as s:
        await auth.delete_user(s, org_id, root.id)
    with pytest.raises(NotFound):
        async with database.transaction() as s:
            await auth.delete_user(s, org_id, root.id)


async def test_login_and_session_lifecycle(database, org_id):
    async with database.transaction() as s:
        u = await auth.create_user(s, org_id, "ada@example.com", "Ada", "password123", ["reviewer"])
    async with database.transaction() as s:
        token, got = await auth.login(s, "ADA@example.com ", "password123")
    assert got.id == u.id and token
    assert len(token) == 43
    async with database.session() as s:
        stored = (await s.execute(sa.text("SELECT token_hash FROM sessions"))).scalar_one()
    assert stored != token and stored == auth.hash_token(token)
    async with database.session() as s:
        p = await auth.principal_from_session(s, token)
    assert (p.kind, p.id, p.org_id, p.email, p.roles) == ("user", u.id, org_id, "ada@example.com", ("reviewer",))
    async with database.transaction() as s:
        await auth.logout(s, token)
    with pytest.raises(Unauthenticated):
        async with database.session() as s:
            await auth.principal_from_session(s, token)
    async with database.transaction() as s:
        await auth.logout(s, token)


async def test_login_failures(database, org_id):
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "ada@example.com", "Ada", "password123", None)
    for email, pw in (("ada@example.com", "wrong-password"), ("nobody@example.com", "password123")):
        with pytest.raises(InvalidCredentials):
            async with database.transaction() as s:
                await auth.login(s, email, pw)


async def test_expired_session_is_rejected(database, org_id):
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "ada@example.com", "Ada", "password123", None)
        token, _ = await auth.login(s, "ada@example.com", "password123")
        await s.execute(sa.text("UPDATE sessions SET expires_at = now() - interval '1 hour'"))
    async with database.session() as s:
        for tok in (token, ""):
            with pytest.raises(Unauthenticated):
                await auth.principal_from_session(s, tok)


async def test_password_change_revokes_sessions(database, org_id):
    async with database.transaction() as s:
        u = await auth.create_user(s, org_id, "ada@example.com", "Ada", "password123", None)
        token, _ = await auth.login(s, "ada@example.com", "password123")
    async with database.transaction() as s:
        await auth.update_user(s, org_id, u.id, password="brand-new-pass")
    with pytest.raises(Unauthenticated):
        async with database.session() as s:
            await auth.principal_from_session(s, token)
    async with database.transaction() as s:
        await auth.login(s, "ada@example.com", "brand-new-pass")


async def test_go_bcrypt_hashes_still_verify(database, org_id):
    """A $2a$ hash written by the Go server (golang.org/x/crypto/bcrypt) must keep working."""
    go_hash = "$2a$04$IsrlfTR3csPDHtQSvuQ3x.4G5/8y2sUIgHC6e/zzdUiSUQyTEqfp6"  # "password123", cost 4, from golang.org/x/crypto/bcrypt
    async with database.transaction() as s:
        u = await auth.create_user(s, org_id, "go@example.com", "Go", "password123", None)
        await s.execute(sa.text("UPDATE users SET password_hash = :h WHERE id = :id"), {"h": go_hash, "id": u.id})
    async with database.transaction() as s:
        _, got = await auth.login(s, "go@example.com", "password123")
    assert got.id == u.id


async def test_api_key_lifecycle(database, org_id):
    async with database.transaction() as s:
        plaintext, k = await auth.create_api_key(s, org_id, " CI deploy ", ["admin"])
    assert re.fullmatch(r"ofk_[0-9A-Za-z]{40}", plaintext)
    assert (k.prefix, k.name, k.org_id, k.revoked_at) == (plaintext[:8], "CI deploy", org_id, None)
    async with database.transaction() as s:
        p = await auth.principal_from_api_key(s, plaintext)
    assert (p.kind, p.id, p.name, p.is_admin(), p.org_id) == ("api_key", k.id, "CI deploy", True, org_id)
    async with database.session() as s:
        keys = await auth.list_api_keys(s, org_id)
    assert len(keys) == 1 and keys[0].last_used_at is not None
    async with database.transaction() as s:
        await auth.revoke_api_key(s, org_id, k.id)
    with pytest.raises(Unauthenticated):
        async with database.session() as s:
            await auth.principal_from_api_key(s, plaintext)
    async with database.transaction() as s:
        await auth.revoke_api_key(s, org_id, k.id)
    with pytest.raises(NotFound):
        async with database.transaction() as s:
            await auth.revoke_api_key(s, org_id, uuid.uuid4())


async def test_api_key_rejects_garbage(database, org_id):
    async with database.session() as s:
        for k in ("", "garbage", "ofk_doesnotexist"):
            with pytest.raises(Unauthenticated):
                await auth.principal_from_api_key(s, k)


async def test_create_api_key_validation(database, org_id):
    with pytest.raises(ValidationError) as ei:
        async with database.transaction() as s:
            await auth.create_api_key(s, org_id, "  ", ["NOPE"])
    assert paths(ei) == ["name", "roles[0]"]


def test_has_any_role():
    reviewer = Principal(org_id=uuid.uuid4(), kind="user", id=uuid.uuid4(), roles=("reviewer",))
    admin = Principal(org_id=uuid.uuid4(), kind="user", id=uuid.uuid4(), roles=("admin",))
    none = Principal(org_id=uuid.uuid4(), kind="user", id=uuid.uuid4())
    assert none.has_any_role()
    assert reviewer.has_any_role("hiring-manager", "reviewer")
    assert not reviewer.has_any_role("hiring-manager")
    assert admin.has_any_role("hiring-manager")
    assert not none.has_any_role("reviewer")
    assert admin.is_admin() and not reviewer.is_admin()

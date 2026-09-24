"""Port of ``internal/seed/seed_test.go``."""

from openforms.server.models import auth
from openforms.server.models import definitions as defs
from openforms.server.models.seed import DEMO_PASSWORD, DEMO_USERS, seed_demo


async def test_seeds_bundle_and_users(database, org_id):
    res = await seed_demo(database, org_id)
    assert len(res.apply.items) == 4
    assert all(it.created and it.version == 1 for it in res.apply.items)
    assert len(res.users_created) == 3
    async with database.session() as s:
        for du in DEMO_USERS:
            u = await auth.get_user_by_email(s, org_id, du.email)
            assert tuple(u.roles) == du.roles
        rec = await defs.get_form(s, org_id, "job-application")
    assert rec.current.source == "seed"
    async with database.transaction() as s:
        await auth.login(s, "reviewer@demo.local", DEMO_PASSWORD)


async def test_is_idempotent(database, org_id):
    await seed_demo(database, org_id)
    res = await seed_demo(database, org_id)
    assert not any(it.changed or it.created for it in res.apply.items)
    assert res.users_created == []
    async with database.session() as s:
        assert len(await defs.form_versions(s, org_id, "job-application")) == 1
        assert len(await auth.list_users(s, org_id)) == 3


async def test_keeps_existing_user(database, org_id):
    async with database.transaction() as s:
        await auth.create_user(s, org_id, "reviewer@demo.local", "Existing", "my-own-password", ["reviewer"])
    res = await seed_demo(database, org_id)
    assert "reviewer@demo.local" not in res.users_created
    async with database.transaction() as s:
        await auth.login(s, "reviewer@demo.local", "my-own-password")

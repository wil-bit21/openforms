import uuid

import pytest
import sqlalchemy as sa

from openforms.server.database.alembic_commands import migrate
from openforms.server.database.engine import asyncpg_url
from openforms.server.database.orm_models import Base, Org
from tests.conftest import drop_schema, migrated_database, new_schema_url


async def count_orgs(db) -> int:
    async with db.session() as s:
        return (await s.execute(sa.text("SELECT count(*) FROM orgs"))).scalar_one()


async def test_migrate_creates_all_tables(database):
    async with database.session() as s:
        names = set(
            (
                await s.execute(
                    sa.text("SELECT table_name FROM information_schema.tables WHERE table_schema = current_schema()")
                )
            ).scalars()
        )
    assert {
        "orgs",
        "users",
        "sessions",
        "api_keys",
        "workflows",
        "workflow_versions",
        "forms",
        "form_versions",
        "submissions",
        "submission_events",
        "jobs",
        "password_resets",
        "alembic_version",
    } <= names


async def test_migrate_is_idempotent(database):
    await migrate(database.engine)


async def test_transaction_commits(database):
    async with database.transaction() as s:
        s.add(Org(id=uuid.uuid4(), name="x"))
    assert await count_orgs(database) == 1


async def test_transaction_rolls_back_on_error(database):
    with pytest.raises(RuntimeError, match="boom"):
        async with database.transaction() as s:
            s.add(Org(id=uuid.uuid4(), name="x"))
            await s.flush()
            raise RuntimeError("boom")
    assert await count_orgs(database) == 0


async def test_schemas_are_isolated(database):
    url, schema = await new_schema_url()
    other = await migrated_database(url)
    try:
        async with database.transaction() as s:
            s.add(Org(id=uuid.uuid4(), name="a"))
        assert await count_orgs(other) == 0
    finally:
        await other.dispose()
        await drop_schema(schema)


async def test_orm_models_match_the_migrated_schema(database):
    async with database.session() as s:
        rows = (
            await s.execute(
                sa.text(
                    "SELECT table_name, column_name, is_nullable FROM information_schema.columns "
                    "WHERE table_schema = current_schema() AND table_name <> 'alembic_version'"
                )
            )
        ).all()
    db_cols = {(t, c): n == "YES" for t, c, n in rows}
    orm_cols = {(t.name, c.name): c.nullable for t in Base.metadata.sorted_tables for c in t.columns}
    assert set(db_cols) == set(orm_cols)
    for key, nullable in db_cols.items():
        assert orm_cols[key] == nullable, key


async def test_adopts_a_database_created_by_the_go_server(db_url):
    db = await migrated_database(db_url)
    try:
        async with db.transaction() as s:
            await s.execute(sa.text("DROP TABLE alembic_version"))
            await s.execute(sa.text("DROP TABLE password_resets"))  # added after the Go server
        await migrate(db.engine)  # tables exist: 0001 is recorded without re-creating them, then 0002 runs
        async with db.session() as s:
            assert (
                await s.execute(sa.text("SELECT version_num FROM alembic_version"))
            ).scalar_one() == "0002_password_resets"
            assert (await s.execute(sa.text("SELECT to_regclass('password_resets') IS NOT NULL"))).scalar()
    finally:
        await db.dispose()


def test_asyncpg_url_translation():
    url, args = asyncpg_url("postgres://u:p@h:5432/db?sslmode=disable&search_path=t_1")
    assert url == "postgresql+asyncpg://u:p@h:5432/db"
    assert args == {"ssl": False, "server_settings": {"search_path": "t_1"}}
    with pytest.raises(ValueError):
        asyncpg_url("mysql://x")

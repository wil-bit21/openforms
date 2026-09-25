"""Orgs, users, sessions and API keys (port of the Go ``internal/auth``)."""

from __future__ import annotations

import datetime as dt
import hashlib
import re
import secrets
import uuid
from collections.abc import Sequence

import anyio
import bcrypt
import sqlalchemy as sa
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession

from ...definition import Problem, ValidationError
from ...definition.common import is_bare_email
from ..database import orm_models as orm
from ..exceptions import EmailTaken, InvalidCredentials, InvalidResetToken, NotFound, Unauthenticated
from ..schemas.core import PRINCIPAL_API_KEY, PRINCIPAL_USER, ROLE_ADMIN, ApiKey, Principal, User
from ._db import is_unique_violation

BCRYPT_ROUNDS = 12  # tests lower this
SESSION_TTL = dt.timedelta(days=30)
RESET_TTL = dt.timedelta(hours=1)
API_KEY_PREFIX = "ofk_"
DEFAULT_ORG_LOCK = 7243001
_BASE62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
_ROLE_RE = re.compile(r"[a-z0-9][a-z0-9_-]{0,31}")


def hash_token(tok: str) -> str:
    return hashlib.sha256(tok.encode()).hexdigest()


def random_token(n_bytes: int) -> str:
    return secrets.token_urlsafe(n_bytes)


def random_base62(n: int) -> str:
    return "".join(secrets.choice(_BASE62) for _ in range(n))


async def _hash_password(pw: str) -> str:
    return (await anyio.to_thread.run_sync(lambda: bcrypt.hashpw(pw.encode(), bcrypt.gensalt(BCRYPT_ROUNDS)))).decode()


async def _check_password(pw: str, hashed: str) -> bool:
    def check() -> bool:
        try:
            return bcrypt.checkpw(pw.encode(), hashed.encode())
        except ValueError:
            return False

    return await anyio.to_thread.run_sync(check)


_dummy_hash: str | None = None


async def _compare_dummy(pw: str) -> None:
    """Spend as long as a real check so unknown emails aren't distinguishable by timing."""
    global _dummy_hash
    if _dummy_hash is None:
        _dummy_hash = await _hash_password("openforms-timing-equaliser")
    await _check_password(pw, _dummy_hash)


# --- validation ---------------------------------------------------------------


def _normalize_email(email: str) -> tuple[str, Problem | None]:
    e = email.strip().lower()
    return e, None if is_bare_email(e) else Problem("email", "must be a valid email address")


def _name_problem(name: str) -> Problem | None:
    return Problem("name", "is required") if not name else None


def _password_problem(pw: str) -> Problem | None:
    if len(pw.encode()) < 8:
        return Problem("password", "must be at least 8 characters")
    if len(pw.encode()) > 72:
        return Problem("password", "must be at most 72 bytes")
    return None


def normalize_roles(roles: Sequence[str]) -> tuple[list[str], list[Problem]]:
    """Trim, de-duplicate (first occurrence wins) and validate role names."""
    out: list[str] = []
    probs: list[Problem] = []
    for i, r in enumerate(roles):
        r = r.strip() if isinstance(r, str) else ""
        if not _ROLE_RE.fullmatch(r):
            probs.append(Problem(f"roles[{i}]", "must be lowercase letters, digits, '-' or '_' (max 32)"))
            continue
        if r not in out:
            out.append(r)
    return out, probs


def _collect(probs: list[Problem], *more: Problem | None) -> list[Problem]:
    return probs + [p for p in more if p is not None]


# --- orgs ---------------------------------------------------------------------


async def ensure_default_org(session: AsyncSession) -> uuid.UUID:
    """Return the single org, creating "Default" on first call. Runs in the caller's
    transaction; an advisory lock serialises concurrent first calls."""
    await session.execute(sa.text("SELECT pg_advisory_xact_lock(:k)"), {"k": DEFAULT_ORG_LOCK})
    row = (await session.execute(sa.select(orm.Org.id).order_by(orm.Org.created_at, orm.Org.id).limit(1))).first()
    if row is not None:
        return row[0]
    org_id = uuid.uuid4()
    session.add(orm.Org(id=org_id, name="Default"))
    await session.flush()
    return org_id


# --- users --------------------------------------------------------------------


def _user(u: orm.User) -> User:
    return User(
        id=u.id, org_id=u.org_id, email=u.email, name=u.name, roles=list(u.roles or []), created_at=u.created_at
    )


async def create_user(
    session: AsyncSession, org_id: uuid.UUID, email: str, name: str, password: str, roles: Sequence[str] | None
) -> User:
    e, email_prob = _normalize_email(email)
    name = name.strip()
    probs = _collect([], email_prob, _name_problem(name), _password_problem(password))
    rs, role_probs = normalize_roles(roles or [])
    probs += role_probs
    if probs:
        raise ValidationError(probs)
    row = orm.User(
        id=uuid.uuid4(), org_id=org_id, email=e, name=name, password_hash=await _hash_password(password), roles=rs
    )
    try:
        async with session.begin_nested():
            session.add(row)
    except IntegrityError as err:
        if is_unique_violation(err):
            raise EmailTaken() from None
        raise
    await session.refresh(row)
    return _user(row)


async def list_users(session: AsyncSession, org_id: uuid.UUID) -> list[User]:
    rows = await session.scalars(
        sa.select(orm.User).where(orm.User.org_id == org_id).order_by(orm.User.created_at, orm.User.email)
    )
    return [_user(u) for u in rows]


async def get_user(session: AsyncSession, org_id: uuid.UUID, user_id: uuid.UUID) -> User:
    u = await session.scalar(sa.select(orm.User).where(orm.User.id == user_id, orm.User.org_id == org_id))
    if u is None:
        raise NotFound()
    return _user(u)


async def get_user_by_email(session: AsyncSession, org_id: uuid.UUID, email: str) -> User:
    u = await session.scalar(
        sa.select(orm.User).where(orm.User.org_id == org_id, sa.func.lower(orm.User.email) == email.strip().lower())
    )
    if u is None:
        raise NotFound()
    return _user(u)


async def users_with_role(session: AsyncSession, org_id: uuid.UUID, role: str) -> list[User]:
    rows = await session.scalars(
        sa.select(orm.User)
        .where(orm.User.org_id == org_id, sa.literal(role) == sa.any_(orm.User.roles))
        .order_by(orm.User.created_at, orm.User.id)
    )
    return [_user(u) for u in rows]


async def _lock_admins(session: AsyncSession, org_id: uuid.UUID) -> list[uuid.UUID]:
    """Lock every admin row, ordered by id so concurrent callers never deadlock."""
    rows = await session.execute(
        sa.select(orm.User.id)
        .where(orm.User.org_id == org_id, sa.literal(ROLE_ADMIN) == sa.any_(orm.User.roles))
        .order_by(orm.User.id)
        .with_for_update()
    )
    return [r[0] for r in rows]


def _last_admin(admins: list[uuid.UUID], user_id: uuid.UUID, path: str) -> None:
    if all(a == user_id for a in admins):
        raise ValidationError([Problem(path, "at least one admin must remain")])


async def update_user(
    session: AsyncSession,
    org_id: uuid.UUID,
    user_id: uuid.UUID,
    *,
    name: str | None = None,
    password: str | None = None,
    roles: Sequence[str] | None = None,
) -> User:
    """Update the given properties (None = unchanged). Changing the password revokes sessions."""
    probs: list[Problem] = []
    if name is not None:
        name = name.strip()
        probs = _collect(probs, _name_problem(name))
    if password is not None:
        probs = _collect(probs, _password_problem(password))
    new_roles: list[str] | None = None
    if roles is not None:
        new_roles, rp = normalize_roles(roles)
        probs += rp
    if probs:
        raise ValidationError(probs)
    pw_hash = await _hash_password(password) if password is not None else None

    admins = await _lock_admins(session, org_id) if new_roles is not None else []
    cur = await session.scalar(
        sa.select(orm.User).where(orm.User.id == user_id, orm.User.org_id == org_id).with_for_update()
    )
    if cur is None:
        raise NotFound()
    if new_roles is not None and ROLE_ADMIN in (cur.roles or []) and ROLE_ADMIN not in new_roles:
        _last_admin(admins, user_id, "roles")
    if new_roles is not None:
        cur.roles = new_roles
    if name is not None:
        cur.name = name
    if pw_hash is not None:
        cur.password_hash = pw_hash
        await session.execute(sa.delete(orm.Session).where(orm.Session.user_id == user_id))
    await session.flush()
    return _user(cur)


async def delete_user(session: AsyncSession, org_id: uuid.UUID, user_id: uuid.UUID) -> None:
    admins = await _lock_admins(session, org_id)
    cur = await session.scalar(
        sa.select(orm.User).where(orm.User.id == user_id, orm.User.org_id == org_id).with_for_update()
    )
    if cur is None:
        raise NotFound()
    if ROLE_ADMIN in (cur.roles or []):
        _last_admin(admins, user_id, "id")
    await session.execute(sa.delete(orm.User).where(orm.User.id == user_id, orm.User.org_id == org_id))


# --- sessions -----------------------------------------------------------------


async def login(session: AsyncSession, email: str, password: str) -> tuple[str, User]:
    e = email.strip().lower()
    u = await session.scalar(
        sa.select(orm.User).where(sa.func.lower(orm.User.email) == e).order_by(orm.User.created_at).limit(1)
    )
    if u is None:
        await _compare_dummy(password)
        raise InvalidCredentials()
    if not await _check_password(password, u.password_hash):
        raise InvalidCredentials()
    token = random_token(32)
    await session.execute(sa.delete(orm.Session).where(orm.Session.expires_at < sa.func.now()))
    session.add(
        orm.Session(token_hash=hash_token(token), user_id=u.id, expires_at=dt.datetime.now(dt.UTC) + SESSION_TTL)
    )
    await session.flush()
    return token, _user(u)


async def request_password_reset(session: AsyncSession, email: str) -> tuple[str, User] | None:
    """Issue a single-use reset token for the user with ``email`` (None when there is no
    such user). Earlier unused tokens of that user stop working."""
    e = email.strip().lower()
    u = await session.scalar(
        sa.select(orm.User).where(sa.func.lower(orm.User.email) == e).order_by(orm.User.created_at).limit(1)
    )
    await session.execute(sa.delete(orm.PasswordReset).where(orm.PasswordReset.expires_at < sa.func.now()))
    if u is None:
        return None
    await session.execute(sa.delete(orm.PasswordReset).where(orm.PasswordReset.user_id == u.id))
    token = random_token(32)
    session.add(
        orm.PasswordReset(token_hash=hash_token(token), user_id=u.id, expires_at=dt.datetime.now(dt.UTC) + RESET_TTL)
    )
    await session.flush()
    return token, _user(u)


async def reset_password(session: AsyncSession, token: str, password: str) -> User:
    """Set a new password with a reset token. The token is consumed and every session of
    the user is signed out."""
    if p := _password_problem(password):
        raise ValidationError([p])
    row = await session.scalar(
        sa.select(orm.PasswordReset)
        .where(orm.PasswordReset.token_hash == hash_token(token), orm.PasswordReset.expires_at > sa.func.now())
        .with_for_update()
    )
    if not token or row is None:
        raise InvalidResetToken()
    u = await session.scalar(sa.select(orm.User).where(orm.User.id == row.user_id).with_for_update())
    if u is None:
        raise InvalidResetToken()
    u.password_hash = await _hash_password(password)
    await session.execute(sa.delete(orm.PasswordReset).where(orm.PasswordReset.user_id == u.id))
    await session.execute(sa.delete(orm.Session).where(orm.Session.user_id == u.id))
    await session.flush()
    return _user(u)


async def logout(session: AsyncSession, token: str) -> None:
    await session.execute(sa.delete(orm.Session).where(orm.Session.token_hash == hash_token(token)))


async def principal_from_session(session: AsyncSession, token: str) -> Principal:
    if not token:
        raise Unauthenticated()
    row = (
        await session.execute(
            sa.select(orm.User.id, orm.User.org_id, orm.User.email, orm.User.name, orm.User.roles)
            .join(orm.Session, orm.Session.user_id == orm.User.id)
            .where(orm.Session.token_hash == hash_token(token), orm.Session.expires_at > sa.func.now())
        )
    ).first()
    if row is None:
        raise Unauthenticated()
    return Principal(
        org_id=row.org_id, kind=PRINCIPAL_USER, id=row.id, name=row.name, email=row.email, roles=tuple(row.roles or ())
    )


# --- API keys -----------------------------------------------------------------


def _api_key(k: orm.ApiKey) -> ApiKey:
    return ApiKey(
        id=k.id,
        org_id=k.org_id,
        name=k.name,
        prefix=k.prefix,
        roles=list(k.roles or []),
        created_at=k.created_at,
        last_used_at=k.last_used_at,
        revoked_at=k.revoked_at,
    )


async def create_api_key(
    session: AsyncSession, org_id: uuid.UUID, name: str, roles: Sequence[str] | None
) -> tuple[str, ApiKey]:
    name = name.strip()
    probs: list[Problem] = []
    if not name or len(name) > 100:
        probs.append(Problem("name", "is required (max 100 characters)"))
    rs, rp = normalize_roles(roles or [])
    probs += rp
    if probs:
        raise ValidationError(probs)
    plaintext = API_KEY_PREFIX + random_base62(40)
    row = orm.ApiKey(
        id=uuid.uuid4(), org_id=org_id, name=name, prefix=plaintext[:8], key_hash=hash_token(plaintext), roles=rs
    )
    session.add(row)
    await session.flush()
    await session.refresh(row)
    return plaintext, _api_key(row)


async def list_api_keys(session: AsyncSession, org_id: uuid.UUID) -> list[ApiKey]:
    rows = await session.scalars(
        sa.select(orm.ApiKey).where(orm.ApiKey.org_id == org_id).order_by(orm.ApiKey.created_at.desc(), orm.ApiKey.id)
    )
    return [_api_key(k) for k in rows]


async def revoke_api_key(session: AsyncSession, org_id: uuid.UUID, key_id: uuid.UUID) -> None:
    """Idempotent; raises NotFound only for unknown ids."""
    res = await session.execute(
        sa.update(orm.ApiKey)
        .where(orm.ApiKey.id == key_id, orm.ApiKey.org_id == org_id)
        .values(revoked_at=sa.func.coalesce(orm.ApiKey.revoked_at, sa.func.now()))
    )
    if res.rowcount == 0:  # type: ignore[attr-defined]
        raise NotFound()


async def principal_from_api_key(session: AsyncSession, plaintext: str) -> Principal:
    if not plaintext.startswith(API_KEY_PREFIX):
        raise Unauthenticated()
    row = (
        await session.execute(
            sa.select(orm.ApiKey.id, orm.ApiKey.org_id, orm.ApiKey.name, orm.ApiKey.roles).where(
                orm.ApiKey.key_hash == hash_token(plaintext), orm.ApiKey.revoked_at.is_(None)
            )
        )
    ).first()
    if row is None:
        raise Unauthenticated()
    # Throttled so busy keys do not write on every request.
    await session.execute(
        sa.update(orm.ApiKey)
        .where(
            orm.ApiKey.id == row.id,
            sa.or_(
                orm.ApiKey.last_used_at.is_(None),
                orm.ApiKey.last_used_at < sa.func.now() - sa.text("interval '1 minute'"),
            ),
        )
        .values(last_used_at=sa.func.now())
    )
    return Principal(org_id=row.org_id, kind=PRINCIPAL_API_KEY, id=row.id, name=row.name, roles=tuple(row.roles or ()))

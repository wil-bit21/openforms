"""Login throttling and the password reset flow (``/auth/password-reset``)."""

import re

import sqlalchemy as sa

from openforms.server.api import auth as auth_api
from openforms.server.models import auth
from tests.conftest import error_code


class FakeMailer:
    def __init__(self):
        self.sent = []

    async def send(self, to, subject, body):
        self.sent.append((to, subject, body))


def reset_token(body: str) -> str:
    m = re.search(r"/admin/reset-password\?token=([A-Za-z0-9_-]+)", body)
    assert m, body
    return m.group(1)


async def login(env, email, password):
    return await env.do("POST", "/api/v1/auth/login", body={"email": email, "password": password})


async def test_reset_flow_sets_the_password_and_signs_out_everywhere(env):
    mailer = env.ctx.services["mailer"] = FakeMailer()
    await env.user("ada@example.com", "reviewer")
    old = await login(env, "ada@example.com", "password123")
    assert old.status_code == 200
    old_cookie = old.cookies["of_session"]

    r = await env.do("POST", "/api/v1/auth/password-reset", body={"email": "  ADA@example.com "})
    assert r.status_code == 202
    [(to, subject, body)] = mailer.sent
    assert to == "ada@example.com" and subject == "Reset your openforms password"
    assert "http://test.local/admin/reset-password?token=" in body
    token = reset_token(body)

    r = await env.do("POST", "/api/v1/auth/password-reset/confirm", body={"token": token, "password": "short"})
    assert r.status_code == 422 and r.json()["error"]["details"][0]["path"] == "password"

    r = await env.do("POST", "/api/v1/auth/password-reset/confirm", body={"token": token, "password": "new-password-1"})
    assert r.status_code == 204

    env.client.cookies.clear()
    r = await env.do("GET", "/api/v1/auth/me", headers={"Cookie": f"of_session={old_cookie}"})
    assert r.status_code == 401  # the old session was revoked
    assert (await login(env, "ada@example.com", "password123")).status_code == 401
    assert (await login(env, "ada@example.com", "new-password-1")).status_code == 200

    # single use
    r = await env.do("POST", "/api/v1/auth/password-reset/confirm", body={"token": token, "password": "another-pass"})
    assert (r.status_code, error_code(r)) == (400, "invalid_token")


async def test_reset_request_does_not_reveal_unknown_emails(env):
    mailer = env.ctx.services["mailer"] = FakeMailer()
    r = await env.do("POST", "/api/v1/auth/password-reset", body={"email": "nobody@example.com"})
    assert r.status_code == 202 and r.content == b""
    assert mailer.sent == []


async def test_new_reset_request_invalidates_the_previous_link(env):
    mailer = env.ctx.services["mailer"] = FakeMailer()
    await env.user("ada@example.com")
    for _ in range(2):
        await env.do("POST", "/api/v1/auth/password-reset", body={"email": "ada@example.com"})
    first, second = (reset_token(b) for _, _, b in mailer.sent)
    r = await env.do("POST", "/api/v1/auth/password-reset/confirm", body={"token": first, "password": "new-password-1"})
    assert error_code(r) == "invalid_token"
    r = await env.do(
        "POST", "/api/v1/auth/password-reset/confirm", body={"token": second, "password": "new-password-1"}
    )
    assert r.status_code == 204


async def test_expired_and_bogus_tokens_are_rejected(env):
    env.ctx.services["mailer"] = FakeMailer()
    u = await env.user("ada@example.com")
    async with env.db.transaction() as s:
        token, _ = await auth.request_password_reset(s, "ada@example.com")
        await s.execute(sa.text("UPDATE password_resets SET expires_at = now() - interval '1 second'"))
    for t in (token, "", "not-a-token"):
        r = await env.do("POST", "/api/v1/auth/password-reset/confirm", body={"token": t, "password": "new-password-1"})
        assert (r.status_code, error_code(r)) == (400, "invalid_token"), t
    async with env.db.session() as s:
        assert (await auth.get_user(s, env.org_id, u.id)).email == "ada@example.com"


async def test_login_is_rate_limited_per_email(env, monkeypatch):
    monkeypatch.setattr(auth_api, "LOGIN_PER_EMAIL", (3, 900.0))
    await env.user("ada@example.com")
    codes = [(await login(env, "Ada@example.com", "wrong-password")).status_code for _ in range(4)]
    assert codes == [401, 401, 401, 429]
    r = await login(env, "ada@example.com", "password123")
    assert (r.status_code, error_code(r)) == (429, "rate_limited")
    assert r.headers["retry-after"] == "300"
    assert (await login(env, "other@example.com", "x")).status_code == 401  # other accounts unaffected


async def test_login_is_rate_limited_per_ip(env, monkeypatch):
    monkeypatch.setattr(auth_api, "LOGIN_PER_IP", (2, 300.0))
    codes = [(await login(env, f"u{i}@example.com", "wrong-password")).status_code for i in range(3)]
    assert codes == [401, 401, 429]


async def test_reset_requests_are_rate_limited(env, monkeypatch):
    env.ctx.services["mailer"] = FakeMailer()
    monkeypatch.setattr(auth_api, "RESET_PER_EMAIL", (1, 3600.0))
    codes = [
        (await env.do("POST", "/api/v1/auth/password-reset", body={"email": "ada@example.com"})).status_code
        for _ in range(2)
    ]
    assert codes == [202, 429]

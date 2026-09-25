from http.cookies import SimpleCookie

from tests.conftest import error_code


def session_cookie(resp):
    for header in resp.headers.get_list("set-cookie"):
        c = SimpleCookie()
        c.load(header)
        if "of_session" in c:
            return c["of_session"]
    raise AssertionError(f"no of_session cookie in {resp.headers.get_list('set-cookie')}")


async def login(env, email, password):
    return await env.do("POST", "/api/v1/auth/login", body={"email": email, "password": password})


async def test_login_me_logout(env):
    await env.user("ada@example.com", "reviewer")
    r = await login(env, "ADA@example.com", "password123")
    assert r.status_code == 200, r.text
    assert r.json()["user"]["email"] == "ada@example.com" and r.json()["user"]["createdAt"]
    assert "password" not in r.text
    c = session_cookie(r)
    assert c["httponly"] and c["path"] == "/" and c["samesite"].lower() == "lax" and int(c["max-age"]) == 30 * 24 * 3600
    env.client.cookies.set("of_session", c.value)
    me = await env.do("GET", "/api/v1/auth/me")
    assert me.status_code == 200
    p = me.json()["principal"]
    assert (p["kind"], p["email"], p["roles"]) == ("user", "ada@example.com", ["reviewer"])
    out = await env.do("POST", "/api/v1/auth/logout")
    assert out.status_code == 204
    cleared = session_cookie(out)
    assert cleared.value in ("", '""') and int(cleared["max-age"] or 0) <= 0
    env.client.cookies.set("of_session", c.value)
    again = await env.do("GET", "/api/v1/auth/me")
    assert again.status_code == 401 and error_code(again) == "unauthenticated"


async def test_login_failures(env):
    await env.user("ada@example.com")
    r = await login(env, "ada@example.com", "wrong-password")
    assert r.status_code == 401 and error_code(r) == "invalid_credentials"
    assert not r.headers.get_list("set-cookie")
    r = await env.do("POST", "/api/v1/auth/login", body='{"email":')
    assert r.status_code == 400 and error_code(r) == "bad_request"


async def test_me_with_api_key(env):
    key = await env.api_key("admin")
    r = await env.do("GET", "/api/v1/auth/me", key)
    p = r.json()["principal"]
    assert (p["kind"], p["roles"], p["email"]) == ("api_key", ["admin"], "")
    assert (await env.do("GET", "/api/v1/auth/me")).status_code == 401


async def test_users_crud(env):
    admin = await env.api_key("admin")
    r = await env.do(
        "POST",
        "/api/v1/users",
        admin,
        {"email": "Ada@Example.com", "name": "Ada", "password": "password123", "roles": ["reviewer"]},
    )
    assert r.status_code == 201, r.text
    created = r.json()["user"]
    assert created["email"] == "ada@example.com" and created["roles"] == ["reviewer"]
    r = await env.do(
        "POST", "/api/v1/users", admin, {"email": "ada@example.com", "name": "Dup", "password": "password123"}
    )
    assert r.status_code == 409 and error_code(r) == "email_taken"
    r = await env.do("POST", "/api/v1/users", admin, {"email": "nope", "name": "", "password": "x"})
    assert r.status_code == 422 and error_code(r) == "validation_failed"
    r = await env.do("POST", "/api/v1/users", admin, {"email": 5})
    assert r.status_code == 400 and error_code(r) == "bad_request"
    assert len((await env.do("GET", "/api/v1/users", admin)).json()["items"]) == 1
    r = await env.do(
        "PATCH",
        f"/api/v1/users/{created['id']}",
        admin,
        {"name": "Ada Lovelace", "roles": ["reviewer", "hiring-manager"]},
    )
    assert r.status_code == 200 and r.json()["user"]["name"] == "Ada Lovelace" and len(r.json()["user"]["roles"]) == 2
    assert (await env.do("DELETE", f"/api/v1/users/{created['id']}", admin)).status_code == 204
    assert (await env.do("DELETE", f"/api/v1/users/{created['id']}", admin)).status_code == 404
    assert (await env.do("PATCH", "/api/v1/users/not-a-uuid", admin, {})).status_code == 404


async def test_users_require_admin(env):
    r = await env.do("GET", "/api/v1/users", await env.api_key("reviewer"))
    assert r.status_code == 403 and error_code(r) == "forbidden"
    assert (await env.do("GET", "/api/v1/users")).status_code == 401


async def test_last_admin_is_protected_over_http(env):
    key = await env.api_key("admin")
    root = await env.user("root@example.com", "admin")
    r = await env.do("PATCH", f"/api/v1/users/{root.id}", key, {"roles": []})
    assert r.status_code == 422 and error_code(r) == "validation_failed"
    assert (await env.do("DELETE", f"/api/v1/users/{root.id}", key)).status_code == 422
    await env.user("second@example.com", "admin")
    assert (await env.do("PATCH", f"/api/v1/users/{root.id}", key, {"roles": []})).status_code == 200


async def test_api_keys_lifecycle(env):
    admin = await env.api_key("admin")
    r = await env.do("POST", "/api/v1/api-keys", admin, {"name": "ci", "roles": ["reviewer"]})
    assert r.status_code == 201
    created = r.json()
    assert created["key"].startswith("ofk_") and created["apiKey"]["prefix"] == created["key"][:8]
    assert created["apiKey"]["roles"] == ["reviewer"] and created["apiKey"]["revokedAt"] is None
    assert (await env.do("GET", "/api/v1/auth/me", created["key"])).status_code == 200
    items = (await env.do("GET", "/api/v1/api-keys", admin)).json()["items"]
    assert len(items) == 2 and all("key" not in it for it in items)
    assert (await env.do("DELETE", f"/api/v1/api-keys/{created['apiKey']['id']}", admin)).status_code == 204
    assert (await env.do("GET", "/api/v1/auth/me", created["key"])).status_code == 401
    assert (await env.do("POST", "/api/v1/api-keys", admin, {"name": ""})).status_code == 422
    assert (await env.do("GET", "/api/v1/api-keys", created["key"])).status_code == 401


async def test_api_keys_require_admin(env):
    r = await env.do("POST", "/api/v1/api-keys", await env.api_key("reviewer"), {"name": "x"})
    assert r.status_code == 403

import pytest


@pytest.fixture
def built(env):
    root = env.ctx.ui_dir
    files = {
        "admin/index.html": b"<html>admin</html>",
        "admin/assets/app-123.js": b"console.log('admin')",
        "admin/favicon.svg": b"<svg/>",
        "hosted/index.html": b"<html>hosted</html>",
        "demo/index.html": b"<html>demo</html>",
        "embed/embed.js": b"/*embed*/",
        ".gitkeep": b"",
    }
    for name, data in files.items():
        p = root / name
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(data)
    return env


@pytest.mark.parametrize(
    "path,want",
    [
        ("/admin", "admin"),
        ("/admin/", "admin"),
        ("/admin/submissions/abc", "admin"),
        ("/f/contact", "hosted"),
        ("/s/1234", "hosted"),
        ("/demo", "demo"),
        ("/demo/anything", "demo"),
    ],
)
async def test_spa_fallbacks(built, path, want):
    r = await built.do("GET", path)
    assert r.status_code == 200 and want in r.text
    assert r.headers["content-type"].startswith("text/html")
    assert r.headers["cache-control"] == "no-cache"


async def test_static_assets(built):
    r = await built.do("GET", "/_app/admin/assets/app-123.js")
    assert r.status_code == 200 and r.text == "console.log('admin')"
    assert "javascript" in r.headers["content-type"]
    assert "immutable" in r.headers["cache-control"]
    r = await built.do("GET", "/_app/admin/favicon.svg")
    assert r.status_code == 200 and r.headers["cache-control"] == "no-cache"


async def test_embed_script(built):
    r = await built.do("GET", "/embed.js")
    assert r.status_code == 200 and r.text == "/*embed*/"


@pytest.mark.parametrize(
    "path",
    [
        "/nope",
        "/_app/admin/missing.js",
        "/_app/admin/",
        "/_app/",
        "/_app/../../etc/passwd",
        "/_app/%2e%2e/%2e%2e/etc/passwd",
    ],
)
async def test_not_found_and_traversal(built, path):
    assert (await built.do("GET", path)).status_code != 200


async def test_root_redirects_to_admin(built):
    r = await built.do("GET", "/", follow_redirects=False)
    assert r.status_code == 302 and r.headers["location"] == "/admin"


async def test_method_not_allowed(built):
    assert (await built.do("POST", "/admin")).status_code == 405


async def test_not_built_returns_503(env):
    r = await env.do("GET", "/admin")
    assert r.status_code == 503 and "make web" in r.text

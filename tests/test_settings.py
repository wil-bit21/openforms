import pytest

from openforms.settings import ConfigError, load_settings

ALL_KEYS = [
    "OPENFORMS_HTTP_ADDR",
    "OPENFORMS_DATABASE_URL",
    "OPENFORMS_BASE_URL",
    "OPENFORMS_SMTP_HOST",
    "OPENFORMS_SMTP_PORT",
    "OPENFORMS_SMTP_USERNAME",
    "OPENFORMS_SMTP_PASSWORD",
    "OPENFORMS_SMTP_FROM",
    "OPENFORMS_WEBHOOK_SECRET",
    "OPENFORMS_WORKER_CONCURRENCY",
    "OPENFORMS_DEMO",
    "OPENFORMS_COOKIE_SECURE",
    "OPENFORMS_SEED_DEMO",
]


@pytest.fixture(autouse=True)
def clear_env(monkeypatch):
    for k in ALL_KEYS:
        monkeypatch.setenv(k, "")


def test_defaults():
    c = load_settings()
    assert c.http_addr == ":8080"
    assert c.base_url == "http://localhost:8080"
    assert c.smtp_port == 1025
    assert c.smtp_from == "openforms@localhost"
    assert c.worker_concurrency == 4
    assert c.demo_mode is False and c.cookie_secure is False
    assert c.database_url == "" and c.smtp_host == "" and c.webhook_secret == ""


def test_overrides(monkeypatch):
    env = {
        "OPENFORMS_HTTP_ADDR": "127.0.0.1:9000",
        "OPENFORMS_DATABASE_URL": "postgres://x",
        "OPENFORMS_BASE_URL": "https://forms.example.com/",
        "OPENFORMS_SMTP_HOST": "smtp.example.com",
        "OPENFORMS_SMTP_PORT": "2525",
        "OPENFORMS_SMTP_USERNAME": "u",
        "OPENFORMS_SMTP_PASSWORD": "p",
        "OPENFORMS_SMTP_FROM": "noreply@example.com",
        "OPENFORMS_WEBHOOK_SECRET": "s3cret",
        "OPENFORMS_WORKER_CONCURRENCY": "8",
        "OPENFORMS_DEMO": "1",
    }
    for k, v in env.items():
        monkeypatch.setenv(k, v)
    c = load_settings()
    assert c.http_addr == "127.0.0.1:9000"
    assert c.listen_host_port == ("127.0.0.1", 9000)
    assert c.database_url == "postgres://x"
    assert c.base_url == "https://forms.example.com"
    assert (c.smtp_host, c.smtp_port, c.smtp_username, c.smtp_password) == ("smtp.example.com", 2525, "u", "p")
    assert c.smtp_from == "noreply@example.com" and c.webhook_secret == "s3cret"
    assert c.worker_concurrency == 8 and c.demo_mode is True and c.cookie_secure is True


def test_cookie_secure_explicit_false_on_https(monkeypatch):
    monkeypatch.setenv("OPENFORMS_BASE_URL", "https://x.example")
    monkeypatch.setenv("OPENFORMS_COOKIE_SECURE", "false")
    assert load_settings().cookie_secure is False


@pytest.mark.parametrize(
    "key,value",
    [
        ("OPENFORMS_SMTP_PORT", "abc"),
        ("OPENFORMS_SMTP_PORT", "0"),
        ("OPENFORMS_WORKER_CONCURRENCY", "-1"),
        ("OPENFORMS_DEMO", "yes"),
        ("OPENFORMS_COOKIE_SECURE", "maybe"),
    ],
)
def test_invalid_values(monkeypatch, key, value):
    monkeypatch.setenv(key, value)
    with pytest.raises(ConfigError) as ei:
        load_settings()
    assert key in str(ei.value)


def test_require_database():
    with pytest.raises(ConfigError, match="OPENFORMS_DATABASE_URL is required"):
        load_settings().require_database()
    load_settings(database_url="postgres://x").require_database()

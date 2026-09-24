"""Runtime configuration, read from ``OPENFORMS_*`` environment variables.

Empty variables count as unset, as they did in the Go server.
"""

from __future__ import annotations

from typing import Any

from pydantic import Field, ValidationError, field_validator, model_validator
from pydantic_settings import BaseSettings, SettingsConfigDict


class ConfigError(ValueError):
    """Raised when the environment holds an invalid setting."""


class Settings(BaseSettings):
    model_config = SettingsConfigDict(env_prefix="OPENFORMS_", env_ignore_empty=True, frozen=True, extra="ignore")

    http_addr: str = ":8080"
    database_url: str = ""
    base_url: str = "http://localhost:8080"
    smtp_host: str = ""
    smtp_port: int = Field(default=1025, ge=1)
    smtp_username: str = ""
    smtp_password: str = ""
    smtp_from: str = "openforms@localhost"
    webhook_secret: str = ""
    worker_concurrency: int = Field(default=4, ge=1)
    demo: bool = False
    cookie_secure: bool | None = None
    seed_demo: bool = False

    @field_validator("demo", "cookie_secure", "seed_demo", mode="before")
    @classmethod
    def _strict_bool(cls, v: Any) -> Any:
        if isinstance(v, str):
            low = v.lower()
            if low in ("true", "1"):
                return True
            if low in ("false", "0"):
                return False
            raise ValueError(f"must be true/false/1/0, got {v!r}")
        return v

    @field_validator("base_url")
    @classmethod
    def _trim_base_url(cls, v: str) -> str:
        return v.rstrip("/")

    @model_validator(mode="after")
    def _default_cookie_secure(self) -> Settings:
        if self.cookie_secure is None:
            object.__setattr__(self, "cookie_secure", self.base_url.startswith("https://"))
        return self

    @property
    def demo_mode(self) -> bool:
        return self.demo

    def require_database(self) -> None:
        if not self.database_url:
            raise ConfigError("OPENFORMS_DATABASE_URL is required")

    @property
    def listen_host_port(self) -> tuple[str, int]:
        """Split ``http_addr`` (Go style ``host:port`` or ``:port``) for uvicorn."""
        host, _, port = self.http_addr.rpartition(":")
        return (host or "0.0.0.0", int(port or 8080))


def load_settings(**overrides: Any) -> Settings:
    """Load settings from the environment, turning validation errors into ConfigError."""
    try:
        return Settings(**overrides)
    except ValidationError as e:
        parts = []
        for err in e.errors():
            name = "OPENFORMS_" + "_".join(str(p) for p in err["loc"]).upper()
            parts.append(f"{name}: {err['msg']}")
        raise ConfigError("; ".join(parts)) from None

"""A small async client for the openforms HTTP API (definitions export, validate, apply)."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from http import HTTPStatus
from typing import Any

import httpx

from .definition import Form, Problem, Workflow


@dataclass
class Bundle:
    """The wire shape of ``/api/v1/definitions`` (export, validate, apply)."""

    forms: list[Form] = field(default_factory=list)
    workflows: list[Workflow] = field(default_factory=list)

    def to_dict(self) -> dict[str, Any]:
        return {"forms": [f.to_dict() for f in self.forms], "workflows": [w.to_dict() for w in self.workflows]}

    @classmethod
    def from_dict(cls, d: dict[str, Any]) -> Bundle:
        return cls(
            forms=[Form.from_dict(f) for f in d.get("forms") or []],
            workflows=[Workflow.from_dict(w) for w in d.get("workflows") or []],
        )


@dataclass
class ApplyItem:
    kind: str
    slug: str
    version: int = 0
    changed: bool = False
    created: bool = False


@dataclass
class ApplyResult:
    items: list[ApplyItem] = field(default_factory=list)


@dataclass
class ValidateResult:
    valid: bool


class APIError(Exception):
    """An API error decoded from the error envelope (spec §7.1)."""

    def __init__(self, status: int, code: str, message: str, details: list[Problem] | None = None):
        self.status, self.code, self.message, self.details = status, code, message, list(details or [])
        super().__init__(f"{code}: {message} (HTTP {status})")


class ClientError(Exception):
    """Transport failures (unreachable server, unreadable response)."""


class Client:
    def __init__(self, base_url: str, api_key: str, *, transport: httpx.AsyncBaseTransport | None = None):
        self.base_url = base_url.rstrip("/")
        self.api_key = api_key
        self._transport = transport

    async def validate(self, b: Bundle) -> ValidateResult:
        out = await self._do("POST", "/api/v1/definitions/validate", b.to_dict())
        return ValidateResult(valid=bool(out.get("valid")))

    async def apply(self, b: Bundle, dry_run: bool = False) -> ApplyResult:
        q = "dryRun=true&source=cli" if dry_run else "source=cli"
        out = await self._do("POST", "/api/v1/definitions/apply?" + q, b.to_dict())
        return ApplyResult(
            items=[
                ApplyItem(
                    kind=i.get("kind", ""),
                    slug=i.get("slug", ""),
                    version=int(i.get("version") or 0),
                    changed=bool(i.get("changed")),
                    created=bool(i.get("created")),
                )
                for i in out.get("items") or []
            ]
        )

    async def export(self) -> Bundle:
        return Bundle.from_dict(await self._do("GET", "/api/v1/definitions"))

    async def _do(self, method: str, path: str, body: Any = None) -> dict[str, Any]:
        headers = {"Accept": "application/json"}
        content = None
        if body is not None:
            headers["Content-Type"] = "application/json"
            content = json.dumps(body, ensure_ascii=False).encode()
        if self.api_key:
            headers["Authorization"] = "Bearer " + self.api_key
        try:
            async with httpx.AsyncClient(transport=self._transport, timeout=30) as http:
                resp = await http.request(method, self.base_url + path, headers=headers, content=content)
                data = resp.content[: 32 << 20]
        except httpx.TransportError as e:
            raise ClientError(f"cannot reach openforms server at {self.base_url}: {e}") from None
        if resp.status_code >= 300:
            raise decode_error(resp.status_code, data)
        if not data:
            return {}
        try:
            out = json.loads(data)
        except ValueError as e:
            raise ClientError(f"decode response from {self.base_url}{path}: {e}") from None
        return out if isinstance(out, dict) else {}


def decode_error(status: int, data: bytes) -> APIError:
    try:
        env = json.loads(data)
        err = env.get("error") if isinstance(env, dict) else None
        if isinstance(err, dict) and err.get("code"):
            details = [Problem(d.get("path", ""), d.get("message", "")) for d in err.get("details") or []]
            return APIError(status, err["code"], err.get("message", ""), details)
    except ValueError:
        pass
    msg = data.decode("utf-8", "replace").strip()
    if not msg:
        try:
            msg = HTTPStatus(status).phrase
        except ValueError:
            msg = ""
    return APIError(status, "http_error", msg[:200])

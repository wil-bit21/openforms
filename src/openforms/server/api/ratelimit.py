"""Fixed-window per-key rate limiter (in-memory, per process — enough for v1)."""

from __future__ import annotations

import threading
import time
from collections.abc import Callable

from starlette.requests import Request


class RateLimiter:
    def __init__(
        self, limit: int, window: float, now: Callable[[], float] = time.monotonic, max_keys: int = 10000
    ) -> None:
        self.limit, self.window, self.now, self.max_keys = limit, window, now, max_keys
        self._hits: dict[str, list[float]] = {}  # key -> [window start, count]
        self._lock = threading.Lock()

    def allow(self, key: str) -> bool:
        """Record a hit for ``key`` and report whether it is within the limit."""
        with self._lock:
            t = self.now()
            w = self._hits.get(key)
            if w is None or t - w[0] >= self.window:
                if w is None and len(self._hits) >= self.max_keys:
                    self._sweep(t)
                self._hits[key] = [t, 1]
                return True
            if w[1] >= self.limit:
                return False
            w[1] += 1
            return True

    def _sweep(self, t: float) -> None:
        for k in [k for k, w in self._hits.items() if t - w[0] >= self.window]:
            del self._hits[k]


def client_ip(request: Request) -> str:
    """The client address; ``openforms serve`` resolves it from X-Forwarded-For when the
    peer is in OPENFORMS_FORWARDED_ALLOW_IPS."""
    return request.client.host if request.client else ""


def shared_limiter(services: dict, name: str, limit: int, window: float) -> RateLimiter:
    """The app-wide limiter called ``name`` (created on first use)."""
    lim = services.get(name)
    if lim is None:
        lim = services[name] = RateLimiter(limit, window)
    return lim


def too_many_requests(message: str, retry_after: float):
    from .errors import envelope

    resp = envelope(429, "rate_limited", message)
    resp.headers["Retry-After"] = str(int(retry_after))
    return resp

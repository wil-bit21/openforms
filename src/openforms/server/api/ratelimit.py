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
    """The peer address (a proxy may rewrite it with --proxy-headers)."""
    return request.client.host if request.client else ""

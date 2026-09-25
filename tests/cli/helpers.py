"""Port of ``internal/cli/helpers_test.go``."""

from __future__ import annotations

import io
from contextlib import asynccontextmanager
from pathlib import Path

from openforms.cli.deps import Deps
from openforms.client import ApplyResult, Bundle, ValidateResult

REMOTE_ENV = {"OPENFORMS_URL": "http://fake", "OPENFORMS_API_KEY": "ofk_fake"}


class FakeRemote:
    def __init__(self, apply_res=None, apply_err=None, export=None, export_err=None):
        self.applied: list[Bundle] = []
        self.dry_runs: list[bool] = []
        self.apply_res = apply_res or ApplyResult()
        self.apply_err, self.export_data, self.export_err = apply_err, export or Bundle(), export_err

    async def validate(self, b):
        return ValidateResult(valid=True)

    async def apply(self, b, dry_run=False):
        self.applied.append(b)
        self.dry_runs.append(dry_run)
        if self.apply_err:
            raise self.apply_err
        return self.apply_res

    async def export(self):
        if self.export_err:
            raise self.export_err
        return self.export_data


class TestIO:
    __test__ = False

    def __init__(self):
        self.out, self.err = io.StringIO(), io.StringIO()

    def reset(self):
        self.out.seek(0)
        self.out.truncate()
        self.err.seek(0)
        self.err.truncate()


@asynccontextmanager
async def _no_auth():
    raise AssertionError("open_auth not configured in this test")
    yield  # pragma: no cover


def new_test_deps(work_dir: Path, env=None, remote=None) -> tuple[Deps, TestIO]:
    tio = TestIO()
    env = env or {}
    return Deps(
        out=tio.out,
        err=tio.err,
        work_dir=Path(work_dir),
        getenv=env.get,
        new_client=lambda server, key: remote,  # type: ignore[arg-type,return-value]
        open_auth=_no_auth,
    ), tio


def put(d: Path, rel: str, content: str | bytes) -> None:
    p = Path(d) / rel
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_bytes(content.encode() if isinstance(content, str) else content)


def read_rel(d: Path, rel: str) -> str:
    return (Path(d) / rel).read_bytes().decode()

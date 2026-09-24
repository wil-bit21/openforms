"""Port of ``internal/cli/e2e_test.go``: init → push → diff → pull round trip against the
real API (in-process ASGI app)."""

import re

import httpx
import pytest

from openforms.cli import commands
from openforms.cli.deps import CLIError, DriftError
from openforms.client import Client
from tests.cli.helpers import new_test_deps, put, read_rel


@pytest.fixture
async def run(env):
    key = await env.api_key("admin")

    async def run(work_dir, cmd, *args, api_key=key, **kw):
        d, tio = new_test_deps(work_dir, {"OPENFORMS_URL": "http://test.local", "OPENFORMS_API_KEY": api_key})
        d.new_client = lambda server, k: Client(server, k, transport=httpx.ASGITransport(app=env.app))
        await getattr(commands, "run_" + cmd)(d, *args, **kw)
        return tio.out.getvalue()

    return run


def must_match(out, pattern):
    assert re.search(pattern, out, re.M), out


async def test_push_pull_round_trip(run, tmp_path):
    proj_a, proj_b = tmp_path / "a", tmp_path / "b"
    await run(proj_a, "init")

    with pytest.raises(CLIError, match="check --api-key or OPENFORMS_API_KEY"):
        await run(proj_a, "push", api_key="ofk_wrong")

    out = await run(proj_a, "push")
    must_match(out, r"^workflow\s+contact-triage\s+1\s+created$")
    must_match(out, r"^form\s+contact\s+1\s+created$")
    must_match(await run(proj_a, "push"), r"^form\s+contact\s+1\s+unchanged$")
    assert await run(proj_a, "diff") == "No differences.\n"
    assert await run(proj_a, "pull", check=True) == "Up to date.\n"

    await run(proj_b, "pull")
    for rel in ("openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"):
        assert read_rel(proj_a, rel) == read_rel(proj_b, rel), rel

    form_a = read_rel(proj_a, "openforms/forms/contact.yaml")
    put(proj_a, "openforms/forms/contact.yaml", form_a.replace("title: Contact us", "title: Talk to us", 1))
    assert await run(proj_a, "diff") == "~ form contact\n"
    must_match(await run(proj_a, "push", dry_run=True), r"^form\s+contact\s+2\s+updated$")
    must_match(await run(proj_a, "push"), r"^form\s+contact\s+2\s+updated$")

    with pytest.raises(DriftError):
        await run(proj_b, "pull", check=True)
    await run(proj_b, "pull")
    assert "title: Talk to us" in read_rel(proj_b, "openforms/forms/contact.yaml")

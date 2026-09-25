"""Port of ``internal/cli/cli_test.go``: the installed ``openforms`` entry point."""

import os
import subprocess
import sys

from openforms.cli import commands
from tests.cli.helpers import new_test_deps


def openforms(*args, **env):
    e = {k: v for k, v in os.environ.items() if not k.startswith("OPENFORMS_")}
    e.update(env)
    return subprocess.run([sys.executable, "-m", "openforms", *args], capture_output=True, text=True, env=e, timeout=60)


async def test_migrate_command(db_url, tmp_path, monkeypatch):
    monkeypatch.setenv("OPENFORMS_DATABASE_URL", db_url)
    d, tio = new_test_deps(tmp_path)
    await commands.run_migrate(d)
    assert tio.out.getvalue() == "migrations applied\n"


def test_serve_migrate_and_seed_require_database_url():
    for args in (["serve"], ["migrate"], ["seed", "--demo"], ["admin", "create-api-key", "--name", "x"]):
        r = openforms(*args)
        assert r.returncode == 1, (args, r.stdout, r.stderr)
        assert r.stderr.startswith("Error: OPENFORMS_DATABASE_URL is required"), (args, r.stderr)


def test_seed_requires_demo_flag():
    r = openforms("seed")
    assert r.returncode == 1 and r.stderr == "Error: nothing to seed: pass --demo\n"


def test_root_help_lists_commands():
    r = openforms("--help")
    assert r.returncode == 0
    for want in ("serve", "migrate", "seed", "admin", "init", "validate", "push", "pull", "diff"):
        assert want in r.stdout


def test_validate_problems_exit_1(tmp_path):
    (tmp_path / "openforms/forms").mkdir(parents=True)
    (tmp_path / "openforms/forms/x.yaml").write_text("slug: y\n")
    r = subprocess.run([sys.executable, "-m", "openforms", "validate"], capture_output=True, text=True, cwd=tmp_path)
    assert r.returncode == 1 and r.stderr.endswith("problem(s) found\nError: definitions are invalid\n"), r.stderr

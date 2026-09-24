"""Command implementations. Each takes :class:`Deps` so tests can run it in-process;
``openforms.cli`` wires them to the command line."""

from __future__ import annotations

import os
from dataclasses import dataclass
from importlib import resources
from pathlib import Path
from typing import TextIO

from ..client import APIError, ApplyItem, ApplyResult, Bundle, ClientError
from ..definition import ValidationError, canonical, parse_form, parse_workflow
from .deps import CLIError, Deps, DriftError, ProblemsError, explain_remote_error, load_project_config
from .local import definition_files, load_local, print_problems
from .yamlout import canonical_yaml

INIT_CONFIG = "server: http://localhost:8080\ndir: openforms\n"


def split_roles(s: str) -> list[str]:
    return [r.strip() for r in s.split(",") if r.strip()]


def to_yaml(v) -> str:
    try:
        return canonical_yaml(v.to_dict() if hasattr(v, "to_dict") else v)
    except ValueError as e:
        raise CLIError(str(e)) from None


# --- admin -------------------------------------------------------------------------------


async def admin_create_user(d: Deps, email: str, name: str, password: str, roles: str) -> None:
    from ..server.exceptions import EmailTaken
    from ..server.models import auth

    if not email or not password:
        raise CLIError("--email and --password are required")
    name = name or email.split("@", 1)[0]
    async with d.open_auth() as (db, org_id), db.transaction() as s:
        try:
            u = await auth.create_user(s, org_id, email, name, password, split_roles(roles))
        except EmailTaken:
            raise CLIError(f"a user with email {email.lower()} already exists") from None
    print(f"Created user {u.email} ({u.id}) with roles [{', '.join(u.roles)}]", file=d.out)


async def admin_create_api_key(d: Deps, name: str, roles: str) -> None:
    from ..server.models import auth

    if not name:
        raise CLIError("--name is required")
    async with d.open_auth() as (db, org_id), db.transaction() as s:
        plaintext, k = await auth.create_api_key(s, org_id, name, split_roles(roles))
    print(f'Created API key "{k.name}" (prefix {k.prefix}) with roles [{", ".join(k.roles)}]', file=d.out)
    print(plaintext, file=d.out)
    print("Store this key now; it will not be shown again.", file=d.out)


# --- init / validate -----------------------------------------------------------------------


def _template(name: str) -> bytes:
    return resources.files("openforms.cli").joinpath("templates", name).read_bytes()


async def run_init(d: Deps, target_dir: str = "") -> None:
    target = d.abs(target_dir) if target_dir else d.work_dir
    try:
        form = parse_form(_template("contact.yaml"))
    except ValidationError as e:
        raise CLIError(f"built-in form template is invalid: {e}") from None
    try:
        wf = parse_workflow(_template("contact-triage.yaml"))
    except ValidationError as e:
        raise CLIError(f"built-in workflow template is invalid: {e}") from None
    files = [
        ("openforms.yaml", INIT_CONFIG),
        ("openforms/forms/contact.yaml", to_yaml(form)),
        ("openforms/workflows/contact-triage.yaml", to_yaml(wf)),
    ]
    for rel, _ in files:
        if os.path.lexists(target / rel):
            raise CLIError(f"{rel} already exists; refusing to overwrite")
    for rel, data in files:
        p = target / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(data.encode())
        print(f"created {rel}", file=d.out)
    print("\nNext steps:", file=d.out)
    print("  openforms validate", file=d.out)
    print("  export OPENFORMS_API_KEY=<key from `openforms admin create-api-key --name cli --roles admin`>", file=d.out)
    print("  openforms push", file=d.out)


async def run_validate(d: Deps, dir_flag: str = "") -> None:
    pc = load_project_config(d.work_dir)
    lb, problems = load_local(d.resolve_dir(dir_flag, pc))
    if problems:
        print_problems(d.err, problems)
        raise ProblemsError()
    print(f"OK: {len(lb.bundle.forms)} form(s), {len(lb.bundle.workflows)} workflow(s) valid", file=d.out)


# --- push / pull / diff ----------------------------------------------------------------


def _remote_failure(d: Deps, e: Exception) -> Exception:
    out = explain_remote_error(d.err, e)
    if isinstance(out, (APIError, ClientError)):
        return CLIError(str(out))
    return out


def apply_status(it: ApplyItem) -> str:
    if it.created:
        return "created"
    if it.changed:
        return "updated"
    return "unchanged"


def print_apply_table(w: TextIO, res: ApplyResult) -> None:
    """Go ``text/tabwriter`` with padding 2: every column but the last is padded."""
    rows = [["KIND", "SLUG", "VERSION", "STATUS"]]
    rows += [[it.kind, it.slug, str(it.version), apply_status(it)] for it in res.items]
    widths = [max(len(r[i]) for r in rows) + 2 for i in range(3)]
    for r in rows:
        print("".join(c.ljust(widths[i]) for i, c in enumerate(r[:3])) + r[3], file=w)


async def run_push(d: Deps, dir_flag: str = "", server: str = "", api_key: str = "", dry_run: bool = False) -> None:
    pc = load_project_config(d.work_dir)
    lb, problems = load_local(d.resolve_dir(dir_flag, pc))
    if problems:
        print_problems(d.err, problems)
        raise ProblemsError()
    srv, key = d.resolve_remote(server, api_key, pc)
    try:
        res = await d.new_client(srv, key).apply(lb.bundle, dry_run)
    except (APIError, ClientError) as e:
        raise _remote_failure(d, e) from None
    if dry_run:
        print("Dry run: nothing was applied.", file=d.out)
    print_apply_table(d.out, res)


@dataclass
class PullChange:
    rel: str  # e.g. "forms/contact.yaml"
    path: Path
    data: bytes
    status: str  # created | updated | unchanged


def plan_pull(d: Path, b: Bundle) -> tuple[list[PullChange], list[str]]:
    """What pull would write, without touching the filesystem."""
    changes: list[PullChange] = []
    remote: set[str] = set()

    def add(kind: str, slug: str, v) -> None:
        remote.add(f"{kind}/{slug}")
        for ext in (".yml", ".json"):
            if (d / kind / (slug + ext)).exists():
                raise CLIError(
                    f"{kind}/{slug}{ext} exists; pull only manages .yaml files, rename it to {kind}/{slug}.yaml"
                )
        data = to_yaml(v).encode()
        c = PullChange(f"{kind}/{slug}.yaml", d / kind / (slug + ".yaml"), data, "created")
        try:
            existing = c.path.read_bytes()
        except FileNotFoundError:
            pass
        else:
            c.status = "unchanged" if existing == data else "updated"
        changes.append(c)

    for f in b.forms:
        add("forms", f.slug, f)
    for w in b.workflows:
        add("workflows", w.slug, w)
    local_only = []
    for kind in ("forms", "workflows"):
        for p in definition_files(d / kind):
            if f"{kind}/{p.stem}" not in remote:
                local_only.append(f"{kind}/{p.name}")
    return changes, local_only


async def run_pull(d: Deps, dir_flag: str = "", server: str = "", api_key: str = "", check: bool = False) -> None:
    pc = load_project_config(d.work_dir)
    target = d.resolve_dir(dir_flag, pc)
    srv, key = d.resolve_remote(server, api_key, pc)
    try:
        b = await d.new_client(srv, key).export()
    except (APIError, ClientError) as e:
        raise _remote_failure(d, e) from None
    changes, local_only = plan_pull(target, b)
    if check:
        drift = 0
        for c in changes:
            if c.status != "unchanged":
                drift += 1
                print(f"would {c.status.removesuffix('d')} {c.rel}", file=d.out)
        if drift:
            raise DriftError()
        print("Up to date.", file=d.out)
        return
    counts = {"created": 0, "updated": 0, "unchanged": 0}
    for c in changes:
        counts[c.status] += 1
        if c.status == "unchanged":
            continue
        c.path.parent.mkdir(parents=True, exist_ok=True)
        c.path.write_bytes(c.data)
        print(f"{c.status} {c.rel}", file=d.out)
    for rel in local_only:
        print(f"note: {rel} exists locally but not on the server (left untouched)", file=d.out)
    print(
        f"Pulled {len(b.forms)} form(s), {len(b.workflows)} workflow(s): "
        f"{counts['created']} created, {counts['updated']} updated, {counts['unchanged']} unchanged.",
        file=d.out,
    )


def _hashes(b: Bundle) -> dict[tuple[str, str], str]:
    m = {("form", f.slug): canonical(f)[1] for f in b.forms}
    m.update({("workflow", w.slug): canonical(w)[1] for w in b.workflows})
    return m


def diff_bundles(local: Bundle, remote: Bundle) -> list[str]:
    """Compare by canonical hash; unchanged definitions are omitted."""
    lh, rh = _hashes(local), _hashes(remote)
    out = []
    for kind, slug in sorted(lh.keys() | rh.keys()):
        if (kind, slug) not in rh:
            out.append(f"+ {kind} {slug} (local only)")
        elif (kind, slug) not in lh:
            out.append(f"- {kind} {slug} (remote only)")
        elif lh[kind, slug] != rh[kind, slug]:
            out.append(f"~ {kind} {slug}")
    return out


async def run_diff(d: Deps, dir_flag: str = "", server: str = "", api_key: str = "") -> None:
    pc = load_project_config(d.work_dir)
    lb, problems = load_local(d.resolve_dir(dir_flag, pc))
    if problems:
        print_problems(d.err, problems)
        raise ProblemsError()
    srv, key = d.resolve_remote(server, api_key, pc)
    try:
        remote = await d.new_client(srv, key).export()
    except (APIError, ClientError) as e:
        raise _remote_failure(d, e) from None
    lines = diff_bundles(lb.bundle, remote)
    if not lines:
        print("No differences.", file=d.out)
        return
    for line in lines:
        print(line, file=d.out)


# --- server-side commands --------------------------------------------------------------


async def run_migrate(d: Deps) -> None:
    from ..server.database.alembic_commands import migrate
    from ..server.database.engine import Database
    from ..settings import load_settings

    settings = load_settings()
    settings.require_database()
    db = Database(settings.database_url)
    try:
        await migrate(db.engine)
    finally:
        await db.dispose()
    print("migrations applied", file=d.out)


async def run_seed_demo(d: Deps) -> None:
    from ..server.models.seed import DEMO_PASSWORD, seed_demo

    async with d.open_auth() as (db, org_id):
        res = await seed_demo(db, org_id)
    for it in res.apply.items:
        status = "created" if it.created else "updated" if it.changed else "unchanged"
        print(f"{it.kind:<9} {it.slug:<20} v{it.version:<4} {status}", file=d.out)
    if not res.users_created:
        print("demo users already exist", file=d.out)
    for email in res.users_created:
        print(f"user      {email:<20} password {DEMO_PASSWORD}", file=d.out)

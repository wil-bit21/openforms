# openforms Plan 10 — Python Backend (Prefect-style) Implementation Plan

> **For agentic workers:** Execute task by task with superpowers:executing-plans. Each task ports a slice of the Go backend and **ports its Go tests to pytest first** (red), then the implementation (green), then commits. The Go code stays in the tree as the reference until Task 14 deletes it.

**Goal:** Replace the Go server and CLI with one Python package, `openforms`, laid out the way PrefectHQ/prefect lays out its server. Nothing a user can see changes: the HTTP API, database schema, CLI commands and flags, env vars, web apps, conformance fixtures and Playwright E2E suite all stay as they are. The Python backend must pass all of them.

**Architecture (mirrors `prefect`):** A `src/` layout package built with hatchling and managed with `uv`. `openforms.server` holds:

| Directory | Contents |
|---|---|
| `schemas/` | Pydantic v2 API models |
| `database/` | SQLAlchemy 2 async ORM and Alembic migrations |
| `models/` | async functions that take an `AsyncSession` first and hold all SQL |
| `api/` | FastAPI routers |
| `services/` | background loops (the job worker) |

The pure definition language lives in `openforms.definition`, the HTTP client in `openforms.client`, and the `openforms` command in `openforms.cli` (cyclopts). New endpoints follow Prefect's layering: schema, then model, then route.

**Tech stack:**

- **Python and packaging:** Python ≥ 3.11 (the Docker image uses 3.12), uv, hatchling.
- **Server:** FastAPI, uvicorn, Pydantic v2, pydantic-settings.
- **Database:** SQLAlchemy 2 (asyncio) with asyncpg, and Alembic.
- **Definitions and auth:** jsonschema (Draft 2020-12), PyYAML (a YAML 1.1 loader, matching Go's `sigs.k8s.io/yaml`), bcrypt.
- **I/O:** httpx for the webhook action and the CLI client, stdlib `smtplib` run via `anyio.to_thread` for email.
- **CLI:** cyclopts, as Prefect uses.
- **Dev tools:** pytest, pytest-asyncio, ruff, pyright.

**Database:** PostgreSQL 16 only. The user chose this over Prefect's SQLite + Postgres, so the job queue keeps `FOR UPDATE SKIP LOCKED`.

## Binding contracts (unchanged from the spec)

`docs/superpowers/specs/2026-09-23-openforms-design.md`:

- **§4 data model:** schema-identical tables, columns and constraints.
- **§5 definitions:** the JSON Schemas in `schemas/`, the semantic rules, the conformance fixtures and versioning.
- **§7 HTTP API:** paths, status codes, error envelope and camelCase JSON shapes.
- **§8 CLI:** command names, flags, output formats and exit codes.
- **§9 frontend:** unchanged.
- **§10 demo and packaging:** demo users, `OPENFORMS_*` env vars and the compose profile.
- **§11 edge cases.**
- **§12 tests.**

Two spec sections are replaced by this plan:

- **§2 backend rows and §3 backend layout:** replaced by the stack above and the file map below.
- **§6 Go package contracts:** replaced by the Python module map below. Behaviour stays identical, and the Go tests define it.

**Compatibility is checked the same way throughout:** the Plan 02 conformance fixtures, the Go tests ported case by case, the unchanged web suites, and the unchanged Playwright E2E suite (12 tests) run against the Docker image of the Python server.

### Decisions this plan makes

1. **The command surface stays as it is:** `openforms serve`, `migrate`, `admin create-user|create-api-key`, `init`, `validate`, `push`, `pull`, `diff`, `seed --demo`. This keeps docs, E2E and compose working. Unlike Prefect's `prefect server start`, there is no `server` sub-app.
2. **Migrations:** Alembic lives at `src/openforms/server/database/_migrations/` (Prefect's location). Revision `0001_initial` runs the SQL of Go migrations `00001`–`00003` verbatim through `op.execute`, so the schema cannot drift. Later changes are ordinary Alembic revisions. `openforms migrate` runs `alembic upgrade head` programmatically.
3. **Passwords:** bcrypt at cost 12, which a test may lower. Hashes are interchangeable with Go's `$2a$` hashes, so existing databases keep working.
4. **Tokens:** session and API-key formats, prefixes (`ofk_`), lengths and sha256 storage match Go byte for byte.
5. **Web builds:** they move from `internal/webui/dist` to `src/openforms/server/ui/` (as in Prefect's `src/prefect/server/ui`). They ship inside the wheel as package data. `web/scripts/copy-dist.mjs` gets the new target. The SPA routing rules are those of `internal/webui/webui.go`: `/admin/*`, `/f/*`, `/s/*`, `/demo*`, `/_app/*`, `/embed.js`, with 503 when a bundle is missing.
6. **Packaged data:** the JSON Schemas (`schemas/*.json`) and the example bundle (`examples/openforms/`) stay at the repo root, because the web workspace reads them there. Hatch `force-include` copies them into the wheel as `openforms/_schemas` and `openforms/_examples`, and code reads them with `importlib.resources`. In an editable install, a fallback resolves them from the repo root.
7. **Concurrency:** one asyncpg engine per process, with `AsyncSession` per request via a FastAPI dependency. Transactions use `async with session.begin()`. Row locks use `with_for_update()` and `with_for_update(skip_locked=True)`.
8. **Job worker:** an asyncio task started in the FastAPI lifespan, the equivalent of Prefect's `services`. It polls the queue with the same claim, backoff, lock-reclaim and permanent-failure semantics as `internal/jobs`, and shuts down gracefully after the HTTP server drains.
9. **Test isolation:** each test gets a fresh Postgres schema. `tests/conftest.py` ports `dbtest`: create schema `t_<hex>`, run Alembic against it through `search_path`, drop it afterwards. The default DSN and the `OPENFORMS_TEST_DATABASE_URL` override are unchanged.

## File structure

```
pyproject.toml, uv.lock                  Task 1
src/openforms/
  __init__.py, py.typed, _paths.py       Task 1   (resources: schemas, examples, ui)
  settings.py                            Task 1   (port of internal/config)
  definition/                            Task 2   (port of internal/definition)
    types.py  problems.py  canonical.py  values.py  validate_form.py
    validate_workflow.py  parse.py  bundle.py  submission.py  workflow_fields.py
  server/
    database/
      engine.py  orm_models.py  alembic.ini  alembic_commands.py
      _migrations/env.py, script.py.mako, versions/0001_initial.py     Task 3
    schemas/   core.py (camel base)  auth.py  definitions.py  submissions.py  workflow.py  jobs.py
    models/    auth.py  definitions.py  submissions.py  jobs.py  seed.py
    workflow/  engine.py                                  (port of internal/workflow)
    actions/   render.py  vars.py  mailer.py  payload.py  runner.py  webhook.py  email.py  assign.py
    services/  worker.py
    api/       server.py (create_app, lifespan)  errors.py  dependencies.py  ratelimit.py
               root.py (healthz)  auth.py  users.py  api_keys.py  definitions.py  public.py
               submissions.py  workflow.py  jobs.py  public_config.py  ui.py
    ui/        (build output, git-ignored)
  client/      client.py                                  (port of internal/client)
  cli/         __init__.py (app)  _deps.py  _yamlout.py  _local.py  server.py (serve, migrate)
               admin.py  init.py  validate.py  push.py  diff.py  pull.py  seed.py  templates/
tests/
  conftest.py  (db schema isolation, app/client fixtures, fixture bundle)
  definition/ server/ cli/ client/     (ported from the Go *_test.go files)
```

## Tasks

Each task lists the Go sources it ports and the Go tests whose cases must all reappear as pytest tests.

### Task 1: Scaffold
- `pyproject.toml`:
  - project `openforms`, `requires-python >=3.11`
  - runtime dependencies as in the tech stack
  - `[dependency-groups] dev`: pytest, pytest-asyncio, pytest-timeout, ruff, pyright, respx
  - `[project.scripts] openforms = "openforms.cli:app"`
  - hatch wheel config: `packages = ["src/openforms"]`, `force-include` for `schemas/` and `examples/openforms/`, and `artifacts = ["src/openforms/server/ui"]`
  - ruff settings (line length 120; rule sets E, F, I, UP, B, ASYNC)
  - pytest settings (`asyncio_mode = "auto"`, `testpaths = ["tests"]`)
  - pyright settings (`basic`, `src`)
- `settings.py`: `Settings(BaseSettings)` with the same `OPENFORMS_*` names, defaults and validation errors as `internal/config` (bool parsing, `require_database()`). Port `config_test.go`.
- `_paths.py`: `schemas_dir()`, `examples_dir()`, `ui_dir()`.
- **Gate:** `uv sync`, `uv run ruff check`, `uv run pytest`.

### Task 2: Definition language
- **Ports:** `internal/definition/*.go`.
- **Types:** Pydantic models with camelCase aliases, `exclude_none` / `omitempty`-equivalent dumping, and `$schema` dropped on load.
- **Parsing:** `ParseForm`/`ParseWorkflow` keep the same behaviour and problem paths:
  - reject YAML duplicate keys at the root path;
  - YAML 1.1 implicit booleans (`value: yes`) fail schema validation at the exact path;
  - JSON Schema errors map to `fields[0].options[0].value`-style paths.
- **Validation:** keep Go's semantics exactly:
  - `canonical()` (sorted keys, compact, no HTML escaping, sha256 hex);
  - `validate_form` / `validate_workflow` / `validate_bundle`;
  - `check_value` (code-point lengths, anchored patterns, trimming, `net/mail`-equivalent bare-address check, http(s) URLs, real dates);
  - `visible` / `validate_submission`, one evaluator;
  - `validate_workflow_fields`, where a patch with `None` means unset.
- **Tests:** port every case from `internal/definition/*_test.go` and `schemas/schemas_test.go`, and run both conformance fixture files unchanged.

### Task 3: Database layer
- `engine.py`: a `create_async_engine` factory and `session_scope()`.
- `orm_models.py`: SQLAlchemy 2 declarative models for every table in §4, including JSONB columns, and `uuid` primary keys generated in Python.
- **Alembic:**
  - `alembic.ini`, `env.py` (async, honours `search_path` from the URL options), `versions/0001_initial.py` (verbatim SQL of `00001`–`00003`);
  - `alembic_commands.alembic_upgrade()`.
- `tests/conftest.py`: per-test schema isolation, plus an `orm_models` ↔ database smoke test that compares every ORM column against `information_schema`.
- **Tests:** port `db_test.go` and `migrations_00002_test.go`.

### Task 4: Auth models
- **Ports:** `internal/auth` (org bootstrap, users with the last-admin guard, sessions, API keys, principal, validation).
- **Tests:** port `auth/*_test.go`.

### Task 5: API core
- `create_app(settings)` with a lifespan and the error envelope, which preserves the `WriteError` order: `APIError`, `ValidationError`, the mapping table, then 500.
- A 1 MiB JSON body limit, returning 413 `bad_request`.
- Dependencies: `Authenticate` (Bearer API key or `of_session` cookie), `require_auth`, `require_admin`.
- `/healthz`, plus the auth, users and API-keys routers.
- Static UI serving (port of `webui.go`).
- Every `/api/v1` 404 or 405 returns the envelope.
- **Tests:** port `httpapi/{errors,middleware,routes,auth_handlers,users_handlers,apikeys_handlers}_test.go`, `webui_test.go` and `app_test.go`, using `httpx.AsyncClient(transport=ASGITransport(app))`.

### Task 6: Definitions store
- **Ports:** `internal/definitions`:
  - apply with hash dedupe;
  - re-pinning of dependent forms;
  - dry-run via rollback;
  - a concurrent same-slug create that yields exactly one v1, using `ON CONFLICT` or a retry;
  - version reads and an immutable-version cache;
  - export.
- **Tests:** port `definitions/*_test.go`, including `TestApplyConcurrentSameNewSlug` with `asyncio.gather`.

### Task 7: Submissions
- **Ports:** `internal/submissions`:
  - create with receipt tokens (hashed) and a created hook inside the transaction;
  - get / get-for-update;
  - events;
  - list with the `created_at|id` cursor;
  - count by form;
  - public status with a constant-time token compare (`hmac.compare_digest`);
  - CSV export with the formula-escaping rule;
  - `set_assignee`.
- **Tests:** port `submissions/*_test.go`.

### Task 8: Definition, public and submission routes
- **Ports:** `definition_handlers.go`, `public_handlers.go` (plus the rate limiter at 20/min/IP with `Retry-After: 60`), `submission_handlers.go` and `json_types.go`.
- **Tests:** port the matching `httpapi` tests, `ratelimit_test.go` and `json_types_test.go`.

### Task 9: Job queue and worker service
- **Ports:** `internal/jobs`, keeping everything the same:
  - claim with `SKIP LOCKED`, the 5-minute lock with reclaim, backoff `min(5s·2^(n-1), 1h)`, and `max_attempts`;
  - `Permanent`, the failed hook, `retry` and `list`;
  - an injectable clock and poll interval.
- **Tests:** port `jobs/*_test.go`.

### Task 10: Actions
- **Ports:** `internal/actions`: `render` and `template_vars`, the mailers, `build_message` (header-injection safe), payloads, and the webhook handler:
  - headers `X-OpenForms-Event`, `X-OpenForms-Delivery` and `X-OpenForms-Signature` (HMAC-SHA256);
  - 10 s timeout;
  - retryable vs permanent status codes.
- Also ports the email and least-loaded assign handlers, and the success/failure events.
- **Tests:** port `actions/*_test.go`. The shared fixture from `testutil/fixture` becomes a pytest fixture.

### Task 11: Workflow engine and routes
- **Ports:** `internal/workflow`:
  - `available`, `transition` (guards, required fields, `expectedState` → `state_conflict`, transactional enqueue, before-commit hook), `on_created`;
  - `update_fields`, `comment`, `assign`.
- Also ports `workflow_handlers.go` and `jobs_handlers.go`, and wires detail `transitions`.
- The worker runs in the app lifespan.
- **Tests:** port `workflow/*_test.go`, `workflow_handlers_test.go` and `app/workers_test.go`.

### Task 12: Client and CLI
- **Ports:** `internal/client` (httpx) and `internal/cli`:
  - injectable deps, config resolution, canonical YAML (key order and quoting of ambiguous strings), `load_local` (CRLF/BOM);
  - the admin, init (same templates), validate, push, diff, pull (`--check`) and serve/migrate commands on cyclopts.
- Output strings and exit codes are byte-identical.
- **Tests:** port `client_test.go` and `cli/*_test.go`, including the push → pull round trip against the ASGI app.

### Task 13: Seed and public config
- **Ports:** `internal/seed` and `cli/seed.go`, `OPENFORMS_SEED_DEMO` auto-seed in the lifespan, and `GET /api/v1/public/config`.
- **Tests:** port `seed_test.go`, `cli/seed_test.go`, `public_config_test.go` and `examples_test.go`.

### Task 14: Packaging, repo switch-over and cleanup
- `copy-dist.mjs` targets `src/openforms/server/ui`. Update `.gitignore`.
- **Dockerfile:** node build → `python:3.12-slim` + uv builds the wheel with the UI → `python:3.12-slim` runtime with the non-root `openforms` entrypoint. Keep the missing-UI guard.
- **Compose:** the E2E global setup calls `openforms admin create-api-key` inside the container.
- **Makefile:** `test` runs `uv run pytest`, `lint` runs `ruff` + `pyright`, and `dev-db`, `web`, `web-test`, `build` (wheel), `up`, `down` and `e2e` keep their jobs.
- **CI:** the `go` job becomes a `python` job (uv, ruff, pyright, pytest against a Postgres service).
- **Docs:** README and `docs/*` are updated for `pip install openforms` / `uvx openforms`, the dev commands and the Python requirements. Spec §2/§3/§6 get a pointer to this plan.
- **Delete:** `go.mod`, `go.sum`, `cmd/`, `internal/`, `schemas/*.go` and `examples/*.go`.

### Task 15: Final verification (definition of done)
1. `uv run ruff check && uv run ruff format --check && uv run pyright && uv run pytest` are all green.
2. `pnpm -C web typecheck test build` is green.
3. `docker compose --profile app up --build` gives: `/healthz` ok, `/demo` works, demo seeded; all 12 Playwright tests pass; the wrong-secret run fails only the webhook test.
4. `openforms init && openforms push` against the stack creates `/f/contact`.

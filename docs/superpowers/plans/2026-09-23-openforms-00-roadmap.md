# openforms — Delivery Roadmap (Plans 01–09)

> **For agentic workers:** This is the index for a series of implementation plans. Execute each plan with superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Where this roadmap says plans or tasks can run in parallel, use superpowers:dispatching-parallel-agents with **one git worktree per lane** (superpowers:using-git-worktrees) and merge lanes back to `main` at the end of each wave.

**Goal:** Ship openforms v1: a self-hosted, developer-first form platform with forms-as-code, per-form submission workflows, a headless API + SDK, hosted forms and embeds, a full admin UI with visual editors, and a live `/demo` page.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md` (binding contracts: §4 data model, §6 Go package signatures, §7 HTTP API, §9 frontend APIs). Every plan argues from the spec; executors read both.

## Plans

| # | File | Deliverable | Depends on |
|---|---|---|---|
| 01 | `2026-09-23-openforms-01-foundation.md` | Go server boots, migrates, logs in users, issues API keys, serves `/healthz` | — |
| 02 | `2026-09-23-openforms-02-definition-language.md` | JSON Schemas, conformance fixtures, `internal/definition` | — |
| 03 | `2026-09-23-openforms-03-definitions-submissions-api.md` | Versioned definitions, public + authenticated submissions API | 01, 02 |
| 04 | `2026-09-23-openforms-04-workflow-engine.md` | Transitions, guards, fields, comments, assignment, job queue, webhook/email/assign actions | 03 |
| 05 | `2026-09-23-openforms-05-cli.md` | `openforms admin/init/validate/push/pull/diff` | 02; remote cmds tested against 03 |
| 06 | `2026-09-23-openforms-06-sdk-renderer-hosted.md` | `@openforms/sdk`, `@openforms/react`, hosted pages, `embed.js` | 02 |
| 07 | `2026-09-23-openforms-07-admin-app.md` | Admin SPA: login, inbox, detail, forms/workflows overview, users, keys, jobs | 06 |
| 08 | `2026-09-23-openforms-08-visual-editors.md` | Form editor, workflow editor + diagram, version history/diff | 07 |
| 09 | `2026-09-23-openforms-09-demo-packaging-e2e.md` | Examples, `seed --demo`, `/demo`, Docker image, CI, docs, Playwright E2E | all |
| 10 | `2026-09-24-openforms-10-python-backend.md` | Replace the Go server + CLI with a Prefect-style Python package (FastAPI, SQLAlchemy async, Alembic, cyclopts); same API, schema, CLI | 01–09 |

## Execution waves (multi-agent)

```
Wave 1  ─┬─ Lane A: Plan 01 Foundation            (Go infra/auth)
         └─ Lane B: Plan 02 Definition language   (pure Go, no DB)
                       │
Wave 2  ─┬─ Lane A: Plan 03 Definitions & submissions API
         └─ Lane B: Plan 06 SDK/renderer/hosted/embed   (TS; API mocked with MSW per spec §7)
                       │
Wave 3  ─┬─ Lane A: Plan 04 Workflow engine & actions
         ├─ Lane B: Plan 05 CLI
         └─ Lane C: Plan 07 Admin app                   (TS; API mocked with MSW per spec §7)
                       │
Wave 4  ─┬─ Lane A: Plan 08 Visual editors
         └─ Lane B: Plan 09 Tasks 1–4 (examples, seed, public config, Docker/compose/Makefile/CI)
                       │
Wave 5  ── Plan 09 remaining tasks (demo page, docs, Playwright E2E) — single lane, needs everything merged
```

**Why these lanes don't collide:** lanes in the same wave touch disjoint directories.

| Wave | Lane A | Lane B | Lane C |
|---|---|---|---|
| 1 | `go.mod`, `internal/{config,db,auth,httpapi,webui,app,cli}`, `cmd/`, `docker-compose.yml`, `Makefile` | `schemas/`, `internal/definition/` (Lane B adds its deps with `go get` and **must rebase `go.mod`/`go.sum` onto Lane A at merge**; resolve by re-running `go mod tidy`) | — |
| 2 | `internal/{definitions,submissions}`, `internal/db/migrations/00002*`, `internal/httpapi/*` | `web/` only | — |
| 3 | `internal/{jobs,actions,workflow}`, `00003*`, `internal/httpapi/*`, `internal/app/*` | `internal/{client,cli}` (only adds files + one `root.go` line) | `web/apps/admin/` only |
| 4 | `web/apps/admin/src/editors/**`, admin routes file (one-line additions) | `examples/`, `internal/cli/seed.go`, `Dockerfile`, compose, `.github/` | — |

**Merge protocol at the end of each wave:** merge lane branches into `main` one at a time → `go mod tidy` → `make test` (and `make web-test` once `web/` exists) → fix breaks on `main` before the next wave starts.

## Intra-plan parallelism

Each plan marks tasks with a **Parallel group** line. Tasks in the same group touch disjoint files and may be dispatched to concurrent subagents (each in its own worktree or with strictly disjoint file sets). Tasks without a group run sequentially in the listed order. A group only starts after every earlier sequential task is done.

## Prerequisites (developer machine / CI)

- Go ≥ 1.26, Node 22 LTS, pnpm 9 (`corepack enable`), Docker (for Postgres/Mailpit), Git.
- `docker compose up -d postgres mailpit` before any DB-backed Go test (Plan 01 Task 1 creates the compose file).
- Windows: run commands from Git Bash; all Makefile targets are POSIX-shell.

## Definition of done (whole product)

1. `make test web-test e2e` green locally and in CI.
2. `docker compose --profile app up --build` → `http://localhost:8080/demo` runs the full loop: submit → track → review → email visible in Mailpit (`http://localhost:8025`).
3. `openforms init && openforms push` against that stack creates the sample form, visible at `/f/contact`.
4. Docs in `docs/` cover getting started, definitions, workflows, API, CLI, self-hosting.

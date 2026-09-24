# openforms

**Forms as code. Workflows built in.**

openforms is an open-source, self-hosted form platform for developers, and an alternative to Typeform and Tally for teams that want their forms in git.

- **Forms as code.** Forms and workflows are YAML or JSON files. `openforms push` deploys them; `openforms pull --check` catches drift in CI.
- **Submission workflows.** Every form can have a state machine with custom states, role and required-field guards, and actions (webhook, email, auto-assign) that run reliably in the background with retries.
- **Headless first, hosted too.** Use the REST API, the TypeScript SDK or the React renderer, or share a hosted page (`/f/<slug>`) and embed it anywhere with two lines of HTML.
- **A real admin UI.** A reviewer inbox, submission timelines, and visual editors for forms and workflows.
- **One container.** A single Python package (FastAPI + SQLAlchemy) with the web UI inside, plus PostgreSQL.

## Try it in two minutes

```bash
git clone https://github.com/openforms/openforms && cd openforms
docker compose --profile app up -d --build
```

Then open:

| URL | What |
|---|---|
| http://localhost:8080/demo | Live demo: define → collect → track & review → embed |
| http://localhost:8080/admin | Admin inbox (demo accounts are listed on the login page, password `demo1234`) |
| http://localhost:8080/f/job-application | Hosted form |
| http://localhost:8025 | Mailpit, which catches every email the demo sends |

## A form and its workflow

```yaml
# openforms/forms/contact.yaml
slug: contact
title: Contact us
workflow: contact-triage
settings: { public: true }
fields:
  - { key: email, type: email, label: Email, required: true }
  - { key: message, type: textarea, label: Message, required: true }
```

```yaml
# openforms/workflows/contact-triage.yaml
slug: contact-triage
title: Contact triage
initial: open
states:
  - { key: open, label: Open }
  - { key: resolved, label: Resolved, terminal: true }
fields:
  - { key: reply, type: textarea, label: Reply }
transitions:
  - key: resolve
    label: Resolve
    from: [open]
    to: resolved
    guard: { roles: [support], requireFields: [reply] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Re: your message", body: "{{submission.fields.reply}}" }
```

```bash
openforms validate && openforms push
```

## Documentation

- [Getting started](docs/getting-started.md)
- [Form definitions](docs/definitions.md)
- [Workflows](docs/workflows.md)
- [HTTP API](docs/api.md)
- [CLI](docs/cli.md)
- [Self-hosting](docs/self-hosting.md)

## Architecture

```
            ┌──────────────── openforms (one Python process: FastAPI) ────────────┐
 browser ──▶│ /f, /s (hosted)  /admin (inbox + editors)  /demo  /embed.js         │
 your app ─▶│ /api/v1  ── definitions store ── submissions ── workflow engine    │
 CLI ──────▶│                                               └─▶ job queue ──▶ webhook / email / assign
            └───────────────────────────────────┬──────────────────────────────────┘
                                                ▼
                                           PostgreSQL
```

Actions are written to the job queue in the same database transaction as the state change, so a failed transition never fires actions and a committed one always does (at least once).

## Repository layout

| Path | Contents |
|---|---|
| `src/openforms` | The Python package: `definition` (the definition language), `server` (`database`, `models`, `api`, `services`, `actions`, `workflow`), `cli` and `client` |
| `tests/` | pytest suite (runs against PostgreSQL) |
| `schemas/` | JSON Schemas and conformance fixtures shared by Python and TypeScript |
| `web/` | pnpm workspace: `@openforms/sdk`, `@openforms/react`, embed script, hosted/admin/demo apps |
| `examples/openforms` | The demo bundle loaded by `openforms seed --demo` |
| `e2e/` | Playwright end-to-end tests |

## Development

Requirements: Python ≥ 3.11 with [uv](https://docs.astral.sh/uv/), Node 22, pnpm 9, Docker.

```bash
make sync        # uv sync: create .venv with the package (editable) and dev tools
make dev-db      # start Postgres (port 54329) and Mailpit
make test        # pytest
make lint        # ruff + pyright
make web-test    # TypeScript typecheck + tests
make build       # build the web apps and a wheel in dist/
make e2e         # full stack + Playwright
```

Run the server from source with `OPENFORMS_DATABASE_URL=postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable uv run openforms serve`.

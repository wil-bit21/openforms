# Running openforms

This is the single reference for getting the stack up, in every mode. For the
concepts (forms, workflows, actions), see [docs/getting-started.md](docs/getting-started.md).

Requirements: Docker with the Compose plugin. For source builds: Python ≥ 3.11
with [uv](https://docs.astral.sh/uv/), Node 22, pnpm 9.

## 1. Full stack in one command (recommended)

Everything — PostgreSQL, Mailpit and openforms itself, seeded with a demo — in
one `docker compose` invocation:

```bash
git clone https://github.com/openforms/openforms && cd openforms
docker compose --profile app up -d --build
curl http://localhost:8080/healthz   # {"status":"ok"}
```

| URL | What |
|---|---|
| http://localhost:8080/demo | Guided demo: define → collect → track & review → embed |
| http://localhost:8080/admin | Admin inbox (`admin@demo.local` / `demo1234`) |
| http://localhost:8080/f/job-application | A hosted demo form |
| http://localhost:8025 | Mailpit — every email the demo sends lands here |

Stop and remove everything, including the database volume:

```bash
docker compose --profile app down -v
```

## 2. Dev services only (run the server from source)

Useful when iterating on the Python or web code. This starts just Postgres
(port `54329`) and Mailpit:

```bash
make sync         # uv sync: .venv with the package (editable) + dev tools
make dev-db        # docker compose up -d postgres mailpit
OPENFORMS_DATABASE_URL=postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable \
  uv run openforms serve
```

The web UIs (admin/hosted/demo/embed) are pre-built and committed to
`src/openforms/server/ui`, so `openforms serve` alone is enough. To work on
the web UI itself, see `web/README.md`.

## 3. Everything, without Docker

```bash
make build                                 # builds the web apps and a wheel in dist/
uv tool install dist/openforms-*.whl
# start your own PostgreSQL 16, then:
OPENFORMS_DATABASE_URL=postgres://user:pass@host:5432/openforms?sslmode=disable openforms serve
```

## 4. Tests and checks

```bash
make test       # pytest, against the Postgres started by `make dev-db`
make lint       # ruff + pyright
make web-test   # TypeScript typecheck + tests
make e2e        # full stack (docker compose --profile app) + Playwright
```

## 5. The project site (GitHub Pages)

The static landing page published at the project's GitHub Pages URL lives in
[`site/`](site/) — a single self-contained `index.html`, no build step. Open
it directly in a browser, or serve it locally:

```bash
python3 -m http.server 8000 --directory site
```

It deploys automatically from `main` via
[`.github/workflows/pages.yml`](.github/workflows/pages.yml).

## Production

For a real deployment (TLS, SMTP, backups, running more than one instance),
see [docs/self-hosting.md](docs/self-hosting.md).

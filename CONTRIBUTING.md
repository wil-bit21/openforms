# Contributing to openforms

Thanks for helping! This guide covers setting up a checkout, the checks every change must pass, and how to send a pull request.

## Prerequisites

| Tool | Version | Used for |
|---|---|---|
| [uv](https://docs.astral.sh/uv/) | 0.8+ | Python, virtualenv, dependencies, running tools |
| Python | 3.11+ (uv can install it: `uv python install 3.12`) | the server and CLI |
| Node.js | 22 | the web apps and E2E tests |
| pnpm | 9 (`corepack enable`) | the web workspace |
| Docker | recent | Postgres, Mailpit and the E2E stack |

**Use uv for everything Python.** Do not use `pip`, `pipx` or a hand-made virtualenv: uv reads `pyproject.toml`, keeps `uv.lock` exact, and runs every tool at the version the lockfile pins.

## Set up a checkout

```bash
git clone https://github.com/openforms/openforms && cd openforms
uv sync                          # .venv with the package (editable) and the dev tools
uv run pre-commit install        # git hooks (see below)
make dev-db                      # Postgres on :54329 and Mailpit on :8025
pnpm -C web install              # web workspace
```

Run the server from source:

```bash
export OPENFORMS_DATABASE_URL="postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable"
uv run openforms seed --demo     # optional: demo forms, workflows and users
uv run openforms serve           # http://localhost:8080
```

For the web apps with hot reload, keep the server running and start an app's Vite dev server (for example `pnpm -C web/apps/admin dev`). Each app proxies `/api` to `:8080`. Run `make web` to build the apps into `src/openforms/server/ui`, where the server serves them.

### Working with Claude Code

The project settings (`.claude/settings.json`) enable the [Superpowers](https://github.com/obra/superpowers) plugin from `obra/superpowers-marketplace`. It adds skills for brainstorming, writing plans, test-driven development and systematic debugging, which is the workflow the plans in `docs/superpowers/` were written with. When you trust the folder, Claude Code offers to install it. To opt out for yourself, set `"superpowers@superpowers-marketplace": false` under `enabledPlugins` in `.claude/settings.local.json`.

## Dependencies

```bash
uv add httpx                     # runtime dependency
uv add --dev pytest-xdist        # development-only tool
uv lock --upgrade-package httpx  # upgrade one package
```

Commit `pyproject.toml` and `uv.lock` together. The `uv-lock` hook and CI (`uv sync --frozen`) fail if they disagree. Web dependencies are managed with `pnpm` inside `web/` and `e2e/`, and each has its own `pnpm-lock.yaml`.

## Checks

Every commit runs the pre-commit hooks from `.pre-commit-config.yaml`:

- file hygiene (trailing whitespace, final newline, LF line endings, valid YAML/TOML/JSON, no merge markers, large files or private keys)
- `uv-lock`, which keeps `uv.lock` in sync with `pyproject.toml`
- `ruff check --fix` and `ruff format`
- `pyright`
- `pnpm typecheck` for `web/` and `e2e/` when TypeScript files change

Hooks that fix files (ruff, whitespace) leave the fixes unstaged. Review them, `git add`, and commit again. To run everything by hand:

```bash
uv run pre-commit run --all-files   # or: make pre-commit
```

Tests:

```bash
uv run pytest                    # Python; needs the dev database (make dev-db)
pnpm -C web test                 # SDK, React, embed and app tests (Vitest)
make e2e                         # full Docker stack + Playwright (slow; CI runs it on every PR)
```

Each Python test runs in its own Postgres schema, so tests don't interfere with each other or with your dev data. Point them elsewhere with `OPENFORMS_TEST_DATABASE_URL`.

## Where things live

| Path | What |
|---|---|
| `src/openforms/definition/` | The definition language: parsing, validation, canonical hashing |
| `src/openforms/server/` | FastAPI app: `database` (SQLAlchemy + Alembic), `models`, `api`, `services`, `actions`, `workflow` |
| `src/openforms/cli/`, `client.py` | The `openforms` command and its HTTP client |
| `schemas/` | JSON Schemas and conformance fixtures shared by Python and TypeScript |
| `web/` | SDK, React renderer, embed script, hosted/admin/demo apps |
| `e2e/` | Playwright tests against the Docker stack |
| `tests/` | pytest suite, mirroring `src/openforms` |
| `docs/` | User documentation; update it with any behaviour change |

## Making changes

- **Tests come with the change.** Bug fixes get a test that fails without the fix. Features get tests at the level they live at: a pytest for API and model behaviour, a Vitest for UI, and a Playwright test for cross-cutting flows.
- **Definition schema changes** (`schemas/*.schema.json`) need three things: the Python validation in `src/openforms/definition/`, regenerated SDK types (`pnpm -C web/packages/sdk gen`), and cases in `schemas/fixtures/` that both test suites run.
- **Database changes** get a new Alembic migration in `src/openforms/server/database/_migrations/versions/`, numbered after the latest one (see `0002_password_resets.py`), plus the matching model in `orm_models.py`. Never edit a migration that has been released. `tests/server/test_database.py` checks that the models match the migrated schema.
- **API changes** update `docs/api.md` and, when the web apps use the endpoint, `web/packages/sdk/src/client.ts` and its tests.
- **CLI output** is stable. Scripts and CI depend on it, and `tests/cli/` pins it byte for byte.
- Keep pull requests focused. Refactors go in their own PR, separate from behaviour changes.

## Commits and pull requests

- Write commit messages in the [Conventional Commits](https://www.conventionalcommits.org/) style used in the history: `feat: …`, `fix: …`, `docs: …`, `build: …`, `chore: …`, `test: …`. Use the imperative mood and keep the subject under about 72 characters.
- Open the PR against `main`. Describe what changed and why, and how you tested it.
- CI must be green (the `python`, `web` and `e2e` jobs) and review comments must be addressed before merging.

## Reporting security issues

Please do not open public issues for vulnerabilities. Report them privately through GitHub's **Security → Report a vulnerability** on the repository.

## License

By contributing you agree that your contributions are licensed under the [MIT License](LICENSE).

# openforms Plan 01: Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents), each in its own worktree; merge them back before the next sequential task.

**Goal:** A Go server binary `openforms` that boots, migrates Postgres, bootstraps the single org, logs users in with session cookies, issues API keys, manages users (admin-only), serves `/healthz`, and serves the (not-yet-built) web UI shell with a helpful 503.

**Architecture:** One Go module at the repo root. `internal/config` reads env vars; `internal/db` opens a pgx pool and runs goose migrations embedded in the binary; `internal/auth` owns orgs/users/sessions/API keys; `internal/httpapi` is a chi router with a single error envelope, authentication middleware and per-area `mountX` files; `internal/webui` serves embedded SPA builds; `internal/app` wires everything; `internal/cli` exposes `serve` and `migrate` via cobra. Tests that touch Postgres get an isolated schema via `dbtest`.

**Tech Stack:** Go ≥ 1.24, chi v5, pgx v5 (pgxpool), goose v3 (Provider API, embedded SQL), google/uuid, x/crypto/bcrypt, cobra, Postgres 16 + Mailpit via docker compose.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md` (binding: §2, §3, §4 migration 00001, §6.1, §6.2, §6.5, §6.11, §6.12, §6.13, §6.15, §7.1, §7.2 rows owned by Plan 01, §8 `serve`/`migrate`). Roadmap: `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md`.

## Global Constraints

- Go module path `github.com/openforms/openforms`; `go 1.24` directive in `go.mod`.
- Postgres 16 on host port **54329**, user/password/db `openforms`; Mailpit SMTP **1025**, UI **8025**.
- Default test DSN `postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable`, override with `OPENFORMS_TEST_DATABASE_URL`.
- All JSON over the wire is camelCase; timestamps RFC 3339 UTC.
- Error envelope: `{"error": {"code": "...", "message": "...", "details": [{"path","message"}]}}` (`details` omitted when empty).
- Session cookie `of_session`: HttpOnly, SameSite=Lax, Path=/, Secure per `Config.CookieSecure`, TTL 30 days. Session tokens: 32 random bytes base64url; DB stores sha256 hex.
- API keys: `"ofk_"` + 40 base62 chars; `prefix` = first 8 chars of the plaintext; DB stores sha256 hex.
- bcrypt cost 12; passwords ≥ 8 characters; emails lower-cased on write.
- Every Vite app is served under `/_app/<name>/`; SPA fallbacks per spec §6.12.
- Commit after every task; conventional commit prefixes (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).
- Commands are written for **Git Bash on Windows** (also valid on macOS/Linux). `make` is optional (install with `choco install make` or `winget install GnuWin32.Make`); every step shows the raw command.

## Review Focus

1. **Last admin removal** — demoting or deleting the only admin must fail with 422 `validation_failed` and leave the user unchanged; otherwise everyone is locked out of admin endpoints. (Tests: Task 6 service, Task 10 HTTP.)
2. **Email case** — `Ada@Example.com` and `ada@example.com` are the same user: login works with any case and a second create returns 409 `email_taken`. (Tests: Task 6, Task 9.)
3. **Revoked keys and expired sessions** — a revoked API key or expired/logged-out session must yield 401 on protected endpoints, not silently act as anonymous-but-allowed. (Tests: Task 7, Task 9, Task 10.)
4. **Malformed / oversized bodies** — invalid JSON → 400 `bad_request`, body > 1 MiB → 413 `bad_request`, never a 500. Unknown `/api/v1/*` paths return the JSON envelope 404, not the SPA HTML. (Tests: Task 8.)
5. **Web UI not built** — with only `.gitkeep` in `dist/`, `/admin` returns 503 "web UI not built; run `make web`" while `/healthz` and the API keep working; path traversal under `/_app/` never serves files outside `dist/`. (Tests: Task 4.)

## Cross-plan notes

- `internal/definition/problem.go` (Task 5) defines `definition.Problem` and `definition.ValidationError` exactly as spec §6.4, because `httpapi.APIError.Details` and auth input validation need them before Plan 02 lands. **Plan 02 owns the rest of `internal/definition`.** If Plan 02 merges with its own definitions of these two types, keep one copy (delete whichever duplicates) — the shapes are identical.
- `httpapi.Deps` and `app.App` contain only the fields whose packages exist after Plan 01 (`Config`, `Pool`, `Auth`, `Web`, `OrgID`). `Pool` is an additive field (needed by `/healthz`). Plans 03/04 add `Defs`, `Subs`, `Engine`, `Queue` exactly as in spec §6.11/§6.13.
- **How later plans add error mappings:** append a row to `errorMappings` in `internal/httpapi/errors.go` (sentinel matched with `errors.Is`). Typed errors (`*APIError`, `*definition.ValidationError`) are matched with `errors.As` before the table.
- Additive helpers beyond the spec: `config.Config.RequireDatabase()`, `auth.SessionTTL`, `dbtest.NewURL(t)` (isolated schema DSN, used by `app`/`cli` tests).

## File map

```
.gitignore, go.mod, go.sum, Makefile, docker-compose.yml
cmd/openforms/main.go
internal/config/config.go, config_test.go
internal/db/db.go, db_test.go, migrations/00001_core.sql
internal/db/dbtest/dbtest.go
internal/definition/problem.go, problem_test.go
internal/auth/errors.go, principal.go, principal_test.go
internal/auth/service.go, validate.go, users.go, users_test.go
internal/auth/tokens.go, sessions.go, apikeys.go, sessions_test.go, apikeys_test.go
internal/httpapi/errors.go, errors_test.go, middleware.go, middleware_test.go, routes.go, routes_test.go
internal/httpapi/json.go, auth_handlers.go, auth_handlers_test.go
internal/httpapi/users_handlers.go, users_handlers_test.go, apikeys_handlers.go, apikeys_handlers_test.go
internal/testutil/testutil.go
internal/webui/webui.go, webui_test.go, dist/.gitkeep
internal/app/app.go, app_test.go
internal/cli/root.go, serve.go, migrate.go, cli_test.go
```

## Task order

| Task | Title | Parallel group |
|---|---|---|
| 1 | Repository scaffold + dev services | — |
| 2 | Config | A |
| 3 | Database, migrations, dbtest | A |
| 4 | Web UI handler | A |
| 5 | Problem types + auth principal | A |
| 6 | Auth service: org + users | — (needs 3, 5) |
| 7 | Auth service: sessions + API keys | — |
| 8 | HTTP core: errors, JSON, middleware, router, healthz, testutil | — |
| 9 | Auth endpoints (login/logout/me) | B lane 1 |
| 10 | Users + API keys endpoints | B lane 1 (after 9) |
| 11 | App wiring + CLI `serve`/`migrate` | B lane 2 |

Group A lanes each run `go get` for their own dependencies; at merge, resolve `go.mod`/`go.sum` conflicts by taking both sides and running `go mod tidy`.

---

### Task 1: Repository scaffold + dev services

**Files:**
- Create: `.gitignore`, `go.mod`, `docker-compose.yml`, `Makefile`

**Interfaces:**
- Consumes: nothing.
- Produces: module `github.com/openforms/openforms`; Postgres at `localhost:54329` (`openforms`/`openforms`/`openforms`); Mailpit SMTP `localhost:1025`, UI `http://localhost:8025`; Makefile targets `dev-db`, `test`, `build`, `lint`.

- [ ] **Step 1: Initialise git and the Go module**

Run (from `D:/Github/openforms` in Git Bash):
```bash
git init -b main
go mod init github.com/openforms/openforms
go mod edit -go=1.24
```
Expected: `Initialized empty Git repository ...` and `go: creating new go.mod: module github.com/openforms/openforms`.

- [ ] **Step 2: Write `.gitignore`**

```gitignore
/bin/
/dist/
*.exe
*.test
*.out
coverage.*
node_modules/
.env
.env.*
!.env.example
/internal/webui/dist/*
!/internal/webui/dist/.gitkeep
/test-results/
/playwright-report/
.DS_Store
.idea/
.vscode/
```

- [ ] **Step 3: Write `docker-compose.yml`**

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: openforms
      POSTGRES_PASSWORD: openforms
      POSTGRES_DB: openforms
    ports:
      - "54329:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openforms -d openforms"]
      interval: 2s
      timeout: 3s
      retries: 30

  mailpit:
    image: axllent/mailpit:v1.21
    ports:
      - "1025:1025"
      - "8025:8025"

  # Completed in Plan 09 (Dockerfile, demo seeding). Only started with `--profile app`.
  openforms:
    profiles: ["app"]
    build: .
    environment:
      OPENFORMS_DATABASE_URL: postgres://openforms:openforms@postgres:5432/openforms?sslmode=disable
      OPENFORMS_BASE_URL: http://localhost:8080
      OPENFORMS_SMTP_HOST: mailpit
      OPENFORMS_SMTP_PORT: "1025"
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
      mailpit:
        condition: service_started

volumes:
  pgdata:
```

- [ ] **Step 4: Write `Makefile`** (recipe lines are indented with a TAB)

```makefile
.PHONY: dev-db test build lint tidy

dev-db:
	docker compose up -d postgres mailpit

test:
	go test ./... -count=1

build:
	go build -o bin/openforms ./cmd/openforms

lint:
	go vet ./...
	@test -z "$$(gofmt -l .)" || (echo "gofmt needed on:"; gofmt -l .; exit 1)

tidy:
	go mod tidy
```

- [ ] **Step 5: Start dev services and verify**

Run:
```bash
docker compose up -d postgres mailpit
docker compose exec postgres pg_isready -U openforms -d openforms
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8025/
```
Expected: `/var/run/postgresql:5432 - accepting connections` and `200`. (If `pg_isready` reports "no response", wait 5 s and rerun.)

- [ ] **Step 6: Commit**

```bash
git add .gitignore go.mod docker-compose.yml Makefile docs
git commit -m "chore: scaffold repository, dev services and Makefile"
```

---

### Task 2: Config

**Parallel group:** A (touches only `internal/config/`)

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (spec §6.1, plus additive `RequireDatabase`):
```go
type Config struct { HTTPAddr, DatabaseURL, BaseURL, SMTPHost string; SMTPPort int; SMTPUsername, SMTPPassword, SMTPFrom, WebhookSecret string; WorkerConcurrency int; DemoMode, CookieSecure bool }
func Load() (Config, error)
func (c Config) RequireDatabase() error // error "OPENFORMS_DATABASE_URL is required" when empty
```

- [ ] **Step 1: Write the failing test**

`internal/config/config_test.go`:
```go
package config_test

import (
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/config"
)

var allKeys = []string{
	"OPENFORMS_HTTP_ADDR", "OPENFORMS_DATABASE_URL", "OPENFORMS_BASE_URL",
	"OPENFORMS_SMTP_HOST", "OPENFORMS_SMTP_PORT", "OPENFORMS_SMTP_USERNAME",
	"OPENFORMS_SMTP_PASSWORD", "OPENFORMS_SMTP_FROM", "OPENFORMS_WEBHOOK_SECRET",
	"OPENFORMS_WORKER_CONCURRENCY", "OPENFORMS_DEMO", "OPENFORMS_COOKIE_SECURE",
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, k := range allKeys {
		t.Setenv(k, "")
	}
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q", c.HTTPAddr)
	}
	if c.BaseURL != "http://localhost:8080" {
		t.Errorf("BaseURL = %q", c.BaseURL)
	}
	if c.SMTPPort != 1025 {
		t.Errorf("SMTPPort = %d", c.SMTPPort)
	}
	if c.SMTPFrom != "openforms@localhost" {
		t.Errorf("SMTPFrom = %q", c.SMTPFrom)
	}
	if c.WorkerConcurrency != 4 {
		t.Errorf("WorkerConcurrency = %d", c.WorkerConcurrency)
	}
	if c.DemoMode || c.CookieSecure {
		t.Errorf("DemoMode=%v CookieSecure=%v, want false/false", c.DemoMode, c.CookieSecure)
	}
	if c.DatabaseURL != "" || c.SMTPHost != "" || c.WebhookSecret != "" {
		t.Errorf("expected empty optional values, got %+v", c)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENFORMS_HTTP_ADDR", "127.0.0.1:9000")
	t.Setenv("OPENFORMS_DATABASE_URL", "postgres://x")
	t.Setenv("OPENFORMS_BASE_URL", "https://forms.example.com/")
	t.Setenv("OPENFORMS_SMTP_HOST", "smtp.example.com")
	t.Setenv("OPENFORMS_SMTP_PORT", "2525")
	t.Setenv("OPENFORMS_SMTP_USERNAME", "u")
	t.Setenv("OPENFORMS_SMTP_PASSWORD", "p")
	t.Setenv("OPENFORMS_SMTP_FROM", "noreply@example.com")
	t.Setenv("OPENFORMS_WEBHOOK_SECRET", "s3cret")
	t.Setenv("OPENFORMS_WORKER_CONCURRENCY", "8")
	t.Setenv("OPENFORMS_DEMO", "1")

	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := config.Config{
		HTTPAddr: "127.0.0.1:9000", DatabaseURL: "postgres://x", BaseURL: "https://forms.example.com",
		SMTPHost: "smtp.example.com", SMTPPort: 2525, SMTPUsername: "u", SMTPPassword: "p",
		SMTPFrom: "noreply@example.com", WebhookSecret: "s3cret", WorkerConcurrency: 8,
		DemoMode: true, CookieSecure: true,
	}
	if c != want {
		t.Errorf("got %+v\nwant %+v", c, want)
	}
}

func TestCookieSecureExplicitOverride(t *testing.T) {
	clearEnv(t)
	t.Setenv("OPENFORMS_BASE_URL", "https://forms.example.com")
	t.Setenv("OPENFORMS_COOKIE_SECURE", "false")
	c, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.CookieSecure {
		t.Error("CookieSecure should be false when explicitly disabled")
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		"OPENFORMS_SMTP_PORT":          "abc",
		"OPENFORMS_WORKER_CONCURRENCY": "0",
		"OPENFORMS_DEMO":               "maybe",
		"OPENFORMS_COOKIE_SECURE":      "yes please",
	}
	for key, val := range cases {
		t.Run(key, func(t *testing.T) {
			clearEnv(t)
			t.Setenv(key, val)
			_, err := config.Load()
			if err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("err = %v, want error mentioning %s", err, key)
			}
		})
	}
}

func TestRequireDatabase(t *testing.T) {
	if err := (config.Config{}).RequireDatabase(); err == nil || !strings.Contains(err.Error(), "OPENFORMS_DATABASE_URL") {
		t.Fatalf("err = %v", err)
	}
	if err := (config.Config{DatabaseURL: "postgres://x"}).RequireDatabase(); err != nil {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — `undefined: config.Load` (package does not compile).

- [ ] **Step 3: Write minimal implementation**

`internal/config/config.go`:
```go
// Package config loads openforms runtime configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr          string // OPENFORMS_HTTP_ADDR, default ":8080"
	DatabaseURL       string // OPENFORMS_DATABASE_URL, required for serve/migrate/seed/admin
	BaseURL           string // OPENFORMS_BASE_URL, default "http://localhost:8080"
	SMTPHost          string // OPENFORMS_SMTP_HOST; empty → log mailer
	SMTPPort          int    // OPENFORMS_SMTP_PORT, default 1025
	SMTPUsername      string // OPENFORMS_SMTP_USERNAME
	SMTPPassword      string // OPENFORMS_SMTP_PASSWORD
	SMTPFrom          string // OPENFORMS_SMTP_FROM, default "openforms@localhost"
	WebhookSecret     string // OPENFORMS_WEBHOOK_SECRET; empty → no signature header
	WorkerConcurrency int    // OPENFORMS_WORKER_CONCURRENCY, default 4
	DemoMode          bool   // OPENFORMS_DEMO ("true"/"1")
	CookieSecure      bool   // OPENFORMS_COOKIE_SECURE, default: BaseURL starts with https
}

// Load reads configuration from the environment. Empty variables count as unset.
func Load() (Config, error) {
	c := Config{
		HTTPAddr:      getenv("OPENFORMS_HTTP_ADDR", ":8080"),
		DatabaseURL:   os.Getenv("OPENFORMS_DATABASE_URL"),
		BaseURL:       strings.TrimRight(getenv("OPENFORMS_BASE_URL", "http://localhost:8080"), "/"),
		SMTPHost:      os.Getenv("OPENFORMS_SMTP_HOST"),
		SMTPUsername:  os.Getenv("OPENFORMS_SMTP_USERNAME"),
		SMTPPassword:  os.Getenv("OPENFORMS_SMTP_PASSWORD"),
		SMTPFrom:      getenv("OPENFORMS_SMTP_FROM", "openforms@localhost"),
		WebhookSecret: os.Getenv("OPENFORMS_WEBHOOK_SECRET"),
	}
	var err error
	if c.SMTPPort, err = intEnv("OPENFORMS_SMTP_PORT", 1025, 1); err != nil {
		return Config{}, err
	}
	if c.WorkerConcurrency, err = intEnv("OPENFORMS_WORKER_CONCURRENCY", 4, 1); err != nil {
		return Config{}, err
	}
	if c.DemoMode, err = boolEnv("OPENFORMS_DEMO", false); err != nil {
		return Config{}, err
	}
	if c.CookieSecure, err = boolEnv("OPENFORMS_COOKIE_SECURE", strings.HasPrefix(c.BaseURL, "https://")); err != nil {
		return Config{}, err
	}
	return c, nil
}

// RequireDatabase reports an error when no database URL is configured.
func (c Config) RequireDatabase() error {
	if c.DatabaseURL == "" {
		return errors.New("OPENFORMS_DATABASE_URL is required")
	}
	return nil
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func intEnv(key string, def, min int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return 0, fmt.Errorf("%s must be an integer >= %d, got %q", key, min, v)
	}
	return n, nil
}

func boolEnv(key string, def bool) (bool, error) {
	switch strings.ToLower(os.Getenv(key)) {
	case "":
		return def, nil
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true/false/1/0, got %q", key, os.Getenv(key))
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS for `TestLoadDefaults`, `TestLoadOverrides`, `TestCookieSecureExplicitOverride`, `TestLoadRejectsInvalidValues` (4 subtests), `TestRequireDatabase`.

- [ ] **Step 5: Commit**

```bash
git add internal/config
git commit -m "feat: load configuration from environment"
```

---

### Task 3: Database, migrations, dbtest

**Parallel group:** A (touches only `internal/db/`, `go.mod`, `go.sum`)

**Files:**
- Create: `internal/db/db.go`, `internal/db/migrations/00001_core.sql`, `internal/db/dbtest/dbtest.go`
- Test: `internal/db/db_test.go`

**Interfaces:**
- Consumes: Postgres from Task 1.
- Produces (spec §6.2 + additive `NewURL`, `DefaultURL`):
```go
// package db
type DBTX interface { Exec(...) (pgconn.CommandTag, error); Query(...) (pgx.Rows, error); QueryRow(...) pgx.Row }
func Open(ctx context.Context, url string) (*pgxpool.Pool, error)
func Migrate(ctx context.Context, pool *pgxpool.Pool) error
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error
// package dbtest
const DefaultURL = "postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable"
func NewURL(t testing.TB) string          // creates schema t_<16 hex>, returns DSN with search_path=<schema>; drops schema on cleanup
func New(t testing.TB) *pgxpool.Pool      // NewURL + Open + Migrate; pool closed on cleanup
```

- [ ] **Step 1: Add dependencies**

Run:
```bash
go get github.com/jackc/pgx/v5@latest github.com/pressly/goose/v3@latest
```
Expected: `go: added github.com/jackc/pgx/v5 ...` and `go: added github.com/pressly/goose/v3 ...`.

- [ ] **Step 2: Write the failing test**

`internal/db/db_test.go`:
```go
package db_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/db/dbtest"
)

func countOrgs(t *testing.T, q db.DBTX) int {
	t.Helper()
	var n int
	if err := q.QueryRow(context.Background(), `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
		t.Fatalf("count orgs: %v", err)
	}
	return n
}

func TestMigrateCreatesCoreTables(t *testing.T) {
	pool := dbtest.New(t)
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = ANY($1)`,
		[]string{"orgs", "users", "sessions", "api_keys"}).Scan(&n)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if n != 4 {
		t.Fatalf("found %d core tables, want 4", n)
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	pool := dbtest.New(t)
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestWithTxCommits(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO orgs (id, name) VALUES ($1, 'x')`, uuid.New())
		return err
	})
	if err != nil {
		t.Fatalf("WithTx: %v", err)
	}
	if got := countOrgs(t, pool); got != 1 {
		t.Fatalf("orgs = %d, want 1", got)
	}
}

func TestWithTxRollsBackOnError(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()
	boom := errors.New("boom")
	err := db.WithTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `INSERT INTO orgs (id, name) VALUES ($1, 'x')`, uuid.New()); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	if got := countOrgs(t, pool); got != 0 {
		t.Fatalf("orgs = %d, want 0 after rollback", got)
	}
}

func TestSchemasAreIsolated(t *testing.T) {
	a := dbtest.New(t)
	b := dbtest.New(t)
	if _, err := a.Exec(context.Background(), `INSERT INTO orgs (id, name) VALUES ($1, 'a')`, uuid.New()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if got := countOrgs(t, b); got != 0 {
		t.Fatalf("schema b sees %d orgs, want 0", got)
	}
}
```
Also run `go get github.com/google/uuid@latest` (the test imports it).

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/db/...`
Expected: FAIL — `no required module provides package github.com/openforms/openforms/internal/db` / `undefined: db.Migrate`.

- [ ] **Step 4: Write the migration**

`internal/db/migrations/00001_core.sql` (spec §4; the Down statements are split one per line so goose runs each separately):
```sql
-- +goose Up
CREATE TABLE orgs (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE users (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  email text NOT NULL,
  name text NOT NULL,
  password_hash text NOT NULL,
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_org_email ON users (org_id, lower(email));
CREATE TABLE sessions (
  token_hash text PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE api_keys (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  name text NOT NULL,
  prefix text NOT NULL,
  key_hash text NOT NULL UNIQUE,
  roles text[] NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now(),
  last_used_at timestamptz,
  revoked_at timestamptz
);
-- +goose Down
DROP TABLE api_keys;
DROP TABLE sessions;
DROP TABLE users;
DROP TABLE orgs;
```

- [ ] **Step 5: Write `internal/db/db.go`**

```go
// Package db opens the Postgres pool, runs embedded migrations and provides transaction helpers.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DBTX is satisfied by *pgxpool.Pool, *pgxpool.Conn and pgx.Tx.
type DBTX interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Open creates a pool and verifies connectivity.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending embedded migrations (goose "up").
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return err
	}
	// The *sql.DB wrapper borrows connections from the pool and keeps none idle,
	// so it is intentionally not closed (closing must not affect the pool).
	sqlDB := stdlib.OpenDBFromPool(pool)
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys)
	if err != nil {
		return fmt.Errorf("init migrations: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

// WithTx runs fn in a transaction, committing on nil and rolling back otherwise.
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after Commit
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
```

- [ ] **Step 6: Write `internal/db/dbtest/dbtest.go`**

```go
// Package dbtest gives each test an isolated, migrated Postgres schema.
package dbtest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/db"
)

const DefaultURL = "postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable"

func baseURL() string {
	if v := os.Getenv("OPENFORMS_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return DefaultURL
}

// NewURL creates an empty schema t_<16 hex> and returns a DSN whose connections
// use it as search_path. The schema is dropped when the test ends.
func NewURL(t testing.TB) string {
	t.Helper()
	ctx := context.Background()
	base := baseURL()
	conn, err := pgx.Connect(ctx, base)
	if err != nil {
		t.Fatalf("connect to test database: %v (start it with `docker compose up -d postgres`)", err)
	}
	defer conn.Close(ctx)

	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	schema := "t_" + hex.EncodeToString(b)
	if _, err := conn.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		c, err := pgx.Connect(context.Background(), base)
		if err != nil {
			t.Logf("dbtest cleanup connect: %v", err)
			return
		}
		defer c.Close(context.Background())
		if _, err := c.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("dbtest drop schema %s: %v", schema, err)
		}
	})

	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse test database url: %v", err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	return u.String()
}

// New returns a pool bound to a fresh, fully migrated schema.
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dsn := NewURL(t)
	pool, err := db.Open(context.Background(), dsn)
	if err != nil {
		t.Fatalf("open test pool: %v", err)
	}
	t.Cleanup(pool.Close) // registered after the schema drop, so it runs first
	if err := db.Migrate(context.Background(), pool); err != nil {
		t.Fatalf("migrate test schema: %v", err)
	}
	return pool
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run (Postgres from Task 1 must be running):
```bash
go mod tidy
go test ./internal/db/... -count=1 -v
```
Expected: PASS for all 5 tests.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/db
git commit -m "feat: postgres pool, embedded goose migrations and dbtest schemas"
```

---

### Task 4: Web UI handler

**Parallel group:** A (touches only `internal/webui/`)

**Files:**
- Create: `internal/webui/webui.go`, `internal/webui/dist/.gitkeep`
- Test: `internal/webui/webui_test.go`

**Interfaces:**
- Consumes: nothing (web builds arrive in Plan 06+ via `web/scripts/copy-dist.mjs`).
- Produces (spec §6.12): `func Handler() http.Handler` over embedded `dist/`; internal `newHandler(fsys fs.FS) http.Handler` for tests. Routes: `/` → 302 `/admin`; `/_app/*` static (immutable cache for `/assets/`, else `no-cache`); `/admin`, `/admin/*` → `admin/index.html`; `/f/*`, `/s/*` → `hosted/index.html`; `/demo`, `/demo/*` → `demo/index.html`; `/embed.js` → `embed/embed.js`; otherwise 404; non-GET/HEAD → 405; missing index → 503 `web UI not built; run \`make web\``.

- [ ] **Step 1: Create the placeholder dist directory**

Run:
```bash
mkdir -p internal/webui/dist && touch internal/webui/dist/.gitkeep
```

- [ ] **Step 2: Write the failing test**

`internal/webui/webui_test.go`:
```go
package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func builtFS() fstest.MapFS {
	return fstest.MapFS{
		"admin/index.html":         {Data: []byte("<html>admin</html>")},
		"admin/assets/app-123.js":  {Data: []byte("console.log('admin')")},
		"admin/favicon.svg":        {Data: []byte("<svg/>")},
		"hosted/index.html":        {Data: []byte("<html>hosted</html>")},
		"demo/index.html":          {Data: []byte("<html>demo</html>")},
		"embed/embed.js":           {Data: []byte("/*embed*/")},
		".gitkeep":                 {Data: []byte{}},
	}
}

func get(h http.Handler, method, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestSPAFallbacks(t *testing.T) {
	h := newHandler(builtFS())
	cases := map[string]string{
		"/admin":                  "admin",
		"/admin/":                 "admin",
		"/admin/submissions/abc":  "admin",
		"/f/contact":              "hosted",
		"/s/1234":                 "hosted",
		"/demo":                   "demo",
		"/demo/anything":          "demo",
	}
	for path, want := range cases {
		rec := get(h, http.MethodGet, path)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: code=%d body=%q, want 200 containing %q", path, rec.Code, rec.Body.String(), want)
		}
		if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s: content-type %q", path, ct)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
			t.Errorf("%s: cache-control %q", path, cc)
		}
	}
}

func TestStaticAssets(t *testing.T) {
	h := newHandler(builtFS())
	rec := get(h, http.MethodGet, "/_app/admin/assets/app-123.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log('admin')" {
		t.Fatalf("asset: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("content-type %q", rec.Header().Get("Content-Type"))
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("assets cache-control %q", cc)
	}
	rec = get(h, http.MethodGet, "/_app/admin/favicon.svg")
	if rec.Code != http.StatusOK || rec.Header().Get("Cache-Control") != "no-cache" {
		t.Errorf("favicon: code=%d cc=%q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

func TestEmbedScript(t *testing.T) {
	rec := get(newHandler(builtFS()), http.MethodGet, "/embed.js")
	if rec.Code != http.StatusOK || rec.Body.String() != "/*embed*/" {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestNotFoundAndTraversal(t *testing.T) {
	h := newHandler(builtFS())
	for _, p := range []string{"/nope", "/_app/admin/missing.js", "/_app/admin/", "/_app/", "/_app/../../etc/passwd"} {
		if rec := get(h, http.MethodGet, p); rec.Code == http.StatusOK {
			t.Errorf("%s: got 200, want non-200", p)
		}
	}
}

func TestRootRedirectsToAdmin(t *testing.T) {
	rec := get(newHandler(builtFS()), http.MethodGet, "/")
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/admin" {
		t.Fatalf("code=%d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestMethodNotAllowed(t *testing.T) {
	if rec := get(newHandler(builtFS()), http.MethodPost, "/admin"); rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("code=%d, want 405", rec.Code)
	}
}

func TestNotBuiltReturns503(t *testing.T) {
	h := newHandler(fstest.MapFS{".gitkeep": {Data: []byte{}}})
	rec := get(h, http.MethodGet, "/admin")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "make web") {
		t.Fatalf("code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestEmbeddedHandlerWorksWithoutBuild(t *testing.T) {
	// The committed dist/ only has .gitkeep: the real Handler must still construct and respond.
	rec := get(Handler(), http.MethodGet, "/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d", rec.Code)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/webui/...`
Expected: FAIL — `undefined: newHandler`, `undefined: Handler`.

- [ ] **Step 4: Write minimal implementation**

`internal/webui/webui.go`:
```go
// Package webui serves the embedded single-page-app builds (admin, hosted, demo, embed.js).
package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// Handler serves the embedded web builds from dist/.
func Handler() http.Handler {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err) // dist/ is embedded at build time; this cannot fail
	}
	return newHandler(sub)
}

func newHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		p := r.URL.Path
		switch {
		case p == "/":
			http.Redirect(w, r, "/admin", http.StatusFound)
		case strings.HasPrefix(p, "/_app/"):
			serveStatic(w, r, fsys, strings.TrimPrefix(p, "/_app/"))
		case p == "/embed.js":
			serveStatic(w, r, fsys, "embed/embed.js")
		case p == "/admin" || strings.HasPrefix(p, "/admin/"):
			serveIndex(w, r, fsys, "admin")
		case strings.HasPrefix(p, "/f/") || strings.HasPrefix(p, "/s/"):
			serveIndex(w, r, fsys, "hosted")
		case p == "/demo" || strings.HasPrefix(p, "/demo/"):
			serveIndex(w, r, fsys, "demo")
		default:
			http.NotFound(w, r)
		}
	})
}

func serveStatic(w http.ResponseWriter, r *http.Request, fsys fs.FS, name string) {
	name = strings.TrimPrefix(path.Clean("/"+name), "/")
	if name == "" {
		http.NotFound(w, r)
		return
	}
	info, err := fs.Stat(fsys, name)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	if strings.Contains(name, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFileFS(w, r, fsys, name)
}

func serveIndex(w http.ResponseWriter, r *http.Request, fsys fs.FS, app string) {
	data, err := fs.ReadFile(fsys, app+"/index.html")
	if err != nil {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		io.WriteString(w, "web UI not built; run `make web`\n") //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if r.Method != http.MethodHead {
		w.Write(data) //nolint:errcheck
	}
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/webui/... -v`
Expected: PASS for all 8 tests.

- [ ] **Step 6: Commit**

```bash
git add internal/webui
git commit -m "feat: serve embedded web builds with SPA fallbacks"
```

---

### Task 5: Problem types + auth principal

**Parallel group:** A (touches only `internal/definition/problem*.go`, `internal/auth/errors.go`, `internal/auth/principal*.go`)

**Files:**
- Create: `internal/definition/problem.go`, `internal/auth/errors.go`, `internal/auth/principal.go`
- Test: `internal/definition/problem_test.go`, `internal/auth/principal_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces:
```go
// package definition (spec §6.4 subset; rest of package is Plan 02)
type Problem struct { Path string `json:"path"`; Message string `json:"message"` }
type ValidationError struct { Problems []Problem }
func (e *ValidationError) Error() string
// package auth (spec §6.5 subset)
const RoleAdmin = "admin"
var ErrInvalidCredentials, ErrUnauthenticated, ErrNotFound, ErrEmailTaken error
type PrincipalKind string; const PrincipalUser, PrincipalAPIKey
type Principal struct { OrgID uuid.UUID; Kind PrincipalKind; ID uuid.UUID; Name, Email string; Roles []string }
func (p Principal) IsAdmin() bool
func (p Principal) HasAnyRole(roles ...string) bool
func WithPrincipal(ctx context.Context, p Principal) context.Context
func PrincipalFrom(ctx context.Context) (Principal, bool)
```

- [ ] **Step 1: Write the failing tests**

`internal/definition/problem_test.go`:
```go
package definition_test

import (
	"errors"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

func TestValidationErrorMessage(t *testing.T) {
	var err error = &definition.ValidationError{Problems: []definition.Problem{
		{Path: "email", Message: "must be a valid email address"},
		{Path: "password", Message: "must be at least 8 characters"},
	}}
	want := "validation failed: email: must be a valid email address; password: must be at least 8 characters"
	if err.Error() != want {
		t.Fatalf("Error() = %q\nwant %q", err.Error(), want)
	}
	var verr *definition.ValidationError
	if !errors.As(err, &verr) || len(verr.Problems) != 2 {
		t.Fatal("errors.As failed")
	}
	if (&definition.ValidationError{}).Error() != "validation failed" {
		t.Fatal("empty problems message")
	}
}
```

`internal/auth/principal_test.go`:
```go
package auth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

func TestHasAnyRole(t *testing.T) {
	reviewer := auth.Principal{Roles: []string{"reviewer"}}
	admin := auth.Principal{Roles: []string{"admin"}}
	none := auth.Principal{}
	cases := []struct {
		name  string
		p     auth.Principal
		roles []string
		want  bool
	}{
		{"empty guard allows anyone", none, nil, true},
		{"matching role", reviewer, []string{"hiring-manager", "reviewer"}, true},
		{"non-matching role", reviewer, []string{"hiring-manager"}, false},
		{"admin satisfies any guard", admin, []string{"hiring-manager"}, true},
		{"no roles vs guard", none, []string{"reviewer"}, false},
	}
	for _, c := range cases {
		if got := c.p.HasAnyRole(c.roles...); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if !admin.IsAdmin() || reviewer.IsAdmin() {
		t.Error("IsAdmin wrong")
	}
}

func TestPrincipalContext(t *testing.T) {
	if _, ok := auth.PrincipalFrom(context.Background()); ok {
		t.Fatal("empty context should have no principal")
	}
	p := auth.Principal{ID: uuid.New(), Kind: auth.PrincipalUser, Name: "Ada"}
	got, ok := auth.PrincipalFrom(auth.WithPrincipal(context.Background(), p))
	if !ok || got.ID != p.ID || got.Name != "Ada" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/definition/... ./internal/auth/...`
Expected: FAIL — `undefined: definition.ValidationError`, `undefined: auth.Principal`.

- [ ] **Step 3: Write minimal implementation**

`internal/definition/problem.go`:
```go
// Package definition holds the form/workflow definition language (Plan 02).
// This file (Plan 01) provides the shared validation problem types.
package definition

import "strings"

// Problem is one validation failure; Path is JSON-pointer-ish, e.g. "fields[2].showIf.field".
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError aggregates problems; the HTTP layer maps it to 422 validation_failed.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Path + ": " + p.Message
	}
	return "validation failed: " + strings.Join(parts, "; ")
}
```

`internal/auth/errors.go`:
```go
package auth

import "errors"

var (
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	ErrUnauthenticated    = errors.New("auth: unauthenticated")
	ErrNotFound           = errors.New("auth: not found")
	ErrEmailTaken         = errors.New("auth: email already taken")
)
```

`internal/auth/principal.go`:
```go
// Package auth owns orgs, users, sessions and API keys.
package auth

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

const RoleAdmin = "admin"

type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"
	PrincipalAPIKey PrincipalKind = "api_key"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	OrgID uuid.UUID
	Kind  PrincipalKind
	ID    uuid.UUID
	Name  string
	Email string
	Roles []string
}

func (p Principal) IsAdmin() bool { return slices.Contains(p.Roles, RoleAdmin) }

// HasAnyRole is true if roles is empty, the principal is admin, or any role matches.
func (p Principal) HasAnyRole(roles ...string) bool {
	if len(roles) == 0 || p.IsAdmin() {
		return true
	}
	for _, r := range roles {
		if slices.Contains(p.Roles, r) {
			return true
		}
	}
	return false
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go get github.com/google/uuid@latest && go test ./internal/definition/... ./internal/auth/... -v`
Expected: PASS — `TestValidationErrorMessage`, `TestHasAnyRole`, `TestPrincipalContext`.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum internal/definition/problem.go internal/definition/problem_test.go internal/auth
git commit -m "feat: validation problem types and auth principal"
```

---

### Task 6: Auth service — org + users

**Files:**
- Create: `internal/auth/service.go`, `internal/auth/validate.go`, `internal/auth/users.go`
- Test: `internal/auth/users_test.go`

**Interfaces:**
- Consumes: `db.WithTx`, `dbtest.New` (Task 3); `definition.Problem/ValidationError`, auth errors/principal (Task 5).
- Produces (spec §6.5):
```go
type User struct { ID, OrgID uuid.UUID; Email, Name string; Roles []string; CreatedAt time.Time }
type UpdateUserInput struct { Name, Password *string; Roles []string /* nil = unchanged */ }
func NewService(pool *pgxpool.Pool) *Service
func (s *Service) EnsureDefaultOrg(ctx) (uuid.UUID, error)
func (s *Service) CreateUser(ctx, orgID uuid.UUID, email, name, password string, roles []string) (User, error)
func (s *Service) ListUsers(ctx, orgID) ([]User, error)
func (s *Service) GetUserByEmail(ctx, orgID, email) (User, error)
func (s *Service) UsersWithRole(ctx, orgID, role) ([]User, error)
func (s *Service) UpdateUser(ctx, orgID, id uuid.UUID, in UpdateUserInput) (User, error)
func (s *Service) DeleteUser(ctx, orgID, id uuid.UUID) error
```
Validation failures return `*definition.ValidationError` with paths `email`, `name`, `password`, `roles[i]`, `roles` (last admin), `id` (delete last admin). Role names must match `^[a-z0-9][a-z0-9_-]{0,31}$`. Changing a password deletes that user's sessions.

- [ ] **Step 1: Add dependency**

Run: `go get golang.org/x/crypto/bcrypt@latest`

- [ ] **Step 2: Write the failing test**

`internal/auth/users_test.go`:
```go
package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/openforms/openforms/internal/db/dbtest"
	"github.com/openforms/openforms/internal/definition"
)

func TestMain(m *testing.M) {
	bcryptCost = bcrypt.MinCost // keep tests fast; production uses 12
	os.Exit(m.Run())
}

func newTestService(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	s := NewService(dbtest.New(t))
	org, err := s.EnsureDefaultOrg(context.Background())
	if err != nil {
		t.Fatalf("EnsureDefaultOrg: %v", err)
	}
	return s, org
}

func problemPaths(t *testing.T, err error) []string {
	t.Helper()
	var verr *definition.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *definition.ValidationError", err)
	}
	var paths []string
	for _, p := range verr.Problems {
		paths = append(paths, p.Path)
	}
	return paths
}

func TestEnsureDefaultOrgIsIdempotent(t *testing.T) {
	s, org := newTestService(t)
	again, err := s.EnsureDefaultOrg(context.Background())
	if err != nil || again != org {
		t.Fatalf("second call = %v, %v; want %v", again, err, org)
	}
}

func TestCreateUserNormalisesAndRejectsDuplicates(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, org, "  Ada@Example.com ", " Ada ", "password123", []string{"reviewer", "admin", "reviewer"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != "ada@example.com" || u.Name != "Ada" || u.OrgID != org {
		t.Fatalf("user = %+v", u)
	}
	if len(u.Roles) != 2 || u.Roles[0] != "reviewer" || u.Roles[1] != "admin" {
		t.Fatalf("roles = %v, want [reviewer admin]", u.Roles)
	}
	if _, err := s.CreateUser(ctx, org, "ADA@example.com", "Other", "password123", nil); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate err = %v, want ErrEmailTaken", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	s, org := newTestService(t)
	_, err := s.CreateUser(context.Background(), org, "not-an-email", " ", "short", []string{"Bad Role"})
	got := problemPaths(t, err)
	want := []string{"email", "name", "password", "roles[0]"}
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
		}
	}
}

func TestListGetAndUsersWithRole(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	a, _ := s.CreateUser(ctx, org, "a@example.com", "A", "password123", []string{"reviewer"})
	s.CreateUser(ctx, org, "b@example.com", "B", "password123", []string{"hiring-manager"}) //nolint:errcheck
	all, err := s.ListUsers(ctx, org)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListUsers = %d, %v", len(all), err)
	}
	got, err := s.GetUserByEmail(ctx, org, "A@EXAMPLE.COM")
	if err != nil || got.ID != a.ID {
		t.Fatalf("GetUserByEmail = %+v, %v", got, err)
	}
	if _, err := s.GetUserByEmail(ctx, org, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user err = %v", err)
	}
	reviewers, err := s.UsersWithRole(ctx, org, "reviewer")
	if err != nil || len(reviewers) != 1 || reviewers[0].ID != a.ID {
		t.Fatalf("UsersWithRole = %+v, %v", reviewers, err)
	}
}

func TestUpdateUser(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "root@example.com", "Root", "password123", []string{"admin"}) //nolint:errcheck
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", []string{"reviewer"})

	name, pw := "Ada L.", "newpassword1"
	got, err := s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Name: &name, Password: &pw, Roles: []string{"hiring-manager"}})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got.Name != "Ada L." || len(got.Roles) != 1 || got.Roles[0] != "hiring-manager" {
		t.Fatalf("updated = %+v", got)
	}
	// nil Roles leaves roles unchanged
	got, err = s.UpdateUser(ctx, org, u.ID, UpdateUserInput{})
	if err != nil || got.Roles[0] != "hiring-manager" || got.Name != "Ada L." {
		t.Fatalf("no-op update = %+v, %v", got, err)
	}
	if _, err := s.UpdateUser(ctx, org, uuid.New(), UpdateUserInput{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user err = %v", err)
	}
	empty := " "
	if paths := problemPaths(t, mustErr(s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Name: &empty}))); paths[0] != "name" {
		t.Fatalf("paths = %v", paths)
	}
}

func mustErr(_ User, err error) error { return err }

func TestLastAdminCannotBeDemotedOrDeleted(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	root, _ := s.CreateUser(ctx, org, "root@example.com", "Root", "password123", []string{"admin"})

	_, err := s.UpdateUser(ctx, org, root.ID, UpdateUserInput{Roles: []string{"reviewer"}})
	if paths := problemPaths(t, err); paths[0] != "roles" {
		t.Fatalf("demote paths = %v", paths)
	}
	if paths := problemPaths(t, s.DeleteUser(ctx, org, root.ID)); paths[0] != "id" {
		t.Fatalf("delete paths = %v", paths)
	}
	still, _ := s.GetUserByEmail(ctx, org, "root@example.com")
	if len(still.Roles) != 1 || still.Roles[0] != "admin" {
		t.Fatalf("root changed: %+v", still)
	}

	// With a second admin both operations succeed.
	s.CreateUser(ctx, org, "second@example.com", "Second", "password123", []string{"admin"}) //nolint:errcheck
	if _, err := s.UpdateUser(ctx, org, root.ID, UpdateUserInput{Roles: []string{"reviewer"}}); err != nil {
		t.Fatalf("demote with another admin: %v", err)
	}
	if err := s.DeleteUser(ctx, org, root.ID); err != nil {
		t.Fatalf("delete non-admin: %v", err)
	}
	if err := s.DeleteUser(ctx, org, root.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/auth/... -count=1`
Expected: FAIL — `undefined: NewService`, `undefined: bcryptCost`, `undefined: Service`.

- [ ] **Step 4: Write `internal/auth/service.go`**

```go
package auth

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/db"
)

// bcryptCost is a variable so tests can lower it.
var bcryptCost = 12

// defaultOrgLock is the advisory lock key serialising default-org creation.
const defaultOrgLock = 7243001

type Service struct {
	pool *pgxpool.Pool
}

func NewService(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// EnsureDefaultOrg returns the single org, creating "Default" on first call.
func (s *Service) EnsureDefaultOrg(ctx context.Context) (uuid.UUID, error) {
	var id uuid.UUID
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, defaultOrgLock); err != nil {
			return err
		}
		err := tx.QueryRow(ctx, `SELECT id FROM orgs ORDER BY created_at, id LIMIT 1`).Scan(&id)
		if errors.Is(err, pgx.ErrNoRows) {
			id = uuid.New()
			_, err = tx.Exec(ctx, `INSERT INTO orgs (id, name) VALUES ($1, 'Default')`, id)
		}
		return err
	})
	return id, err
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
```

- [ ] **Step 5: Write `internal/auth/validate.go`**

```go
package auth

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

var roleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func normalizeEmail(email string) (string, *definition.Problem) {
	e := strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return e, &definition.Problem{Path: "email", Message: "must be a valid email address"}
	}
	return e, nil
}

func nameProblem(name string) *definition.Problem {
	if name == "" {
		return &definition.Problem{Path: "name", Message: "is required"}
	}
	return nil
}

func passwordProblem(pw string) *definition.Problem {
	switch {
	case len(pw) < 8:
		return &definition.Problem{Path: "password", Message: "must be at least 8 characters"}
	case len(pw) > 72:
		return &definition.Problem{Path: "password", Message: "must be at most 72 bytes"}
	}
	return nil
}

// normalizeRoles trims, de-duplicates (keeping first occurrence) and validates role names.
// It always returns a non-nil slice.
func normalizeRoles(roles []string) ([]string, []definition.Problem) {
	out := []string{}
	var probs []definition.Problem
	for i, r := range roles {
		r = strings.TrimSpace(r)
		if !roleRe.MatchString(r) {
			probs = append(probs, definition.Problem{
				Path:    fmt.Sprintf("roles[%d]", i),
				Message: "must be lowercase letters, digits, '-' or '_' (max 32)",
			})
			continue
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out, probs
}

func collect(probs []definition.Problem, more ...*definition.Problem) []definition.Problem {
	for _, p := range more {
		if p != nil {
			probs = append(probs, *p)
		}
	}
	return probs
}
```

- [ ] **Step 6: Write `internal/auth/users.go`**

```go
package auth

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
)

type User struct {
	ID, OrgID uuid.UUID
	Email     string
	Name      string
	Roles     []string
	CreatedAt time.Time
}

type UpdateUserInput struct {
	Name     *string
	Password *string
	Roles    []string // nil = unchanged
}

const userColumns = "id, org_id, email, name, roles, created_at"

func scanUser(row pgx.Row) (User, error) {
	var u User
	err := row.Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Roles, &u.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if u.Roles == nil {
		u.Roles = []string{}
	}
	return u, err
}

func collectUsers(rows pgx.Rows, err error) ([]User, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Service) CreateUser(ctx context.Context, orgID uuid.UUID, email, name, password string, roles []string) (User, error) {
	e, emailProb := normalizeEmail(email)
	name = strings.TrimSpace(name)
	probs := collect(nil, emailProb, nameProblem(name), passwordProblem(password))
	rs, roleProbs := normalizeRoles(roles)
	probs = append(probs, roleProbs...)
	if len(probs) > 0 {
		return User{}, &definition.ValidationError{Problems: probs}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return User{}, err
	}
	u, err := scanUser(s.pool.QueryRow(ctx,
		`INSERT INTO users (id, org_id, email, name, password_hash, roles)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+userColumns,
		uuid.New(), orgID, e, name, string(hash), rs))
	if isUniqueViolation(err) {
		return User{}, ErrEmailTaken
	}
	return u, err
}

func (s *Service) ListUsers(ctx context.Context, orgID uuid.UUID) ([]User, error) {
	return collectUsers(s.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 ORDER BY created_at, email`, orgID))
}

func (s *Service) GetUserByEmail(ctx context.Context, orgID uuid.UUID, email string) (User, error) {
	return scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 AND lower(email) = lower($2)`,
		orgID, strings.TrimSpace(email)))
}

func (s *Service) UsersWithRole(ctx context.Context, orgID uuid.UUID, role string) ([]User, error) {
	return collectUsers(s.pool.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE org_id = $1 AND $2 = ANY(roles) ORDER BY created_at, id`,
		orgID, role))
}

// lockAdmins locks every admin row (ordered by id, so concurrent callers never deadlock).
func lockAdmins(ctx context.Context, tx pgx.Tx, orgID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM users WHERE org_id = $1 AND $2 = ANY(roles) ORDER BY id FOR UPDATE`, orgID, RoleAdmin)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func lastAdminError(admins []uuid.UUID, id uuid.UUID, path string) error {
	for _, a := range admins {
		if a != id {
			return nil
		}
	}
	return &definition.ValidationError{Problems: []definition.Problem{{Path: path, Message: "at least one admin must remain"}}}
}

func (s *Service) UpdateUser(ctx context.Context, orgID, id uuid.UUID, in UpdateUserInput) (User, error) {
	var probs []definition.Problem
	var name *string
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		probs = collect(probs, nameProblem(n))
		name = &n
	}
	if in.Password != nil {
		probs = collect(probs, passwordProblem(*in.Password))
	}
	var roles []string
	if in.Roles != nil {
		rs, rp := normalizeRoles(in.Roles)
		roles, probs = rs, append(probs, rp...)
	}
	if len(probs) > 0 {
		return User{}, &definition.ValidationError{Problems: probs}
	}
	var hash *string
	if in.Password != nil {
		h, err := bcrypt.GenerateFromPassword([]byte(*in.Password), bcryptCost)
		if err != nil {
			return User{}, err
		}
		hs := string(h)
		hash = &hs
	}

	var out User
	err := db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		var admins []uuid.UUID
		if roles != nil {
			var err error
			if admins, err = lockAdmins(ctx, tx, orgID); err != nil {
				return err
			}
		}
		cur, err := scanUser(tx.QueryRow(ctx,
			`SELECT `+userColumns+` FROM users WHERE id = $1 AND org_id = $2 FOR UPDATE`, id, orgID))
		if err != nil {
			return err
		}
		if roles == nil {
			roles = cur.Roles
		} else if slices.Contains(cur.Roles, RoleAdmin) && !slices.Contains(roles, RoleAdmin) {
			if err := lastAdminError(admins, id, "roles"); err != nil {
				return err
			}
		}
		if name == nil {
			name = &cur.Name
		}
		out, err = scanUser(tx.QueryRow(ctx,
			`UPDATE users SET name = $3, roles = $4, password_hash = COALESCE($5, password_hash)
			 WHERE id = $1 AND org_id = $2 RETURNING `+userColumns,
			id, orgID, *name, roles, hash))
		if err != nil {
			return err
		}
		if hash != nil {
			_, err = tx.Exec(ctx, `DELETE FROM sessions WHERE user_id = $1`, id)
		}
		return err
	})
	return out, err
}

func (s *Service) DeleteUser(ctx context.Context, orgID, id uuid.UUID) error {
	return db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		admins, err := lockAdmins(ctx, tx, orgID)
		if err != nil {
			return err
		}
		cur, err := scanUser(tx.QueryRow(ctx,
			`SELECT `+userColumns+` FROM users WHERE id = $1 AND org_id = $2 FOR UPDATE`, id, orgID))
		if err != nil {
			return err
		}
		if slices.Contains(cur.Roles, RoleAdmin) {
			if err := lastAdminError(admins, id, "id"); err != nil {
				return err
			}
		}
		_, err = tx.Exec(ctx, `DELETE FROM users WHERE id = $1 AND org_id = $2`, id, orgID)
		return err
	})
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go mod tidy && go test ./internal/auth/... -count=1 -v`
Expected: PASS — `TestEnsureDefaultOrgIsIdempotent`, `TestCreateUserNormalisesAndRejectsDuplicates`, `TestCreateUserValidation`, `TestListGetAndUsersWithRole`, `TestUpdateUser`, `TestLastAdminCannotBeDemotedOrDeleted`, plus Task 5 tests.

- [ ] **Step 8: Commit**

```bash
git add go.mod go.sum internal/auth
git commit -m "feat: default org and user management with last-admin guard"
```

---

### Task 7: Auth service — sessions + API keys

**Files:**
- Create: `internal/auth/tokens.go`, `internal/auth/sessions.go`, `internal/auth/apikeys.go`
- Test: `internal/auth/sessions_test.go`, `internal/auth/apikeys_test.go`

**Interfaces:**
- Consumes: Task 6 `Service`, `User`, `scanUser`, `normalizeRoles`, `collect`, `bcryptCost`.
- Produces (spec §6.5):
```go
const SessionTTL = 30 * 24 * time.Hour
type APIKey struct { ID, OrgID uuid.UUID; Name, Prefix string; Roles []string; CreatedAt time.Time; LastUsedAt, RevokedAt *time.Time }
func (s *Service) Login(ctx, email, password string) (token string, u User, err error)
func (s *Service) Logout(ctx, token string) error
func (s *Service) PrincipalFromSession(ctx, token string) (Principal, error)
func (s *Service) CreateAPIKey(ctx, orgID uuid.UUID, name string, roles []string) (plaintext string, k APIKey, err error)
func (s *Service) ListAPIKeys(ctx, orgID) ([]APIKey, error)
func (s *Service) RevokeAPIKey(ctx, orgID, id uuid.UUID) error
func (s *Service) PrincipalFromAPIKey(ctx, plaintext string) (Principal, error)
```

- [ ] **Step 1: Write the failing tests**

`internal/auth/sessions_test.go`:
```go
package auth

import (
	"context"
	"errors"
	"testing"
)

func TestLoginAndSessionLifecycle(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", []string{"reviewer"})

	token, got, err := s.Login(ctx, "ADA@example.com ", "password123")
	if err != nil || got.ID != u.ID || token == "" {
		t.Fatalf("Login = %q, %+v, %v", token, got, err)
	}
	var stored string
	s.pool.QueryRow(ctx, `SELECT token_hash FROM sessions`).Scan(&stored) //nolint:errcheck
	if stored == token || stored != hashToken(token) {
		t.Fatal("session must store sha256 of token, not the token")
	}

	p, err := s.PrincipalFromSession(ctx, token)
	if err != nil {
		t.Fatalf("PrincipalFromSession: %v", err)
	}
	if p.Kind != PrincipalUser || p.ID != u.ID || p.OrgID != org || p.Email != "ada@example.com" || p.Roles[0] != "reviewer" {
		t.Fatalf("principal = %+v", p)
	}

	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("after logout err = %v", err)
	}
	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("second Logout should be a no-op, got %v", err)
	}
}

func TestLoginFailures(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil) //nolint:errcheck
	if _, _, err := s.Login(ctx, "ada@example.com", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, _, err := s.Login(ctx, "nobody@example.com", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user err = %v", err)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil) //nolint:errcheck
	token, _, _ := s.Login(ctx, "ada@example.com", "password123")
	if _, err := s.pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session err = %v", err)
	}
	if _, err := s.PrincipalFromSession(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty token err = %v", err)
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil)
	token, _, _ := s.Login(ctx, "ada@example.com", "password123")
	pw := "brand-new-pass"
	if _, err := s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Password: &pw}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old session err = %v", err)
	}
	if _, _, err := s.Login(ctx, "ada@example.com", pw); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}
```

`internal/auth/apikeys_test.go`:
```go
package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
)

func TestAPIKeyLifecycle(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()

	plaintext, k, err := s.CreateAPIKey(ctx, org, " CI deploy ", []string{"admin"})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !regexp.MustCompile(`^ofk_[0-9A-Za-z]{40}$`).MatchString(plaintext) {
		t.Fatalf("plaintext %q has wrong format", plaintext)
	}
	if k.Prefix != plaintext[:8] || k.Name != "CI deploy" || k.OrgID != org || k.RevokedAt != nil {
		t.Fatalf("key = %+v", k)
	}

	p, err := s.PrincipalFromAPIKey(ctx, plaintext)
	if err != nil {
		t.Fatalf("PrincipalFromAPIKey: %v", err)
	}
	if p.Kind != PrincipalAPIKey || p.ID != k.ID || p.Name != "CI deploy" || !p.IsAdmin() || p.OrgID != org {
		t.Fatalf("principal = %+v", p)
	}

	keys, err := s.ListAPIKeys(ctx, org)
	if err != nil || len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatalf("ListAPIKeys = %+v, %v (LastUsedAt must be set after use)", keys, err)
	}

	if err := s.RevokeAPIKey(ctx, org, k.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := s.PrincipalFromAPIKey(ctx, plaintext); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked key err = %v", err)
	}
	if err := s.RevokeAPIKey(ctx, org, k.ID); err != nil {
		t.Fatalf("second revoke should be idempotent, got %v", err)
	}
	if err := s.RevokeAPIKey(ctx, org, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown key err = %v", err)
	}
}

func TestAPIKeyRejectsGarbage(t *testing.T) {
	s, _ := newTestService(t)
	for _, k := range []string{"", "garbage", "ofk_doesnotexist"} {
		if _, err := s.PrincipalFromAPIKey(context.Background(), k); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%q: err = %v", k, err)
		}
	}
}

func TestCreateAPIKeyValidation(t *testing.T) {
	s, org := newTestService(t)
	_, _, err := s.CreateAPIKey(context.Background(), org, "  ", []string{"NOPE"})
	paths := problemPaths(t, err)
	if len(paths) != 2 || paths[0] != "name" || paths[1] != "roles[0]" {
		t.Fatalf("paths = %v", paths)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/auth/... -count=1`
Expected: FAIL — `undefined: hashToken`, `s.Login undefined`, `s.CreateAPIKey undefined`.

- [ ] **Step 3: Write `internal/auth/tokens.go`**

```go
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"math/big"
)

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

func randomToken(nBytes int) (string, error) {
	b := make([]byte, nBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomBase62(n int) (string, error) {
	out := make([]byte, n)
	max := big.NewInt(int64(len(base62)))
	for i := range out {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = base62[v.Int64()]
	}
	return string(out), nil
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Write `internal/auth/sessions.go`**

```go
package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"golang.org/x/crypto/bcrypt"
)

const SessionTTL = 30 * 24 * time.Hour

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// compareDummy spends the same time as a real password check so unknown emails
// are not distinguishable by timing.
func compareDummy(password string) {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("openforms-timing-equaliser"), bcryptCost)
	})
	bcrypt.CompareHashAndPassword(dummyHash, []byte(password)) //nolint:errcheck
}

func (s *Service) Login(ctx context.Context, email, password string) (string, User, error) {
	e := strings.ToLower(strings.TrimSpace(email))
	var u User
	var hash string
	err := s.pool.QueryRow(ctx,
		`SELECT `+userColumns+`, password_hash FROM users WHERE lower(email) = $1 ORDER BY created_at LIMIT 1`, e).
		Scan(&u.ID, &u.OrgID, &u.Email, &u.Name, &u.Roles, &u.CreatedAt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		compareDummy(password)
		return "", User{}, ErrInvalidCredentials
	}
	if err != nil {
		return "", User{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return "", User{}, ErrInvalidCredentials
	}
	token, err := randomToken(32)
	if err != nil {
		return "", User{}, err
	}
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`); err != nil {
		return "", User{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`INSERT INTO sessions (token_hash, user_id, expires_at) VALUES ($1, $2, $3)`,
		hashToken(token), u.ID, time.Now().Add(SessionTTL)); err != nil {
		return "", User{}, err
	}
	if u.Roles == nil {
		u.Roles = []string{}
	}
	return token, u, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash = $1`, hashToken(token))
	return err
}

func (s *Service) PrincipalFromSession(ctx context.Context, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{Kind: PrincipalUser}
	err := s.pool.QueryRow(ctx,
		`SELECT u.id, u.org_id, u.email, u.name, u.roles
		 FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = $1 AND s.expires_at > now()`, hashToken(token)).
		Scan(&p.ID, &p.OrgID, &p.Email, &p.Name, &p.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	return p, err
}
```

- [ ] **Step 5: Write `internal/auth/apikeys.go`**

```go
package auth

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

const apiKeyPrefix = "ofk_"

type APIKey struct {
	ID, OrgID  uuid.UUID
	Name       string
	Prefix     string
	Roles      []string
	CreatedAt  time.Time
	LastUsedAt *time.Time
	RevokedAt  *time.Time
}

const apiKeyColumns = "id, org_id, name, prefix, roles, created_at, last_used_at, revoked_at"

func scanAPIKey(row pgx.Row) (APIKey, error) {
	var k APIKey
	err := row.Scan(&k.ID, &k.OrgID, &k.Name, &k.Prefix, &k.Roles, &k.CreatedAt, &k.LastUsedAt, &k.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return APIKey{}, ErrNotFound
	}
	if k.Roles == nil {
		k.Roles = []string{}
	}
	return k, err
}

func (s *Service) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name string, roles []string) (string, APIKey, error) {
	name = strings.TrimSpace(name)
	var probs []definition.Problem
	if name == "" || len(name) > 100 {
		probs = append(probs, definition.Problem{Path: "name", Message: "is required (max 100 characters)"})
	}
	rs, rp := normalizeRoles(roles)
	probs = append(probs, rp...)
	if len(probs) > 0 {
		return "", APIKey{}, &definition.ValidationError{Problems: probs}
	}
	secret, err := randomBase62(40)
	if err != nil {
		return "", APIKey{}, err
	}
	plaintext := apiKeyPrefix + secret
	k, err := scanAPIKey(s.pool.QueryRow(ctx,
		`INSERT INTO api_keys (id, org_id, name, prefix, key_hash, roles)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+apiKeyColumns,
		uuid.New(), orgID, name, plaintext[:8], hashToken(plaintext), rs))
	if err != nil {
		return "", APIKey{}, err
	}
	return plaintext, k, nil
}

func (s *Service) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]APIKey, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE org_id = $1 ORDER BY created_at DESC, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}

// RevokeAPIKey is idempotent; it returns ErrNotFound only for unknown ids.
func (s *Service) RevokeAPIKey(ctx context.Context, orgID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET revoked_at = COALESCE(revoked_at, now()) WHERE id = $1 AND org_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) PrincipalFromAPIKey(ctx context.Context, plaintext string) (Principal, error) {
	if !strings.HasPrefix(plaintext, apiKeyPrefix) {
		return Principal{}, ErrUnauthenticated
	}
	p := Principal{Kind: PrincipalAPIKey}
	err := s.pool.QueryRow(ctx,
		`SELECT id, org_id, name, roles FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL`,
		hashToken(plaintext)).Scan(&p.ID, &p.OrgID, &p.Name, &p.Roles)
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, ErrUnauthenticated
	}
	if err != nil {
		return Principal{}, err
	}
	// Throttled so busy keys do not write on every request.
	if _, err := s.pool.Exec(ctx,
		`UPDATE api_keys SET last_used_at = now()
		 WHERE id = $1 AND (last_used_at IS NULL OR last_used_at < now() - interval '1 minute')`, p.ID); err != nil {
		return Principal{}, err
	}
	return p, nil
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/auth/... -count=1 -v`
Expected: PASS — all Task 5/6 tests plus `TestLoginAndSessionLifecycle`, `TestLoginFailures`, `TestExpiredSessionIsRejected`, `TestPasswordChangeRevokesSessions`, `TestAPIKeyLifecycle`, `TestAPIKeyRejectsGarbage`, `TestCreateAPIKeyValidation`.

- [ ] **Step 7: Commit**

```bash
git add internal/auth
git commit -m "feat: sessions and API keys"
```

---

### Task 8: HTTP core — errors, JSON, middleware, router, healthz, testutil

**Files:**
- Create: `internal/httpapi/errors.go`, `internal/httpapi/middleware.go`, `internal/httpapi/routes.go`, `internal/testutil/testutil.go`
- Test: `internal/httpapi/errors_test.go`, `internal/httpapi/middleware_test.go`, `internal/httpapi/routes_test.go`

**Interfaces:**
- Consumes: `auth.*` (Tasks 5–7), `config.Config` (Task 2), `dbtest.New` (Task 3), `definition.Problem/ValidationError` (Task 5).
- Produces (spec §6.11, §6.15):
```go
// package httpapi
type Deps struct { Config config.Config; Pool *pgxpool.Pool; Auth *auth.Service; Web http.Handler; OrgID uuid.UUID }
type APIError struct { Status int; Code, Message string; Details []definition.Problem }
func (e *APIError) Error() string
func NewRouter(d Deps) http.Handler
func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, r *http.Request, err error)
func DecodeJSON(r *http.Request, v any) error
func Authenticate(a *auth.Service) func(http.Handler) http.Handler
func RequireAuth(next http.Handler) http.Handler
func RequireAdmin(next http.Handler) http.Handler
func MustPrincipal(r *http.Request) auth.Principal
func URLParamUUID(r *http.Request, name string) (uuid.UUID, error)
const sessionCookie = "of_session" // unexported, used by auth handlers
// package testutil
type Env struct { Pool *pgxpool.Pool; Auth *auth.Service; OrgID uuid.UUID; Config config.Config }
func NewEnv(t testing.TB) *Env
func (e *Env) APIKey(t testing.TB, roles ...string) string
func (e *Env) User(t testing.TB, email string, roles ...string) auth.User // password "password123"
func Do(t testing.TB, h http.Handler, method, path, apiKey string, body any) *httptest.ResponseRecorder
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T
func ErrorCode(t testing.TB, rec *httptest.ResponseRecorder) string
```
`routes.go` in this task defines `NewRouter` with **no mount calls yet**; Tasks 9/10 add `mountAuth`, then the `RequireAuth` group with `mountAdminUsers`, `mountAPIKeys`.

- [ ] **Step 1: Add dependency**

Run: `go get github.com/go-chi/chi/v5@latest`

- [ ] **Step 2: Write the failing tests**

`internal/httpapi/errors_test.go`:
```go
package httpapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/httpapi"
)

type envelope struct {
	Error struct {
		Code    string               `json:"code"`
		Message string               `json:"message"`
		Details []definition.Problem `json:"details"`
	} `json:"error"`
}

func writeErr(err error) (*httptest.ResponseRecorder, envelope) {
	rec := httptest.NewRecorder()
	httpapi.WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), err)
	var env envelope
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return rec, env
}

func TestWriteErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"api error", &httpapi.APIError{Status: 418, Code: "teapot", Message: "short and stout"}, 418, "teapot"},
		{"validation", &definition.ValidationError{Problems: []definition.Problem{{Path: "email", Message: "bad"}}}, 422, "validation_failed"},
		{"wrapped unauthenticated", fmt.Errorf("ctx: %w", auth.ErrUnauthenticated), 401, "unauthenticated"},
		{"invalid credentials", auth.ErrInvalidCredentials, 401, "invalid_credentials"},
		{"email taken", auth.ErrEmailTaken, 409, "email_taken"},
		{"not found", auth.ErrNotFound, 404, "not_found"},
		{"unknown", errors.New("database exploded"), 500, "internal"},
	}
	for _, c := range cases {
		rec, env := writeErr(c.err)
		if rec.Code != c.status || env.Error.Code != c.code {
			t.Errorf("%s: got %d/%q want %d/%q", c.name, rec.Code, env.Error.Code, c.status, c.code)
		}
		if env.Error.Message == "" {
			t.Errorf("%s: empty message", c.name)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", c.name, ct)
		}
	}
	_, env := writeErr(&definition.ValidationError{Problems: []definition.Problem{{Path: "email", Message: "bad"}}})
	if len(env.Error.Details) != 1 || env.Error.Details[0].Path != "email" {
		t.Errorf("details = %+v", env.Error.Details)
	}
	rec, env := writeErr(errors.New("database exploded"))
	if strings.Contains(rec.Body.String(), "exploded") || env.Error.Details != nil {
		t.Errorf("internal errors must not leak: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"details"`) {
		t.Errorf("details must be omitted when empty: %s", rec.Body.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	var v struct {
		Name string `json:"name"`
	}
	ok := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada","extra":1}`))
	if err := httpapi.DecodeJSON(ok, &v); err != nil || v.Name != "ada" {
		t.Fatalf("valid body: %v %+v", err, v)
	}
	cases := map[string]struct {
		body   string
		status int
	}{
		"malformed": {`{"name":`, 400},
		"empty":     {``, 400},
		"too large": {`{"name":"` + strings.Repeat("a", 1<<20) + `"}`, 413},
	}
	for name, c := range cases {
		err := httpapi.DecodeJSON(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)), &v)
		var apiErr *httpapi.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != c.status || apiErr.Code != "bad_request" {
			t.Errorf("%s: err = %#v, want %d bad_request", name, err, c.status)
		}
	}
}
```

`internal/httpapi/middleware_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/httpapi"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"name": httpapi.MustPrincipal(r).Name})
})

func serveWith(mw func(http.Handler) http.Handler, p *auth.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if p != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), *p))
	}
	rec := httptest.NewRecorder()
	mw(okHandler).ServeHTTP(rec, req)
	return rec
}

func TestRequireAuth(t *testing.T) {
	rec, env := func() (*httptest.ResponseRecorder, envelope) {
		r := serveWith(httpapi.RequireAuth, nil)
		return r, decodeEnvelope(r)
	}()
	if rec.Code != 401 || env.Error.Code != "unauthenticated" {
		t.Fatalf("anonymous: %d %q", rec.Code, env.Error.Code)
	}
	if rec := serveWith(httpapi.RequireAuth, &auth.Principal{Name: "Ada"}); rec.Code != 200 {
		t.Fatalf("authenticated: %d", rec.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	if rec := serveWith(httpapi.RequireAdmin, nil); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	rec := serveWith(httpapi.RequireAdmin, &auth.Principal{Roles: []string{"reviewer"}})
	if rec.Code != 403 || decodeEnvelope(rec).Error.Code != "forbidden" {
		t.Fatalf("reviewer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serveWith(httpapi.RequireAdmin, &auth.Principal{Roles: []string{"admin"}}); rec.Code != 200 {
		t.Fatalf("admin: %d", rec.Code)
	}
}

func TestURLParamUUID(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/things/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, err := httpapi.URLParamUUID(req, "id")
		if err != nil {
			httpapi.WriteError(w, req, err)
			return
		}
		httpapi.WriteJSON(w, 200, map[string]string{"id": id.String()})
	})
	good := uuid.New()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things/"+good.String(), nil))
	if rec.Code != 200 {
		t.Fatalf("valid uuid: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things/not-a-uuid", nil))
	if rec.Code != 404 || decodeEnvelope(rec).Error.Code != "not_found" {
		t.Fatalf("invalid uuid: %d %s", rec.Code, rec.Body.String())
	}
}
```

`internal/httpapi/routes_test.go`:
```go
package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/testutil"
)

func decodeEnvelope(rec *httptest.ResponseRecorder) envelope {
	var env envelope
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return env
}

func newRouter(t *testing.T) (*testutil.Env, http.Handler) {
	t.Helper()
	env := testutil.NewEnv(t)
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(299) // sentinel: request reached the web UI handler
	})
	return env, httpapi.NewRouter(httpapi.Deps{
		Config: env.Config, Pool: env.Pool, Auth: env.Auth, Web: web, OrgID: env.OrgID,
	})
}

func TestHealthz(t *testing.T) {
	_, h := newRouter(t)
	rec := testutil.Do(t, h, http.MethodGet, "/healthz", "", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if got := testutil.Decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Fatalf("body = %v", got)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	_, h := newRouter(t)
	rec := testutil.Do(t, h, http.MethodGet, "/api/v1/does-not-exist", "", nil)
	if rec.Code != 404 || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNonAPIPathsReachWebHandler(t *testing.T) {
	_, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodGet, "/admin/submissions", "", nil); rec.Code != 299 {
		t.Fatalf("code = %d, want web handler sentinel 299", rec.Code)
	}
}

func TestInvalidBearerDoesNotBreakPublicRoutes(t *testing.T) {
	_, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodGet, "/healthz", "ofk_bogus", nil); rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/httpapi/... -count=1`
Expected: FAIL — `undefined: httpapi.WriteError`, `no required module provides package .../internal/testutil`.

- [ ] **Step 4: Write `internal/httpapi/errors.go`**

```go
// Package httpapi is the /api/v1 HTTP layer: router, middleware, handlers and the error envelope.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
)

// APIError is an error with an explicit HTTP status and envelope code.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details []definition.Problem
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// errorMapping maps a sentinel error (matched with errors.Is) to a response.
// Later plans register their packages' sentinel errors by appending rows here
// (e.g. {definitions.ErrNotFound, 404, "not_found", "not found"}).
type errorMapping struct {
	target  error
	status  int
	code    string
	message string
}

var errorMappings = []errorMapping{
	{auth.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "authentication required"},
	{auth.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials", "invalid email or password"},
	{auth.ErrEmailTaken, http.StatusConflict, "email_taken", "a user with this email already exists"},
	{auth.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
}

func notFound(msg string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}

func forbidden(msg string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func badRequest(status int, msg string) *APIError {
	return &APIError{Status: status, Code: "bad_request", Message: msg}
}

type errorBody struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Details []definition.Problem `json:"details,omitempty"`
}

// WriteError renders err as the standard error envelope.
// Order: *APIError, *definition.ValidationError, errorMappings, then 500 internal.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *APIError
	var verr *definition.ValidationError
	switch {
	case errors.As(err, &apiErr):
		writeEnvelope(w, apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	case errors.As(err, &verr):
		writeEnvelope(w, http.StatusUnprocessableEntity, "validation_failed", "validation failed", verr.Problems)
		return
	}
	for _, m := range errorMappings {
		if errors.Is(err, m.target) {
			writeEnvelope(w, m.status, m.code, m.message, nil)
			return
		}
	}
	slog.Error("internal error", "err", err, "method", r.Method, "path", r.URL.Path)
	writeEnvelope(w, http.StatusInternalServerError, "internal", "internal server error", nil)
}

func writeEnvelope(w http.ResponseWriter, status int, code, msg string, details []definition.Problem) {
	WriteJSON(w, status, map[string]errorBody{"error": {Code: code, Message: msg, Details: details}})
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}

const maxBodyBytes = 1 << 20

// DecodeJSON reads at most 1 MiB and unmarshals it into v. Unknown fields are ignored.
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return badRequest(http.StatusBadRequest, "request body is required")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return badRequest(http.StatusBadRequest, "could not read request body")
	}
	if len(data) > maxBodyBytes {
		return badRequest(http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return badRequest(http.StatusBadRequest, "request body is required")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return badRequest(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	return nil
}
```

- [ ] **Step 5: Write `internal/httpapi/middleware.go`**

```go
package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

const sessionCookie = "of_session"

// Authenticate resolves a Bearer API key or the of_session cookie into a principal.
// Invalid credentials leave the request anonymous (RequireAuth rejects later);
// infrastructure errors (e.g. DB down) are returned as 500.
func Authenticate(a *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := principalFromRequest(r, a)
			switch {
			case err == nil:
				r = r.WithContext(auth.WithPrincipal(r.Context(), p))
			case errors.Is(err, auth.ErrUnauthenticated):
			default:
				WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func principalFromRequest(r *http.Request, a *auth.Service) (auth.Principal, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return a.PrincipalFromAPIKey(r.Context(), strings.TrimSpace(tok))
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return a.PrincipalFromSession(r.Context(), c.Value)
	}
	return auth.Principal{}, auth.ErrUnauthenticated
}

// RequireAuth rejects requests without a principal with 401 unauthenticated.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.PrincipalFrom(r.Context()); !ok {
			WriteError(w, r, auth.ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects anonymous requests with 401 and non-admins with 403.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, auth.ErrUnauthenticated)
			return
		}
		if !p.IsAdmin() {
			WriteError(w, r, forbidden("admin role required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MustPrincipal returns the request principal; only call it behind RequireAuth.
func MustPrincipal(r *http.Request) auth.Principal {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		panic("httpapi: MustPrincipal called without RequireAuth")
	}
	return p
}

// URLParamUUID parses a chi URL parameter; malformed values are 404 not_found.
func URLParamUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, notFound("not found")
	}
	return id, nil
}
```

- [ ] **Step 6: Write `internal/httpapi/routes.go`**

```go
package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
)

// Deps are the services handlers use. Later plans add Defs, Subs, Engine and Queue (spec §6.11).
type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Auth   *auth.Service
	Web    http.Handler // webui handler; may be nil
	OrgID  uuid.UUID
}

// NewRouter builds the full HTTP handler: /healthz, /api/v1/*, and the web UI fallback.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	if d.Web != nil {
		r.NotFound(d.Web.ServeHTTP)
		r.MethodNotAllowed(d.Web.ServeHTTP)
	}
	r.Get("/healthz", healthz(d))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(Authenticate(d.Auth))
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, req, notFound("no such endpoint"))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, req, &APIError{Status: http.StatusMethodNotAllowed, Code: "bad_request", Message: "method not allowed"})
		})
		// Plan 01 Tasks 9–10 add: mountAuth(r, d); r.Group(RequireAuth: mountAdminUsers, mountAPIKeys).
	})
	return r
}

func healthz(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := d.Pool.Ping(ctx); err != nil {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}
```

- [ ] **Step 7: Write `internal/testutil/testutil.go`**

```go
// Package testutil provides DB-backed fixtures and HTTP helpers for handler tests.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db/dbtest"
)

type Env struct {
	Pool   *pgxpool.Pool
	Auth   *auth.Service
	OrgID  uuid.UUID
	Config config.Config
}

// NewEnv returns an isolated, migrated schema with the default org created.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	pool := dbtest.New(t)
	a := auth.NewService(pool)
	org, err := a.EnsureDefaultOrg(context.Background())
	if err != nil {
		t.Fatalf("ensure default org: %v", err)
	}
	return &Env{
		Pool:  pool,
		Auth:  a,
		OrgID: org,
		Config: config.Config{
			HTTPAddr:          "127.0.0.1:0",
			BaseURL:           "http://test.local",
			SMTPPort:          1025,
			SMTPFrom:          "openforms@test.local",
			WorkerConcurrency: 1,
		},
	}
}

var keySeq atomic.Int64

// APIKey creates an API key with the given roles and returns its plaintext.
func (e *Env) APIKey(t testing.TB, roles ...string) string {
	t.Helper()
	plaintext, _, err := e.Auth.CreateAPIKey(context.Background(), e.OrgID, fmt.Sprintf("test-key-%d", keySeq.Add(1)), roles)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return plaintext
}

// User creates a user with password "password123".
func (e *Env) User(t testing.TB, email string, roles ...string) auth.User {
	t.Helper()
	u, err := e.Auth.CreateUser(context.Background(), e.OrgID, email, email, "password123", roles)
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

// Do sends a request. body may be nil, a string/[]byte (sent raw) or any value (JSON-encoded).
// apiKey "" sends the request anonymously.
func Do(t testing.TB, h http.Handler, method, path, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = strings.NewReader(b)
	case []byte:
		rdr = bytes.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Decode unmarshals the recorder body into T or fails the test.
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

// ErrorCode returns error.code from the envelope ("" if absent).
func ErrorCode(t testing.TB, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return env.Error.Code
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go mod tidy && go test ./internal/httpapi/... -count=1 -v`
Expected: PASS — `TestWriteErrorMapping`, `TestDecodeJSON`, `TestRequireAuth`, `TestRequireAdmin`, `TestURLParamUUID`, `TestHealthz`, `TestUnknownAPIPathIsJSON404`, `TestNonAPIPathsReachWebHandler`, `TestInvalidBearerDoesNotBreakPublicRoutes`.

- [ ] **Step 9: Commit**

```bash
git add go.mod go.sum internal/httpapi internal/testutil
git commit -m "feat: http error envelope, auth middleware, router and test helpers"
```

---

### Task 9: Auth endpoints (login / logout / me)

**Parallel group:** B lane 1 (touches `internal/httpapi/json.go`, `auth_handlers*.go`, `routes.go`). Runs concurrently with Task 11.

**Files:**
- Create: `internal/httpapi/json.go`, `internal/httpapi/auth_handlers.go`
- Modify: `internal/httpapi/routes.go` (replace the comment inside `r.Route("/api/v1", ...)`)
- Test: `internal/httpapi/auth_handlers_test.go`

**Interfaces:**
- Consumes: Task 8 helpers; `auth.Service.Login/Logout`, `auth.SessionTTL`.
- Produces: `POST /api/v1/auth/login` → 200 `{"user": User}` + `of_session` cookie; `POST /api/v1/auth/logout` (auth) → 204 and expired cookie; `GET /api/v1/auth/me` (auth) → `{"principal": {kind,id,name,email,roles}}`. JSON helpers `userJSON`, `toUserJSON(auth.User)`, `principalJSON`, `toPrincipalJSON(auth.Principal)`, `nonNil([]string) []string` for later handler files. Function `mountAuth(r chi.Router, d Deps)`.

- [ ] **Step 1: Write the failing test**

`internal/httpapi/auth_handlers_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type principalResp struct {
	Principal struct {
		Kind  string   `json:"kind"`
		ID    string   `json:"id"`
		Name  string   `json:"name"`
		Email string   `json:"email"`
		Roles []string `json:"roles"`
	} `json:"principal"`
}

func withCookie(req *http.Request, c *http.Cookie) *http.Request {
	req.AddCookie(c)
	return req
}

func login(t *testing.T, h http.Handler, email, password string) *httptest.ResponseRecorder {
	t.Helper()
	return testutil.Do(t, h, http.MethodPost, "/api/v1/auth/login", "", map[string]string{"email": email, "password": password})
}

func sessionFrom(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == "of_session" {
			return c
		}
	}
	t.Fatalf("no of_session cookie in %v", rec.Result().Cookies())
	return nil
}

func TestLoginMeLogout(t *testing.T) {
	env, h := newRouter(t)
	env.User(t, "ada@example.com", "reviewer")

	rec := login(t, h, "ADA@example.com", "password123")
	if rec.Code != 200 {
		t.Fatalf("login: %d %s", rec.Code, rec.Body.String())
	}
	body := testutil.Decode[map[string]map[string]any](t, rec)
	if body["user"]["email"] != "ada@example.com" || body["user"]["createdAt"] == nil {
		t.Fatalf("login body = %v", body)
	}
	if strings.Contains(rec.Body.String(), "password") {
		t.Fatal("login response must not include password material")
	}
	c := sessionFrom(t, rec)
	if !c.HttpOnly || c.Path != "/" || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 30*24*3600 {
		t.Fatalf("cookie attributes = %+v", c)
	}

	me := httptest.NewRecorder()
	h.ServeHTTP(me, withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), c))
	if me.Code != 200 {
		t.Fatalf("me: %d %s", me.Code, me.Body.String())
	}
	p := testutil.Decode[principalResp](t, me).Principal
	if p.Kind != "user" || p.Email != "ada@example.com" || len(p.Roles) != 1 || p.Roles[0] != "reviewer" {
		t.Fatalf("principal = %+v", p)
	}

	out := httptest.NewRecorder()
	h.ServeHTTP(out, withCookie(httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil), c))
	if out.Code != 204 {
		t.Fatalf("logout: %d", out.Code)
	}
	if cleared := sessionFrom(t, out); cleared.MaxAge >= 0 || cleared.Value != "" {
		t.Fatalf("logout must expire the cookie, got %+v", cleared)
	}

	again := httptest.NewRecorder()
	h.ServeHTTP(again, withCookie(httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil), c))
	if again.Code != 401 || testutil.ErrorCode(t, again) != "unauthenticated" {
		t.Fatalf("me after logout: %d %s", again.Code, again.Body.String())
	}
}

func TestLoginFailures(t *testing.T) {
	env, h := newRouter(t)
	env.User(t, "ada@example.com")
	rec := login(t, h, "ada@example.com", "wrong-password")
	if rec.Code != 401 || testutil.ErrorCode(t, rec) != "invalid_credentials" {
		t.Fatalf("wrong password: %d %s", rec.Code, rec.Body.String())
	}
	if len(rec.Result().Cookies()) != 0 {
		t.Fatal("failed login must not set cookies")
	}
	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/auth/login", "", `{"email":`)
	if rec.Code != 400 || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("malformed: %d %s", rec.Code, rec.Body.String())
	}
}

func TestMeWithAPIKey(t *testing.T) {
	env, h := newRouter(t)
	key := env.APIKey(t, "admin")
	rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", key, nil)
	if rec.Code != 200 {
		t.Fatalf("me: %d %s", rec.Code, rec.Body.String())
	}
	p := testutil.Decode[principalResp](t, rec).Principal
	if p.Kind != "api_key" || p.Roles[0] != "admin" || p.Email != "" {
		t.Fatalf("principal = %+v", p)
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", "", nil); rec.Code != 401 {
		t.Fatalf("anonymous me: %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/httpapi/... -count=1 -run 'TestLogin|TestMe'`
Expected: FAIL — `/api/v1/auth/login` returns 404 (`login: 404 ...`).

- [ ] **Step 3: Write `internal/httpapi/json.go`**

```go
package httpapi

import (
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

type userJSON struct {
	ID        uuid.UUID `json:"id"`
	Email     string    `json:"email"`
	Name      string    `json:"name"`
	Roles     []string  `json:"roles"`
	CreatedAt time.Time `json:"createdAt"`
}

func toUserJSON(u auth.User) userJSON {
	return userJSON{ID: u.ID, Email: u.Email, Name: u.Name, Roles: nonNil(u.Roles), CreatedAt: u.CreatedAt.UTC()}
}

type principalJSON struct {
	Kind  auth.PrincipalKind `json:"kind"`
	ID    uuid.UUID          `json:"id"`
	Name  string             `json:"name"`
	Email string             `json:"email"`
	Roles []string           `json:"roles"`
}

func toPrincipalJSON(p auth.Principal) principalJSON {
	return principalJSON{Kind: p.Kind, ID: p.ID, Name: p.Name, Email: p.Email, Roles: nonNil(p.Roles)}
}

// nonNil makes sure slices serialise as [] rather than null.
func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
```

- [ ] **Step 4: Write `internal/httpapi/auth_handlers.go`**

```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
)

func mountAuth(r chi.Router, d Deps) {
	r.Post("/auth/login", func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := DecodeJSON(req, &body); err != nil {
			WriteError(w, req, err)
			return
		}
		token, u, err := d.Auth.Login(req.Context(), body.Email, body.Password)
		if err != nil {
			WriteError(w, req, err)
			return
		}
		http.SetCookie(w, &http.Cookie{
			Name:     sessionCookie,
			Value:    token,
			Path:     "/",
			MaxAge:   int(auth.SessionTTL.Seconds()),
			HttpOnly: true,
			Secure:   d.Config.CookieSecure,
			SameSite: http.SameSiteLaxMode,
		})
		WriteJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
	})

	r.With(RequireAuth).Post("/auth/logout", func(w http.ResponseWriter, req *http.Request) {
		if c, err := req.Cookie(sessionCookie); err == nil && c.Value != "" {
			if err := d.Auth.Logout(req.Context(), c.Value); err != nil {
				WriteError(w, req, err)
				return
			}
		}
		http.SetCookie(w, &http.Cookie{
			Name: sessionCookie, Value: "", Path: "/", MaxAge: -1,
			HttpOnly: true, Secure: d.Config.CookieSecure, SameSite: http.SameSiteLaxMode,
		})
		w.WriteHeader(http.StatusNoContent)
	})

	r.With(RequireAuth).Get("/auth/me", func(w http.ResponseWriter, req *http.Request) {
		WriteJSON(w, http.StatusOK, map[string]any{"principal": toPrincipalJSON(MustPrincipal(req))})
	})
}
```

- [ ] **Step 5: Mount it in `internal/httpapi/routes.go`**

Replace the line
```go
		// Plan 01 Tasks 9–10 add: mountAuth(r, d); r.Group(RequireAuth: mountAdminUsers, mountAPIKeys).
```
with
```go
		mountAuth(r, d)
		// Plan 01 Task 10 adds the authenticated group (mountAdminUsers, mountAPIKeys).
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/httpapi/... -count=1 -v`
Expected: PASS — all Task 8 tests plus `TestLoginMeLogout`, `TestLoginFailures`, `TestMeWithAPIKey`.

- [ ] **Step 7: Commit**

```bash
git add internal/httpapi
git commit -m "feat: login, logout and me endpoints"
```

---

### Task 10: Users + API keys endpoints

**Parallel group:** B lane 1 (after Task 9; touches `internal/httpapi/users_handlers*.go`, `apikeys_handlers*.go`, `routes.go`)

**Files:**
- Create: `internal/httpapi/users_handlers.go`, `internal/httpapi/apikeys_handlers.go`
- Modify: `internal/httpapi/routes.go`
- Test: `internal/httpapi/users_handlers_test.go`, `internal/httpapi/apikeys_handlers_test.go`

**Interfaces:**
- Consumes: Task 9 `toUserJSON`, `nonNil`; `auth.Service` user/key methods.
- Produces (all admin-only, spec §7.2): `GET /api/v1/users` → `{"items":[User]}`; `POST /api/v1/users` `{email,name,password,roles}` → 201 `{"user"}`; `PATCH /api/v1/users/{id}` `{name?,password?,roles?}` → `{"user"}`; `DELETE /api/v1/users/{id}` → 204; `GET /api/v1/api-keys` → `{"items":[ApiKey]}` where ApiKey = `{id,name,prefix,roles,createdAt,lastUsedAt,revokedAt}`; `POST /api/v1/api-keys` `{name,roles}` → 201 `{"apiKey", "key"}`; `DELETE /api/v1/api-keys/{id}` → 204. Functions `mountAdminUsers`, `mountAPIKeys`.

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/users_handlers_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type userResp struct {
	User struct {
		ID    string   `json:"id"`
		Email string   `json:"email"`
		Name  string   `json:"name"`
		Roles []string `json:"roles"`
	} `json:"user"`
}

func TestUsersCRUD(t *testing.T) {
	env, h := newRouter(t)
	admin := env.APIKey(t, "admin")

	rec := testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "Ada@Example.com", "name": "Ada", "password": "password123", "roles": []string{"reviewer"}})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := testutil.Decode[userResp](t, rec).User
	if created.Email != "ada@example.com" || created.Roles[0] != "reviewer" {
		t.Fatalf("created = %+v", created)
	}

	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "ada@example.com", "name": "Dup", "password": "password123"})
	if rec.Code != 409 || testutil.ErrorCode(t, rec) != "email_taken" {
		t.Fatalf("duplicate: %d %s", rec.Code, rec.Body.String())
	}

	rec = testutil.Do(t, h, http.MethodPost, "/api/v1/users", admin,
		map[string]any{"email": "nope", "name": "", "password": "x"})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("invalid: %d %s", rec.Code, rec.Body.String())
	}

	list := testutil.Decode[struct {
		Items []map[string]any `json:"items"`
	}](t, testutil.Do(t, h, http.MethodGet, "/api/v1/users", admin, nil))
	if len(list.Items) != 1 {
		t.Fatalf("list = %+v", list)
	}

	rec = testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+created.ID, admin,
		map[string]any{"name": "Ada Lovelace", "roles": []string{"reviewer", "hiring-manager"}})
	if rec.Code != 200 {
		t.Fatalf("patch: %d %s", rec.Code, rec.Body.String())
	}
	patched := testutil.Decode[userResp](t, rec).User
	if patched.Name != "Ada Lovelace" || len(patched.Roles) != 2 {
		t.Fatalf("patched = %+v", patched)
	}

	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+created.ID, admin, nil); rec.Code != 204 {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+created.ID, admin, nil); rec.Code != 404 {
		t.Fatalf("second delete: %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodPatch, "/api/v1/users/not-a-uuid", admin, map[string]any{}); rec.Code != 404 {
		t.Fatalf("bad id: %d", rec.Code)
	}
}

func TestUsersRequireAdmin(t *testing.T) {
	env, h := newRouter(t)
	reviewer := env.APIKey(t, "reviewer")
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/users", reviewer, nil); rec.Code != 403 || testutil.ErrorCode(t, rec) != "forbidden" {
		t.Fatalf("reviewer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/users", "", nil); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
}

func TestLastAdminIsProtectedOverHTTP(t *testing.T) {
	env, h := newRouter(t)
	key := env.APIKey(t, "admin")
	root := env.User(t, "root@example.com", "admin")

	rec := testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+root.ID.String(), key, map[string]any{"roles": []string{}})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("demote last admin: %d %s", rec.Code, rec.Body.String())
	}
	rec = testutil.Do(t, h, http.MethodDelete, "/api/v1/users/"+root.ID.String(), key, nil)
	if rec.Code != 422 {
		t.Fatalf("delete last admin: %d %s", rec.Code, rec.Body.String())
	}

	env.User(t, "second@example.com", "admin")
	rec = testutil.Do(t, h, http.MethodPatch, "/api/v1/users/"+root.ID.String(), key, map[string]any{"roles": []string{}})
	if rec.Code != 200 {
		t.Fatalf("demote with second admin: %d %s", rec.Code, rec.Body.String())
	}
}
```

`internal/httpapi/apikeys_handlers_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type createKeyResp struct {
	APIKey struct {
		ID        string   `json:"id"`
		Name      string   `json:"name"`
		Prefix    string   `json:"prefix"`
		Roles     []string `json:"roles"`
		RevokedAt *string  `json:"revokedAt"`
	} `json:"apiKey"`
	Key string `json:"key"`
}

func TestAPIKeysLifecycle(t *testing.T) {
	env, h := newRouter(t)
	admin := env.APIKey(t, "admin")

	rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", admin, map[string]any{"name": "ci", "roles": []string{"reviewer"}})
	if rec.Code != 201 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := testutil.Decode[createKeyResp](t, rec)
	if !strings.HasPrefix(created.Key, "ofk_") || created.APIKey.Prefix != created.Key[:8] || created.APIKey.Roles[0] != "reviewer" {
		t.Fatalf("created = %+v", created)
	}

	// the new key authenticates
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", created.Key, nil); rec.Code != 200 {
		t.Fatalf("me with new key: %d", rec.Code)
	}

	list := testutil.Decode[struct {
		Items []map[string]any `json:"items"`
	}](t, testutil.Do(t, h, http.MethodGet, "/api/v1/api-keys", admin, nil))
	if len(list.Items) != 2 {
		t.Fatalf("items = %d, want 2", len(list.Items))
	}
	for _, it := range list.Items {
		if _, leaked := it["key"]; leaked {
			t.Fatal("list must never include plaintext keys")
		}
	}

	if rec := testutil.Do(t, h, http.MethodDelete, "/api/v1/api-keys/"+created.APIKey.ID, admin, nil); rec.Code != 204 {
		t.Fatalf("revoke: %d %s", rec.Code, rec.Body.String())
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/auth/me", created.Key, nil); rec.Code != 401 {
		t.Fatalf("revoked key must be 401, got %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", admin, map[string]any{"name": ""}); rec.Code != 422 {
		t.Fatalf("invalid create: %d", rec.Code)
	}
	if rec := testutil.Do(t, h, http.MethodGet, "/api/v1/api-keys", created.Key, nil); rec.Code != 401 {
		t.Fatalf("revoked key listing: %d", rec.Code)
	}
}

func TestAPIKeysRequireAdmin(t *testing.T) {
	env, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodPost, "/api/v1/api-keys", env.APIKey(t, "reviewer"), map[string]any{"name": "x"}); rec.Code != 403 {
		t.Fatalf("reviewer: %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/... -count=1 -run 'TestUsers|TestLastAdmin|TestAPIKeys'`
Expected: FAIL — `create: 404 ...`.

- [ ] **Step 3: Write `internal/httpapi/users_handlers.go`**

```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
)

func mountAdminUsers(r chi.Router, d Deps) {
	r.Route("/users", func(r chi.Router) {
		r.Use(RequireAdmin)

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			users, err := d.Auth.ListUsers(req.Context(), MustPrincipal(req).OrgID)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			items := make([]userJSON, len(users))
			for i, u := range users {
				items[i] = toUserJSON(u)
			}
			WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Email    string   `json:"email"`
				Name     string   `json:"name"`
				Password string   `json:"password"`
				Roles    []string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			u, err := d.Auth.CreateUser(req.Context(), MustPrincipal(req).OrgID, body.Email, body.Name, body.Password, body.Roles)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusCreated, map[string]any{"user": toUserJSON(u)})
		})

		r.Patch("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			var body struct {
				Name     *string   `json:"name"`
				Password *string   `json:"password"`
				Roles    *[]string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			in := auth.UpdateUserInput{Name: body.Name, Password: body.Password}
			if body.Roles != nil {
				in.Roles = nonNil(*body.Roles)
			}
			u, err := d.Auth.UpdateUser(req.Context(), MustPrincipal(req).OrgID, id, in)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusOK, map[string]any{"user": toUserJSON(u)})
		})

		r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			if err := d.Auth.DeleteUser(req.Context(), MustPrincipal(req).OrgID, id); err != nil {
				WriteError(w, req, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
```

- [ ] **Step 4: Write `internal/httpapi/apikeys_handlers.go`**

```go
package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

type apiKeyJSON struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Roles      []string   `json:"roles"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func toAPIKeyJSON(k auth.APIKey) apiKeyJSON {
	return apiKeyJSON{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix, Roles: nonNil(k.Roles),
		CreatedAt: k.CreatedAt.UTC(), LastUsedAt: utcPtr(k.LastUsedAt), RevokedAt: utcPtr(k.RevokedAt),
	}
}

func mountAPIKeys(r chi.Router, d Deps) {
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(RequireAdmin)

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			keys, err := d.Auth.ListAPIKeys(req.Context(), MustPrincipal(req).OrgID)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			items := make([]apiKeyJSON, len(keys))
			for i, k := range keys {
				items[i] = toAPIKeyJSON(k)
			}
			WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Name  string   `json:"name"`
				Roles []string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			plaintext, k, err := d.Auth.CreateAPIKey(req.Context(), MustPrincipal(req).OrgID, body.Name, body.Roles)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusCreated, map[string]any{"apiKey": toAPIKeyJSON(k), "key": plaintext})
		})

		r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			if err := d.Auth.RevokeAPIKey(req.Context(), MustPrincipal(req).OrgID, id); err != nil {
				WriteError(w, req, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}
```

- [ ] **Step 5: Mount them in `internal/httpapi/routes.go`**

Replace
```go
		mountAuth(r, d)
		// Plan 01 Task 10 adds the authenticated group (mountAdminUsers, mountAPIKeys).
```
with
```go
		mountAuth(r, d)
		// Public groups (no RequireAuth) are mounted here by later plans: mountPublic (03), mountPublicConfig (09).
		r.Group(func(r chi.Router) {
			r.Use(RequireAuth)
			mountAdminUsers(r, d)
			mountAPIKeys(r, d)
			// Later plans add: mountDefinitions, mountSubmissions (03); mountWorkflow, mountJobs (04).
		})
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test ./internal/httpapi/... -count=1 -v`
Expected: PASS — all previous httpapi tests plus `TestUsersCRUD`, `TestUsersRequireAdmin`, `TestLastAdminIsProtectedOverHTTP`, `TestAPIKeysLifecycle`, `TestAPIKeysRequireAdmin`.

- [ ] **Step 7: Commit**

```bash
git add internal/httpapi
git commit -m "feat: admin users and API keys endpoints"
```

---

### Task 11: App wiring + CLI `serve` / `migrate`

**Parallel group:** B lane 2 (touches `internal/app/`, `internal/cli/`, `cmd/`, `go.mod`, `go.sum`). Runs concurrently with Tasks 9–10; it only calls `httpapi.NewRouter`, which exists after Task 8.

**Files:**
- Create: `internal/app/app.go`, `internal/cli/root.go`, `internal/cli/serve.go`, `internal/cli/migrate.go`, `cmd/openforms/main.go`
- Test: `internal/app/app_test.go`, `internal/cli/cli_test.go`

**Interfaces:**
- Consumes: `config.Load/RequireDatabase`, `db.Open/Migrate`, `auth.NewService/EnsureDefaultOrg`, `httpapi.NewRouter/Deps`, `webui.Handler`, `dbtest.NewURL`.
- Produces (spec §6.13, §8):
```go
// package app
type App struct { Config config.Config; Pool *pgxpool.Pool; Auth *auth.Service; OrgID uuid.UUID; Handler http.Handler }
func New(ctx context.Context, cfg config.Config) (*App, error)
func (a *App) Run(ctx context.Context) error   // HTTP server until ctx done; graceful shutdown 10s
func (a *App) Close()
// package cli
func Execute()                      // os.Exit(1) on error
func NewRootCmd() *cobra.Command    // "openforms" with subcommands serve, migrate
```
Plan 04 adds `Queue`/`Engine` fields and starts the worker inside `Run`; Plan 03 adds `Defs`/`Subs`. Plan 05 adds more commands by appending to `root.AddCommand(...)` in `NewRootCmd`.

- [ ] **Step 1: Add dependency**

Run: `go get github.com/spf13/cobra@latest`

- [ ] **Step 2: Write the failing tests**

`internal/app/app_test.go`:
```go
package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/app"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db/dbtest"
)

func testConfig(t *testing.T) config.Config {
	return config.Config{
		HTTPAddr: "127.0.0.1:0", DatabaseURL: dbtest.NewURL(t),
		BaseURL: "http://test.local", SMTPPort: 1025, WorkerConcurrency: 1,
	}
}

func TestNewServesHealthzAndAPI(t *testing.T) {
	a, err := app.New(context.Background(), testConfig(t))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer a.Close()
	if a.OrgID == uuid.Nil {
		t.Fatal("OrgID not set")
	}

	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
	// Uses an unknown API path (not /auth/me) so this test does not depend on Task 9, which runs in parallel.
	rec = httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("unknown api path: %d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("admin without web build: %d, want 503", rec.Code)
	}
}

func TestNewIsRestartSafe(t *testing.T) {
	cfg := testConfig(t)
	a1, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	org := a1.OrgID
	a1.Close()
	a2, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	defer a2.Close()
	if a2.OrgID != org {
		t.Fatalf("org changed across restarts: %v -> %v", org, a2.OrgID)
	}
}

func TestNewRequiresDatabaseURL(t *testing.T) {
	if _, err := app.New(context.Background(), config.Config{}); err == nil || !strings.Contains(err.Error(), "OPENFORMS_DATABASE_URL") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	a, err := app.New(context.Background(), testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
```

`internal/cli/cli_test.go`:
```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/cli"
	"github.com/openforms/openforms/internal/db/dbtest"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestMigrateCommand(t *testing.T) {
	t.Setenv("OPENFORMS_DATABASE_URL", dbtest.NewURL(t))
	out, err := run(t, "migrate")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "migrations applied") {
		t.Fatalf("output = %q", out)
	}
}

func TestServeAndMigrateRequireDatabaseURL(t *testing.T) {
	t.Setenv("OPENFORMS_DATABASE_URL", "")
	for _, sub := range []string{"serve", "migrate"} {
		out, err := run(t, sub)
		if err == nil || !strings.Contains(err.Error()+out, "OPENFORMS_DATABASE_URL") {
			t.Errorf("%s: err=%v out=%q", sub, err, out)
		}
	}
}

func TestRootHelpListsCommands(t *testing.T) {
	out, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"serve", "migrate"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/app/... ./internal/cli/... -count=1`
Expected: FAIL — `no required module provides package .../internal/app` / `undefined: cli.NewRootCmd`.

- [ ] **Step 4: Write `internal/app/app.go`**

```go
// Package app wires configuration, database, services and the HTTP router together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/webui"
)

type App struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Auth    *auth.Service
	OrgID   uuid.UUID
	Handler http.Handler
}

// New opens the pool, migrates, ensures the default org and builds the router.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.RequireDatabase(); err != nil {
		return nil, err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	authSvc := auth.NewService(pool)
	orgID, err := authSvc.EnsureDefaultOrg(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("ensure default org: %w", err)
	}
	a := &App{Config: cfg, Pool: pool, Auth: authSvc, OrgID: orgID}
	a.Handler = httpapi.NewRouter(httpapi.Deps{
		Config: cfg, Pool: pool, Auth: authSvc, Web: webui.Handler(), OrgID: orgID,
	})
	return a, nil
}

// Run serves HTTP until ctx is cancelled, then shuts down gracefully (10s).
func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.Config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.Config.HTTPAddr, err)
	}
	srv := &http.Server{Handler: a.Handler, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("openforms listening", "addr", ln.Addr().String(), "baseURL", a.Config.BaseURL)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (a *App) Close() { a.Pool.Close() }
```

- [ ] **Step 5: Write the CLI**

`internal/cli/root.go`:
```go
// Package cli implements the openforms command (server and developer CLI in one binary).
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// NewRootCmd builds the command tree. Later plans append subcommands here.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "openforms",
		Short:         "Developer-first forms with submission workflows",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newServeCmd(), newMigrateCmd())
	return root
}
```

`internal/cli/serve.go`:
```go
package cli

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/app"
	"github.com/openforms/openforms/internal/config"
)

func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the openforms server (migrates on start)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			a, err := app.New(ctx, cfg)
			if err != nil {
				return err
			}
			defer a.Close()
			return a.Run(ctx)
		},
	}
}
```

`internal/cli/migrate.go`:
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
)

func newMigrateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply database migrations and exit",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := cfg.RequireDatabase(); err != nil {
				return err
			}
			pool, err := db.Open(cmd.Context(), cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := db.Migrate(cmd.Context(), pool); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "migrations applied")
			return nil
		},
	}
}
```

`cmd/openforms/main.go`:
```go
package main

import "github.com/openforms/openforms/internal/cli"

func main() {
	cli.Execute()
}
```

Note: `cmd.Context()` is nil when a command is executed via `Execute()` without `ExecuteContext`; cobra substitutes `context.Background()` since v1.3 — no extra handling needed.

- [ ] **Step 6: Run tests to verify they pass**

Run: `go mod tidy && go test ./internal/app/... ./internal/cli/... -count=1 -v`
Expected: PASS — `TestNewServesHealthzAndAPI`, `TestNewIsRestartSafe`, `TestNewRequiresDatabaseURL`, `TestRunStopsOnContextCancel`, `TestMigrateCommand`, `TestServeAndMigrateRequireDatabaseURL`, `TestRootHelpListsCommands`.

- [ ] **Step 7: Smoke-test the real binary** (run after Group B lane 1 — Tasks 9–10 — has been merged, since it calls `/api/v1/auth/me`)

Run:
```bash
go build -o bin/openforms ./cmd/openforms
export OPENFORMS_DATABASE_URL="postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable"
./bin/openforms migrate
(./bin/openforms serve &) ; sleep 2
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/api/v1/auth/me
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8080/admin
```
Expected: `migrations applied`; `{"status":"ok"}`; `{"error":{"code":"unauthenticated","message":"authentication required"}}`; `503`. Stop the server afterwards (`taskkill //IM openforms.exe //F` in Git Bash on Windows, or `pkill openforms` elsewhere).

- [ ] **Step 8: Full suite + lint, then commit**

Run:
```bash
go vet ./... && gofmt -l . && go test ./... -count=1
```
Expected: no `gofmt` output; every package `ok`.

```bash
git add go.mod go.sum internal/app internal/cli cmd
git commit -m "feat: app wiring and serve/migrate commands"
```

---

## Plan-level definition of done

- `go test ./... -count=1` passes with `docker compose up -d postgres mailpit` running.
- `./bin/openforms serve` answers `/healthz` with `{"status":"ok"}`, `/api/v1/auth/me` with 401 envelope, `/admin` with 503 "web UI not built".
- An admin can be bootstrapped only through code/tests at this stage (the `openforms admin create-user` command arrives in Plan 05).

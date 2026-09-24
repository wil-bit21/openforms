# openforms — Design Spec

**Date:** 2026-09-23
**Status:** Draft for review

## 1. Intent

openforms is a **public, open-source, developer-first alternative to Typeform/Tally**. Its differentiators:

1. **Forms as code** — form and workflow definitions are JSON/YAML documents. The server stores them (API is the source of truth) and a CLI syncs them to and from files in git (`pull` / `push` / `diff` / `validate`).
2. **Submission workflows** — every form may reference a *workflow*: a per-form state machine with custom states, guarded transitions (role and required-field guards) and side-effect actions (webhook, email, assign).
3. **Headless first, hosted too** — a REST API plus a TypeScript SDK/React renderer for developers who render forms themselves, and a plain hosted form page (`/f/<slug>`) with an iframe embed for everyone else.
4. **Full admin UI** — a reviewer inbox (list, detail, transitions, history) and visual editors for forms and workflows.

**v1 is self-hosted, single-organization** (one Docker image + Postgres). Every tenant-owned table carries `org_id` so a multi-tenant SaaS is possible later without a data-model rewrite.

**Success criteria (v1):** A developer can `openforms init`, define a form and workflow in YAML, `openforms push`, collect responses via the hosted page or their own UI, and move each submission through its states in the inbox or via API — guards enforced, actions firing (with retries), full audit history — and a visitor to `/demo` can experience the whole loop in a browser within two minutes.

### Out of scope for v1
Multi-tenancy/signup, billing, file-upload fields, payments, third-party integrations beyond webhook + email (no Zapier/Sheets), multi-page forms, partial/resumable drafts, migrating in-flight submissions to a newer workflow version, i18n of the UI, SSO/OAuth.

## 2. Tech stack & global constraints

> **Superseded for the backend by Plan 10** (`plans/2026-09-24-openforms-10-python-backend.md`): the server and CLI are a Python package (FastAPI, SQLAlchemy 2 async + asyncpg, Alembic, cyclopts, uv). §3's backend layout and §6's Go package contracts map onto that plan's module map; §4, §5, §7, §8 and §10 stay binding.

| Area | Choice |
|---|---|
| Backend | Go ≥ 1.24, module `github.com/openforms/openforms` at repo root |
| HTTP | `github.com/go-chi/chi/v5` |
| DB | PostgreSQL 16, `github.com/jackc/pgx/v5` (pgxpool), migrations with `github.com/pressly/goose/v3` (embedded SQL) |
| IDs | `github.com/google/uuid` (UUID v4 generated in Go) |
| JSON Schema | `github.com/santhosh-tekuri/jsonschema/v6` (Go), `ajv` (TS editors) |
| YAML | `sigs.k8s.io/yaml` (Go), `yaml` npm package (TS) |
| CLI | `github.com/spf13/cobra`; the **same binary** `openforms` is server + CLI |
| Passwords | `golang.org/x/crypto/bcrypt` (cost 12) |
| Frontend | TypeScript 5, Node 22 LTS, pnpm 9 workspace at `web/`, React 18, Vite 6, Vitest, @testing-library/react, MSW 2 |
| Admin extras | react-router-dom 6, @tanstack/react-query 5, @xyflow/react + @dagrejs/dagre (workflow diagram) |
| E2E | Playwright |
| Dev services | docker compose: Postgres 16 on host port **54329**, Mailpit (SMTP 1025, UI 8025) |
| Packaging | Single Docker image; web builds embedded into the Go binary via `embed.FS` |

Global rules:
- Go tests that need a DB use `dbtest.New(t)` (isolated schema per test). Default test DSN: `postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable`, override with `OPENFORMS_TEST_DATABASE_URL`.
- All JSON over the wire is **camelCase**. Timestamps are RFC 3339 UTC strings.
- Every API error uses the error envelope in §7.1.
- The Go server is **authoritative** for validation. TS validation exists only for UX and must pass the shared conformance fixtures (§5.4).
- No placeholder UI copy like "Lorem ipsum" in shipped pages.
- Commit after every task; conventional commit prefixes (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).

## 3. Repository layout

> **Plan 10:** the Go tree below (`cmd/`, `internal/`, `go.mod`) was replaced by the `src/openforms` Python package; see that plan's module map.

```
openforms/
  go.mod, go.sum
  Makefile
  docker-compose.yml          # postgres, mailpit (dev); openforms service under profile "app"
  Dockerfile
  cmd/openforms/main.go       # calls cli.Execute()
  schemas/
    schemas.go                # package schemas; //go:embed *.schema.json fixtures/*.json
    form.schema.json
    workflow.schema.json
    fixtures/visibility.json  # shared Go/TS conformance cases
    fixtures/submission.json
  internal/
    config/                   # env config
    db/                       # Open, Migrate, WithTx, DBTX; migrations/*.sql
    db/dbtest/                # New(t) isolated test schema
    definition/               # types, Parse*, semantic validation, Visible, ValidateSubmission, Canonical
    auth/                     # users, sessions, api keys, principal
    definitions/              # Store: versioned persistence, Apply bundle, Export
    submissions/              # Service: create, get, list, events, public status, CSV
    jobs/                     # Postgres job queue + worker
    actions/                  # webhook/email/assign job handlers, template rendering, mailer
    workflow/                 # Engine: transitions, guards, fields, comments, assignment
    httpapi/                  # chi router, handlers, middleware, error envelope, rate limit
    webui/                    # embed.FS of dist/ + SPA handler
    webui/dist/.gitkeep
    client/                   # Go HTTP client for the API (used by CLI)
    cli/                      # cobra commands
    app/                      # wiring: app.New(cfg) → *App{Handler, Queue, ...}; Run
  examples/openforms/         # demo bundle used by `openforms seed --demo` and the demo page
    forms/job-application.yaml
    forms/contact.yaml
    workflows/hiring.yaml
    workflows/contact-triage.yaml
  web/
    package.json, pnpm-workspace.yaml, tsconfig.base.json
    packages/sdk/             # @openforms/sdk
    packages/react/           # @openforms/react
    packages/embed/           # embed.js (IIFE)
    apps/hosted/              # hosted form + status pages
    apps/admin/               # admin SPA (inbox + editors)
    apps/demo/                # /demo page
  e2e/                        # Playwright
  docs/                       # user docs + superpowers specs/plans
```

**Web build contract:** `pnpm -C web build` builds every app and copies outputs to `internal/webui/dist/`:
`dist/admin/`, `dist/hosted/`, `dist/demo/` (each with `index.html` + `assets/`), and `dist/embed/embed.js`. Every Vite app sets `base: "/_app/<name>/"` (admin, hosted, demo).
`web/package.json` scripts (Plan 06 creates; later plans only add workspace members): `build` = `pnpm -r --filter "./packages/*" build && pnpm -r --filter "./apps/*" build && node scripts/copy-dist.mjs`; `test` = `pnpm -r test`; `typecheck` = `pnpm -r typecheck`. `scripts/copy-dist.mjs` wipes `internal/webui/dist/*` (keeping `.gitkeep`), copies every `apps/<name>/dist` that exists to `internal/webui/dist/<name>`, and `packages/embed/dist/embed.js` to `internal/webui/dist/embed/embed.js`. Dev: each app's Vite dev server proxies `/api` and `/healthz` to `http://localhost:8080`.

## 4. Data model (PostgreSQL)

Three migrations, each owned by one plan:

**`internal/db/migrations/00001_core.sql`** (Plan 01)
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
DROP TABLE api_keys; DROP TABLE sessions; DROP TABLE users; DROP TABLE orgs;
```

**`00002_definitions_submissions.sql`** (Plan 03)
```sql
-- +goose Up
CREATE TABLE workflows (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  slug text NOT NULL,
  current_version_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);
CREATE TABLE workflow_versions (
  id uuid PRIMARY KEY,
  workflow_id uuid NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
  version int NOT NULL,
  definition jsonb NOT NULL,
  hash text NOT NULL,
  source text NOT NULL CHECK (source IN ('cli','ui','api','seed')),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (workflow_id, version)
);
CREATE TABLE forms (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  slug text NOT NULL,
  current_version_id uuid,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (org_id, slug)
);
CREATE TABLE form_versions (
  id uuid PRIMARY KEY,
  form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  version int NOT NULL,
  definition jsonb NOT NULL,
  hash text NOT NULL,
  workflow_version_id uuid REFERENCES workflow_versions(id),
  source text NOT NULL CHECK (source IN ('cli','ui','api','seed')),
  created_by text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (form_id, version)
);
CREATE TABLE submissions (
  id uuid PRIMARY KEY,
  org_id uuid NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  form_id uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
  form_version_id uuid NOT NULL REFERENCES form_versions(id),
  workflow_version_id uuid REFERENCES workflow_versions(id),
  state text NOT NULL,
  data jsonb NOT NULL,
  fields jsonb NOT NULL DEFAULT '{}',
  assignee_id uuid REFERENCES users(id) ON DELETE SET NULL,
  receipt_token_hash text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submissions_list ON submissions (org_id, created_at DESC, id DESC);
CREATE INDEX submissions_form_state ON submissions (form_id, state);
CREATE INDEX submissions_assignee ON submissions (assignee_id);
CREATE TABLE submission_events (
  id bigserial PRIMARY KEY,
  submission_id uuid NOT NULL REFERENCES submissions(id) ON DELETE CASCADE,
  org_id uuid NOT NULL,
  type text NOT NULL,
  from_state text,
  to_state text,
  transition text,
  actor_type text NOT NULL CHECK (actor_type IN ('user','api_key','system','respondent')),
  actor_id uuid,
  actor_name text NOT NULL DEFAULT '',
  payload jsonb NOT NULL DEFAULT '{}',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX submission_events_sub ON submission_events (submission_id, id);
-- +goose Down
DROP TABLE submission_events; DROP TABLE submissions; DROP TABLE form_versions;
DROP TABLE forms; DROP TABLE workflow_versions; DROP TABLE workflows;
```

**`00003_jobs.sql`** (Plan 04)
```sql
-- +goose Up
CREATE TABLE jobs (
  id bigserial PRIMARY KEY,
  org_id uuid NOT NULL,
  kind text NOT NULL,
  payload jsonb NOT NULL,
  status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','running','done','failed')),
  attempts int NOT NULL DEFAULT 0,
  max_attempts int NOT NULL DEFAULT 8,
  run_at timestamptz NOT NULL DEFAULT now(),
  locked_until timestamptz,
  last_error text NOT NULL DEFAULT '',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX jobs_claim ON jobs (status, run_at);
-- +goose Down
DROP TABLE jobs;
```

Event `type` values: `created`, `transition`, `fields_updated`, `assigned`, `comment`, `action_succeeded`, `action_failed`.

## 5. Definitions

### 5.1 Form definition (YAML or JSON)

```yaml
slug: job-application            # ^[a-z0-9][a-z0-9-]{0,62}$
title: Job application
description: Apply to join the team.   # optional
workflow: hiring                  # optional; slug of a workflow
settings:
  public: true                    # accepts anonymous submissions via public API/hosted page
  submitLabel: Send application   # optional, default "Submit"
  confirmationMessage: Thanks! We'll be in touch.  # optional
fields:
  - key: name                     # ^[a-zA-Z][a-zA-Z0-9_]{0,63}$, unique
    type: text                    # text|textarea|email|number|select|multiselect|checkbox|date|url
    label: Full name
    required: true
    placeholder: Ada Lovelace     # optional
    help: As on your passport     # optional
    validation: { minLength: 2, maxLength: 100 }   # minLength,maxLength,pattern for strings; min,max for number
  - key: role
    type: select
    label: Role
    required: true
    options:
      - { value: engineer, label: Engineer }
      - { value: designer, label: Designer }
  - key: portfolio
    type: url
    label: Portfolio URL
    showIf: { field: role, equals: designer }    # exactly one of equals|notEquals|in
```

### 5.2 Workflow definition

```yaml
slug: hiring
title: Hiring pipeline
initial: new
states:
  - { key: new, label: New, color: gray }            # color optional: gray|blue|green|yellow|red|purple
  - { key: screening, label: Screening, color: blue }
  - { key: interview, label: Interview, color: purple }
  - { key: hired, label: Hired, color: green, terminal: true }
  - { key: rejected, label: Rejected, color: red, terminal: true }
fields:                                 # workflow-managed data on each submission
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
onSubmit:                               # actions when a submission is created
  - { type: assign, role: reviewer }
  - { type: email, to: "{{submission.data.email}}", subject: "We got your application", body: "Hi {{submission.data.name}}, ..." }
transitions:
  - key: screen
    label: Start screening
    from: [new]
    to: screening
    guard: { roles: [reviewer] }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "https://example.com/hooks/interview" }
  - key: hire
    label: Hire
    from: [interview]
    to: hired
    guard: { roles: [hiring-manager] }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
```

Workflow field types: `text|textarea|number|select|checkbox|date` (select requires `options`).
Action types: `webhook` (`url`, http/https), `email` (`to`, `subject`, `body`; templated), `assign` (exactly one of `user` = email, or `role`).

### 5.3 Semantic validation rules (Go `definition` package, beyond JSON Schema)

Forms:
- slug and field keys match the regexes above; field keys unique.
- `select`/`multiselect` require ≥1 option; option values unique. Other types must not have `options`.
- `validation.minLength/maxLength/pattern` only on text/textarea/email/url; `min/max` only on number; min ≤ max; `pattern` must compile as RE2.
- `showIf.field` must reference a field declared **earlier** in the list; exactly one of `equals`, `notEquals`, `in`.

Workflows:
- slug regex; state keys unique and match field-key regex; `initial` is a declared state.
- transition keys unique; every `from` entry and `to` are declared states; a transition must not leave a `terminal` state; `from` non-empty.
- `guard.requireFields` entries are declared workflow fields; workflow field keys unique.
- action rules as in 5.2.

Bundle (`ValidateBundle`): every form's `workflow` must resolve to a workflow in the bundle or in `existingWorkflowSlugs`.

Problems carry a JSON-pointer-ish `path` like `fields[2].showIf.field` or `transitions[0].to`.

### 5.4 Submission semantics & conformance fixtures

- **Visibility:** field visible if no `showIf`, or its condition holds against *cleaned* data of earlier fields; a field whose controlling field is hidden is hidden. `equals`/`notEquals` compare with JSON equality; `in` is membership; for `multiselect` controllers, `equals: x` means "contains x".
- **Cleaning:** unknown keys and hidden fields are dropped. Empty string / null / empty array for a field = "not provided".
- **Types:** text/textarea/email/url/date/select → string; number → JSON number; checkbox → boolean; multiselect → array of strings. `date` is `YYYY-MM-DD`. `email` must parse as a bare address. `url` must be absolute http(s) with host. `select` value must be an option; `multiselect` values must all be options (no duplicates).
- **Required:** a visible required field must be provided; a required `checkbox` must be `true`.
- **Problem paths:** `data.<key>`.

`schemas/fixtures/visibility.json`: `[{ "name", "form", "data", "visible": {key: bool} }]`.
`schemas/fixtures/submission.json`: `[{ "name", "form", "data", "clean": {...} | null, "errorPaths": ["data.x", ...] }]` (`clean` is null when errors are expected). Go (`definition` tests) and TS (`@openforms/sdk` tests) both iterate every case.

### 5.5 Versioning

- Definitions are immutable per version. `Canonical(def)` = JSON with sorted keys, no insignificant whitespace; `hash` = sha256 hex of canonical bytes. Applying a definition with an unchanged hash creates no version (`changed: false`).
- A form version pins the workflow version that was current when the form version was created (`workflow_version_id`). Applying a new workflow version automatically creates a new version of **every form referencing it** (same definition, new pin, `source` of the apply).
- A submission pins `form_version_id` and `workflow_version_id` at creation; transitions always use the pinned workflow version.
- `source` records who created a version: `cli` (push), `ui` (admin editor), `api` (direct PUT), `seed`.

## 6. Go package contracts

> **Plan 10:** these Go contracts now live in Python modules with the same behaviour (`openforms.definition`, `openforms.server.models.*`, `openforms.server.workflow`, `openforms.server.services.jobs`, `openforms.server.actions`, `openforms.client`, `openforms.cli`); see that plan's module map.

These signatures are binding across plans. Package paths are under `github.com/openforms/openforms/`.

### 6.1 `internal/config`
```go
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
func Load() (Config, error)
```

### 6.2 `internal/db`
```go
type DBTX interface {
    Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
    Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
    QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}
func Open(ctx context.Context, url string) (*pgxpool.Pool, error)
func Migrate(ctx context.Context, pool *pgxpool.Pool) error          // goose up, embedded migrations/*.sql
func WithTx(ctx context.Context, pool *pgxpool.Pool, fn func(tx pgx.Tx) error) error
```
`internal/db/dbtest`: `func New(t testing.TB) *pgxpool.Pool` — creates schema `t_<16 hex>`, opens a pool whose connections use `search_path=<schema>`, runs `db.Migrate`, registers cleanup that closes the pool and drops the schema.

### 6.3 `schemas` (root package, Plan 02)
```go
//go:embed form.schema.json workflow.schema.json
var FS embed.FS
//go:embed fixtures/*.json
var Fixtures embed.FS
```

### 6.4 `internal/definition` (Plan 02)
```go
type FieldType string
const (FieldText FieldType = "text"; FieldTextarea = "textarea"; FieldEmail = "email"; FieldNumber = "number";
       FieldSelect = "select"; FieldMultiselect = "multiselect"; FieldCheckbox = "checkbox"; FieldDate = "date"; FieldURL = "url")
type Option struct { Value string `json:"value"`; Label string `json:"label"` }
type Validation struct {
    MinLength *int `json:"minLength,omitempty"`; MaxLength *int `json:"maxLength,omitempty"`
    Pattern string `json:"pattern,omitempty"`; Min *float64 `json:"min,omitempty"`; Max *float64 `json:"max,omitempty"`
}
type Condition struct { Field string `json:"field"`; Equals any `json:"equals,omitempty"`; NotEquals any `json:"notEquals,omitempty"`; In []any `json:"in,omitempty"` }
type Field struct {
    Key string `json:"key"`; Type FieldType `json:"type"`; Label string `json:"label"`
    Help string `json:"help,omitempty"`; Placeholder string `json:"placeholder,omitempty"`; Required bool `json:"required,omitempty"`
    Options []Option `json:"options,omitempty"`; Validation *Validation `json:"validation,omitempty"`; ShowIf *Condition `json:"showIf,omitempty"`
}
type FormSettings struct { Public bool `json:"public"`; SubmitLabel string `json:"submitLabel,omitempty"`; ConfirmationMessage string `json:"confirmationMessage,omitempty"` }
type Form struct {
    Slug string `json:"slug"`; Title string `json:"title"`; Description string `json:"description,omitempty"`
    Workflow string `json:"workflow,omitempty"`; Settings FormSettings `json:"settings"`; Fields []Field `json:"fields"`
}
type State struct { Key string `json:"key"`; Label string `json:"label"`; Color string `json:"color,omitempty"`; Terminal bool `json:"terminal,omitempty"` }
type WorkflowField struct { Key string `json:"key"`; Type FieldType `json:"type"`; Label string `json:"label"`; Options []Option `json:"options,omitempty"` }
type Guard struct { Roles []string `json:"roles,omitempty"`; RequireFields []string `json:"requireFields,omitempty"` }
type ActionType string
const (ActionWebhook ActionType = "webhook"; ActionEmail = "email"; ActionAssign = "assign")
type Action struct {
    Type ActionType `json:"type"`; URL string `json:"url,omitempty"`
    To string `json:"to,omitempty"`; Subject string `json:"subject,omitempty"`; Body string `json:"body,omitempty"`
    User string `json:"user,omitempty"`; Role string `json:"role,omitempty"`
}
type Transition struct {
    Key string `json:"key"`; Label string `json:"label"`; From []string `json:"from"`; To string `json:"to"`
    Guard Guard `json:"guard"`; Actions []Action `json:"actions,omitempty"`
}
type Workflow struct {
    Slug string `json:"slug"`; Title string `json:"title"`; Initial string `json:"initial"`
    States []State `json:"states"`; Fields []WorkflowField `json:"fields,omitempty"`
    OnSubmit []Action `json:"onSubmit,omitempty"`; Transitions []Transition `json:"transitions"`
}
func (w Workflow) State(key string) (State, bool)
func (w Workflow) Transition(key string) (Transition, bool)
func (w Workflow) Field(key string) (WorkflowField, bool)

type Problem struct { Path string `json:"path"`; Message string `json:"message"` }
type ValidationError struct { Problems []Problem }
func (e *ValidationError) Error() string

func ParseForm(raw []byte) (Form, error)            // YAML or JSON → JSON Schema → semantic rules; returns *ValidationError on invalid input
func ParseWorkflow(raw []byte) (Workflow, error)
func ValidateForm(f Form) error                     // semantic only (used after programmatic construction)
func ValidateWorkflow(w Workflow) error
func ValidateBundle(forms []Form, workflows []Workflow, existingWorkflowSlugs []string) error
func Canonical(v any) (canonical []byte, hash string, err error)
func Visible(f Form, data map[string]any) map[string]bool
func ValidateSubmission(f Form, data map[string]any) (map[string]any, error)       // returns cleaned data or *ValidationError
func ValidateWorkflowFields(w Workflow, fields map[string]any) (map[string]any, error) // type-checks values against w.Fields; unknown key → problem "fields.<key>"; null value = unset (key removed)
```

### 6.5 `internal/auth`
```go
const RoleAdmin = "admin"
var (ErrInvalidCredentials, ErrUnauthenticated, ErrNotFound, ErrEmailTaken error)
type PrincipalKind string
const (PrincipalUser PrincipalKind = "user"; PrincipalAPIKey PrincipalKind = "api_key")
type Principal struct { OrgID uuid.UUID; Kind PrincipalKind; ID uuid.UUID; Name, Email string; Roles []string }
func (p Principal) IsAdmin() bool
func (p Principal) HasAnyRole(roles ...string) bool   // true if admin, or roles is empty, or intersects
type User struct { ID, OrgID uuid.UUID; Email, Name string; Roles []string; CreatedAt time.Time }
type APIKey struct { ID, OrgID uuid.UUID; Name, Prefix string; Roles []string; CreatedAt time.Time; LastUsedAt, RevokedAt *time.Time }
type UpdateUserInput struct { Name, Password *string; Roles []string /* nil = unchanged */ }
type Service struct{ /* pool */ }
func NewService(pool *pgxpool.Pool) *Service
func (s *Service) EnsureDefaultOrg(ctx context.Context) (uuid.UUID, error)     // idempotent; org named "Default"
func (s *Service) CreateUser(ctx context.Context, orgID uuid.UUID, email, name, password string, roles []string) (User, error)
func (s *Service) ListUsers(ctx context.Context, orgID uuid.UUID) ([]User, error)
func (s *Service) GetUserByEmail(ctx context.Context, orgID uuid.UUID, email string) (User, error)
func (s *Service) UsersWithRole(ctx context.Context, orgID uuid.UUID, role string) ([]User, error)
func (s *Service) UpdateUser(ctx context.Context, orgID, id uuid.UUID, in UpdateUserInput) (User, error)
func (s *Service) DeleteUser(ctx context.Context, orgID, id uuid.UUID) error
func (s *Service) Login(ctx context.Context, email, password string) (token string, u User, err error)  // session TTL 30 days
func (s *Service) Logout(ctx context.Context, token string) error
func (s *Service) PrincipalFromSession(ctx context.Context, token string) (Principal, error)
func (s *Service) CreateAPIKey(ctx context.Context, orgID uuid.UUID, name string, roles []string) (plaintext string, k APIKey, err error) // "ofk_" + 40 base62 chars; prefix = first 8 chars
func (s *Service) ListAPIKeys(ctx context.Context, orgID uuid.UUID) ([]APIKey, error)
func (s *Service) RevokeAPIKey(ctx context.Context, orgID, id uuid.UUID) error
func (s *Service) PrincipalFromAPIKey(ctx context.Context, plaintext string) (Principal, error)
func WithPrincipal(ctx context.Context, p Principal) context.Context
func PrincipalFrom(ctx context.Context) (Principal, bool)
```
Session tokens: 32 random bytes base64url; DB stores sha256 hex. Cookie `of_session`, HttpOnly, SameSite=Lax, Path=/, Secure per config. Emails lower-cased on write. Passwords ≥ 8 chars.

### 6.6 `internal/definitions`
```go
var ErrNotFound = errors.New("definition not found")
type Source string
const (SourceCLI Source = "cli"; SourceUI = "ui"; SourceAPI = "api"; SourceSeed = "seed")
type VersionInfo struct { ID uuid.UUID; Version int; Hash string; Source Source; CreatedBy string; CreatedAt time.Time }
type FormRecord struct {
    ID uuid.UUID; Slug string; Current VersionInfo; WorkflowVersionID *uuid.UUID
    Definition definition.Form; CreatedAt, UpdatedAt time.Time
}
type WorkflowRecord struct { ID uuid.UUID; Slug string; Current VersionInfo; Definition definition.Workflow; CreatedAt, UpdatedAt time.Time }
type ApplyInput struct { Forms []definition.Form; Workflows []definition.Workflow; Source Source; Actor string; DryRun bool }
type ApplyItem struct { Kind string /* "form"|"workflow" */; Slug string; Version int; Changed bool; Created bool }
type ApplyResult struct { Items []ApplyItem }
type Store struct{ /* pool + immutable-version caches */ }
func NewStore(pool *pgxpool.Pool) *Store
func (s *Store) Apply(ctx context.Context, orgID uuid.UUID, in ApplyInput) (ApplyResult, error) // one tx; validates bundle; workflows then forms; re-pins dependent forms; DryRun rolls back
func (s *Store) GetForm(ctx context.Context, orgID uuid.UUID, slug string) (FormRecord, error)
func (s *Store) GetFormVersion(ctx context.Context, versionID uuid.UUID) (FormRecord, error)
func (s *Store) ListForms(ctx context.Context, orgID uuid.UUID) ([]FormRecord, error)
func (s *Store) FormVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error)          // newest first
func (s *Store) FormVersion(ctx context.Context, orgID uuid.UUID, slug string, version int) (FormRecord, error)
func (s *Store) GetWorkflow(ctx context.Context, orgID uuid.UUID, slug string) (WorkflowRecord, error)
func (s *Store) GetWorkflowVersion(ctx context.Context, versionID uuid.UUID) (WorkflowRecord, error)   // cached
func (s *Store) ListWorkflows(ctx context.Context, orgID uuid.UUID) ([]WorkflowRecord, error)
func (s *Store) WorkflowVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error)
func (s *Store) WorkflowVersion(ctx context.Context, orgID uuid.UUID, slug string, version int) (WorkflowRecord, error)
func (s *Store) Export(ctx context.Context, orgID uuid.UUID) ([]definition.Form, []definition.Workflow, error) // sorted by slug
```

### 6.7 `internal/submissions`
```go
var (ErrNotFound, ErrFormNotPublic error)
type Submission struct {
    ID, OrgID, FormID, FormVersionID uuid.UUID; FormSlug string; FormVersion int
    WorkflowVersionID *uuid.UUID; State, StateLabel string; Terminal bool
    Data, Fields map[string]any
    AssigneeID *uuid.UUID; AssigneeName, AssigneeEmail string
    CreatedAt, UpdatedAt time.Time
}
type Event struct {
    ID int64; SubmissionID, OrgID uuid.UUID; Type string
    FromState, ToState, Transition string   // "" when not applicable (NULL in DB)
    ActorType string; ActorID *uuid.UUID; ActorName string
    Payload map[string]any; CreatedAt time.Time
}
type Actor struct { Type string /* user|api_key|system|respondent */; ID *uuid.UUID; Name string }
func ActorFromPrincipal(p auth.Principal) Actor
type ListFilter struct { FormSlug, State string; AssigneeID *uuid.UUID; Unassigned bool; Cursor string; Limit int /* default 50, max 200 */ }
type PublicStatus struct {
    ID uuid.UUID; FormTitle, State, StateLabel string; Terminal bool
    States []definition.State; History []PublicHistoryItem; CreatedAt time.Time
}
type PublicHistoryItem struct { State, Label string; At time.Time }
type CreatedHook func(ctx context.Context, tx pgx.Tx, sub Submission, wf *definition.Workflow) error
type Service struct{ /* pool, defs */ }
func NewService(pool *pgxpool.Pool, defs *definitions.Store) *Service
func (s *Service) SetCreatedHook(h CreatedHook)     // Plan 04 registers onSubmit action enqueueing
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, formSlug string, data map[string]any, requirePublic bool) (Submission, string /*receiptToken*/, error)
func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (Submission, error)
func (s *Service) GetForUpdate(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (Submission, error) // SELECT ... FOR UPDATE
func (s *Service) List(ctx context.Context, orgID uuid.UUID, f ListFilter) ([]Submission, string /*nextCursor*/, error)
func (s *Service) Events(ctx context.Context, orgID, id uuid.UUID) ([]Event, error)
func (s *Service) PublicStatus(ctx context.Context, id uuid.UUID, token string) (PublicStatus, error) // ErrNotFound on bad token
func (s *Service) ExportCSV(ctx context.Context, orgID uuid.UUID, formSlug string, w io.Writer) error
func InsertEvent(ctx context.Context, q db.DBTX, e Event) (Event, error)
```
State for a form without workflow: `"submitted"` (label "Submitted", terminal true). Receipt token: 24 random bytes base64url; stored as sha256 hex.

### 6.8 `internal/jobs` (Plan 04)
```go
type Job struct { ID int64; OrgID uuid.UUID; Kind string; Payload json.RawMessage; Status string; Attempts, MaxAttempts int; RunAt time.Time; LastError string; CreatedAt, UpdatedAt time.Time }
type Handler func(ctx context.Context, job Job) error
type Queue struct{ /* pool, handlers, clock */ }
func NewQueue(pool *pgxpool.Pool) *Queue
func (q *Queue) Register(kind string, h Handler)
func (q *Queue) Enqueue(ctx context.Context, tx db.DBTX, orgID uuid.UUID, kind string, payload any) (int64, error)
func (q *Queue) RunOnce(ctx context.Context) (processed bool, err error)   // claim ≤1 due job (FOR UPDATE SKIP LOCKED), run, record
func (q *Queue) Run(ctx context.Context, concurrency int)                   // loops RunOnce; idle poll 1s; returns when ctx done
func (q *Queue) List(ctx context.Context, orgID uuid.UUID, status string, limit int) ([]Job, error)
func (q *Queue) Retry(ctx context.Context, orgID uuid.UUID, id int64) error // failed → pending, attempts=0, run_at=now
```
Backoff after failure: `min(5s * 2^(attempts-1), 1h)`; after `max_attempts` → `failed`. Handlers returning `jobs.Permanent(err)` fail immediately. A claimed job sets `status=running, locked_until=now()+5m`; running jobs whose lock expired are reclaimable.

### 6.9 `internal/actions` (Plan 04)
```go
const (KindWebhook = "action.webhook"; KindEmail = "action.email"; KindAssign = "action.assign")
type Payload struct { SubmissionID uuid.UUID `json:"submissionId"`; Trigger string `json:"trigger"` /* "submit"|transition key */; EventID int64 `json:"eventId"`; Action definition.Action `json:"action"` }
type Mailer interface { Send(ctx context.Context, to, subject, body string) error }
func NewMailer(cfg config.Config) Mailer            // SMTP when SMTPHost set, else log mailer
func Render(tmpl string, vars map[string]any) string // {{a.b.c}} dotted lookup; missing → ""; no HTML escaping (plain-text email)
func TemplateVars(cfg config.Config, form definition.Form, sub submissions.Submission, transition *definition.Transition) map[string]any
type Deps struct { Config config.Config; Subs *submissions.Service; Defs *definitions.Store; Auth *auth.Service; Mailer Mailer; HTTPClient *http.Client; Pool *pgxpool.Pool }
func Register(q *jobs.Queue, d Deps)
func Enqueue(ctx context.Context, q *jobs.Queue, tx db.DBTX, orgID uuid.UUID, p Payload) error // chooses kind from p.Action.Type
```
Template variables: `submission.id`, `submission.state`, `submission.stateLabel`, `submission.data.<key>`, `submission.fields.<key>`, `submission.url` (`<BaseURL>/admin/submissions/<id>`), `submission.statusUrl` is NOT available (token is not stored), `form.slug`, `form.title`, `transition.key`, `transition.label`, `baseUrl`.
Webhook: `POST url`, `Content-Type: application/json`, timeout 10s, header `X-OpenForms-Event: submission.created|submission.transitioned`, header `X-OpenForms-Delivery: <job id>`, and when `WebhookSecret` set `X-OpenForms-Signature: sha256=<hex HMAC-SHA256(secret, body)>`. Body: `{"event": "...", "submission": <Submission JSON §7.3>, "transition": {"key","label","from","to"} | null, "form": {"slug","title"}}`. Non-2xx → retryable error; 4xx other than 408/429 → `jobs.Permanent`.
Assign by role: among users having the role (or `admin` if none), pick the one with the fewest assigned non-terminal submissions (ties → earliest created). Assign by user: email lookup; unknown → `jobs.Permanent`. Each successful action writes an `action_succeeded` event; a job that ends `failed` writes `action_failed` with `{"error": "...", "action": {...}}`. Assign writes an `assigned` event.

### 6.10 `internal/workflow` (Plan 04)
```go
var (ErrUnknownTransition, ErrInvalidState, ErrForbidden, ErrNoWorkflow, ErrStateConflict error)
type AvailableTransition struct {
    Key, Label, To, ToLabel string; RequireFields []string
    Allowed bool; Reason string   // Reason: "" | "role" (principal lacks role)
}
type TransitionInput struct { Transition string; Fields map[string]any; Comment string; ExpectedState string /* optional optimistic check */ }
type Engine struct{ /* pool, defs, subs, auth, queue */ }
func NewEngine(pool *pgxpool.Pool, defs *definitions.Store, subs *submissions.Service, authSvc *auth.Service, q *jobs.Queue) *Engine
func (e *Engine) Available(ctx context.Context, p auth.Principal, sub submissions.Submission) ([]AvailableTransition, error) // transitions whose From includes current state
func (e *Engine) Transition(ctx context.Context, p auth.Principal, id uuid.UUID, in TransitionInput) (submissions.Submission, error)
func (e *Engine) UpdateFields(ctx context.Context, p auth.Principal, id uuid.UUID, fields map[string]any) (submissions.Submission, error)
func (e *Engine) Comment(ctx context.Context, p auth.Principal, id uuid.UUID, body string) (submissions.Event, error)
func (e *Engine) Assign(ctx context.Context, p auth.Principal, id uuid.UUID, userID *uuid.UUID) (submissions.Submission, error)
func (e *Engine) OnCreated(ctx context.Context, tx pgx.Tx, sub submissions.Submission, wf *definition.Workflow) error // enqueue onSubmit actions; registered via subs.SetCreatedHook
```
Transition algorithm (single tx): lock row → resolve pinned workflow → unknown key ⇒ `ErrUnknownTransition` → `ExpectedState` mismatch ⇒ `ErrStateConflict` → current state ∉ `From` ⇒ `ErrInvalidState` → `!p.HasAnyRole(guard.Roles...)` ⇒ `ErrForbidden` → merge `in.Fields` into submission fields via `ValidateWorkflowFields` → any `RequireFields` empty ⇒ `*definition.ValidationError` (paths `fields.<key>`, message "required for transition <key>") → update state/fields/updated_at → insert `transition` event (payload `{"comment": ..., "fields": <changed fields>}`) → enqueue each action with `Trigger = transition key`, `EventID` = event id → commit.

### 6.11 `internal/httpapi`
```go
type Deps struct {
    Config config.Config; Auth *auth.Service; Defs *definitions.Store; Subs *submissions.Service
    Engine *workflow.Engine /* nil until Plan 04 */; Queue *jobs.Queue /* nil until Plan 04 */
    Web http.Handler /* webui handler, may be nil */; OrgID uuid.UUID
}
type APIError struct { Status int; Code, Message string; Details []definition.Problem }
func (e *APIError) Error() string
func NewRouter(d Deps) http.Handler
func WriteJSON(w http.ResponseWriter, status int, v any)
func WriteError(w http.ResponseWriter, r *http.Request, err error)   // maps known errors (table in §7.1); unknown → 500 "internal" and logs
func DecodeJSON(r *http.Request, v any) error                          // 1 MiB limit; malformed → APIError 400 bad_request
```
Route registration is split by file; each plan adds a `mountX(r chi.Router, d Deps)` function and one line in `routes.go`.
Middleware and request helpers (Plan 01, exported for use by every handler file):
```go
func Authenticate(a *auth.Service) func(http.Handler) http.Handler // reads Bearer key or of_session cookie; sets principal if valid; never rejects
func RequireAuth(next http.Handler) http.Handler                    // 401 unauthenticated when no principal
func RequireAdmin(next http.Handler) http.Handler                   // 401 when none, 403 forbidden when not admin
func MustPrincipal(r *http.Request) auth.Principal                  // panics if absent (only behind RequireAuth)
func URLParamUUID(r *http.Request, name string) (uuid.UUID, error)  // 404 not_found APIError on malformed
```
`routes.go` layout: `/healthz` at root; `/api/v1` subrouter with `Authenticate` applied; public groups mounted without `RequireAuth`; authenticated groups inside `r.Group(func(r chi.Router){ r.Use(RequireAuth); ... })`. Mount functions: `mountAuth`, `mountAdminUsers`, `mountAPIKeys` (Plan 01); `mountDefinitions`, `mountPublic`, `mountSubmissions` (Plan 03); `mountWorkflow`, `mountJobs` (Plan 04); `mountPublicConfig` (Plan 09).

### 6.12 `internal/webui`
```go
//go:embed all:dist
var dist embed.FS
func Handler() http.Handler
```
Routes: `/_app/*` → static files from `dist/` (long cache for `assets/`); `/admin`, `/admin/*` → `dist/admin/index.html`; `/f/*`, `/s/*` → `dist/hosted/index.html`; `/demo`, `/demo/*` → `dist/demo/index.html`; `/embed.js` → `dist/embed/embed.js`. Missing build → 503 text "web UI not built; run `make web`".

### 6.13 `internal/app`
```go
type App struct { Config config.Config; Pool *pgxpool.Pool; Auth *auth.Service; Defs *definitions.Store; Subs *submissions.Service;
                  Queue *jobs.Queue /* Plan 04 */; Engine *workflow.Engine /* Plan 04 */; OrgID uuid.UUID; Handler http.Handler }
func New(ctx context.Context, cfg config.Config) (*App, error)   // open pool, migrate, ensure org, build services + router
func (a *App) Run(ctx context.Context) error                     // HTTP server + worker (Plan 04) until ctx done; graceful shutdown 10s
func (a *App) Close()
```

### 6.14 `internal/client` (Plan 05)
Go client mirroring the API endpoints the CLI needs: `New(baseURL, apiKey string) *Client`; `Validate(ctx, Bundle) (ValidateResult, error)`; `Apply(ctx, Bundle, dryRun bool) (definitions.ApplyResult-shaped JSON, error)`; `Export(ctx) (Bundle, error)`. `Bundle{Forms []definition.Form; Workflows []definition.Workflow}`. API errors surface as `*client.Error{Status, Code, Message, Details}`.

### 6.15 `internal/testutil` (Plan 01)
```go
type Env struct { Pool *pgxpool.Pool; Auth *auth.Service; OrgID uuid.UUID; Config config.Config }
func NewEnv(t testing.TB) *Env                                   // dbtest.New + EnsureDefaultOrg + test config (BaseURL http://test.local)
func (e *Env) APIKey(t testing.TB, roles ...string) string        // creates key with roles, returns plaintext
func (e *Env) User(t testing.TB, email string, roles ...string) auth.User // password "password123"
func Do(t testing.TB, h http.Handler, method, path, apiKey string, body any) *httptest.ResponseRecorder // JSON body; apiKey "" = anonymous
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T
func ErrorCode(t testing.TB, rec *httptest.ResponseRecorder) string // error.code from envelope
```

## 7. HTTP API (`/api/v1`)

### 7.1 Conventions
- Auth: `Authorization: Bearer ofk_...` (API key) **or** cookie `of_session`. Unauthenticated access only to `/public/*`, `/auth/login`, `/healthz`.
- Error envelope: `{"error": {"code": "...", "message": "...", "details": [{"path","message"}]}}` (`details` omitted when empty).

| Go error | Status | code |
|---|---|---|
| `*APIError` | its Status | its Code |
| `*definition.ValidationError` | 422 | `validation_failed` (details = problems) |
| `auth.ErrUnauthenticated` | 401 | `unauthenticated` |
| `auth.ErrInvalidCredentials` | 401 | `invalid_credentials` |
| `auth.ErrEmailTaken` | 409 | `email_taken` |
| `*.ErrNotFound`, `submissions.ErrFormNotPublic` | 404 | `not_found` |
| `workflow.ErrForbidden` | 403 | `forbidden` |
| `workflow.ErrUnknownTransition` | 422 | `unknown_transition` |
| `workflow.ErrInvalidState` | 409 | `invalid_state` |
| `workflow.ErrStateConflict` | 409 | `state_conflict` |
| `workflow.ErrNoWorkflow` | 409 | `no_workflow` |
| rate limit | 429 | `rate_limited` |
| other | 500 | `internal` |

- Admin-only endpoints return 403 `forbidden` for non-admin principals.
- Cursor pagination: `?cursor=&limit=`; responses `{"items": [...], "nextCursor": "..." | null}`.

### 7.2 Endpoints

| Method & path | Auth | Plan | Notes |
|---|---|---|---|
| GET `/healthz` (root, not /api/v1) | none | 01 | `{"status":"ok"}` after DB ping |
| POST `/auth/login` | none | 01 | `{email,password}` → sets cookie, `{user}` |
| POST `/auth/logout` | session | 01 | 204 |
| GET `/auth/me` | any | 01 | `{"principal": {kind,id,name,email,roles}}` |
| GET/POST `/users`, PATCH/DELETE `/users/{id}` | admin | 01 | User JSON `{id,email,name,roles,createdAt}`; POST `{email,name,password,roles}` |
| GET/POST `/api-keys`, DELETE `/api-keys/{id}` | admin | 01 | POST `{name,roles}` → `{apiKey: {...}, key: "ofk_..."}` (plaintext shown once) |
| GET `/forms` | any | 03 | `{"items":[FormSummary]}`; FormSummary `{slug,title,workflow,public,version,source,updatedAt,submissionCount}` |
| GET `/forms/{slug}` | any | 03 | `{"form": FormRecord JSON}` = `{slug,version,source,updatedAt,workflowVersion: int|null, definition}` |
| PUT `/forms/{slug}` | admin | 03 | body = form definition (slug must match path) + optional `?source=ui`; → `{"item": ApplyItem}` |
| GET `/forms/{slug}/versions` | any | 03 | `{"items":[{version,hash,source,createdBy,createdAt}]}` |
| GET `/forms/{slug}/versions/{n}` | any | 03 | `{"form": FormRecord JSON}` |
| GET `/forms/{slug}/submissions.csv` | any | 03 | CSV download |
| GET `/workflows`, `/workflows/{slug}`, PUT `/workflows/{slug}`, GET `/workflows/{slug}/versions[/{n}]` | as forms | 03 | WorkflowSummary `{slug,title,version,source,updatedAt,stateCount}` |
| POST `/definitions/validate` | admin | 03 | body Bundle `{forms:[raw objects], workflows:[raw objects]}` → 200 `{"valid":true}` or 422 |
| POST `/definitions/apply` | admin | 03 | Bundle + `?dryRun=true` + `?source=cli` → `{"items":[ApplyItem]}` |
| GET `/definitions` | admin | 03 | Bundle export |
| GET `/public/forms/{slug}` | none | 03 | `{"form": definition}` only if `settings.public` |
| POST `/public/forms/{slug}/submissions` | none | 03 | `{"data":{...}}` → 201 `{"id","state","stateLabel","receiptToken","confirmationMessage"}`; rate limit 20/min/IP |
| GET `/public/submissions/{id}?token=` | none | 03 | PublicStatus JSON `{id,formTitle,state,stateLabel,terminal,states:[State],history:[{state,label,at}],createdAt}` |
| GET `/submissions` | any | 03 | filters `form,state,assignee (uuid or "me" or "none")` + cursor |
| GET `/submissions/{id}` | any | 03/04 | `{"submission", "form": definition, "workflow": definition|null, "events":[Event], "transitions":[AvailableTransition]}` (`transitions` = [] until Plan 04) |
| POST `/submissions` | any | 03 | authenticated (non-public) create: `{"form": slug, "data": {...}}` → 201 `{"submission", "receiptToken"}` |
| POST `/submissions/{id}/transitions` | any | 04 | `{transition, fields?, comment?, expectedState?}` → `{"submission"}` |
| PATCH `/submissions/{id}/fields` | any | 04 | `{"fields": {...}}` → `{"submission"}` |
| POST `/submissions/{id}/comments` | any | 04 | `{"body"}` → 201 `{"event"}` |
| PUT `/submissions/{id}/assignee` | any | 04 | `{"userId": uuid|null}` → `{"submission"}` |
| GET `/jobs?status=` | admin | 04 | `{"items":[Job JSON]}` |
| POST `/jobs/{id}/retry` | admin | 04 | 204 |

### 7.3 JSON shapes
- **Submission:** `{id, form: slug, formVersion, state, stateLabel, terminal, data, fields, assignee: {id,name,email}|null, createdAt, updatedAt}`
- **Event:** `{id, type, fromState|null, toState|null, transition|null, actor: {type,id|null,name}, payload, createdAt}`
- **AvailableTransition:** `{key, label, to, toLabel, requireFields, allowed, reason}`
- **ApplyItem:** `{kind, slug, version, changed, created}`
- **Job:** `{id, kind, status, attempts, maxAttempts, runAt, lastError, payload, createdAt, updatedAt}`
- **Principal:** `{kind, id, name, email, roles}`

## 8. CLI (`openforms`)

| Command | Plan | Behaviour |
|---|---|---|
| `serve` | 01 | load config → `app.New` → `Run` |
| `migrate` | 01 | run migrations and exit |
| `admin create-user --email --name --password --roles r1,r2` | 05 | direct DB via `auth.Service` |
| `admin create-api-key --name --roles r1,r2` | 05 | prints plaintext key once |
| `init [dir]` | 05 | writes `openforms.yaml` (`server: http://localhost:8080`, `dir: openforms`) and `openforms/forms/contact.yaml`, `openforms/workflows/contact-triage.yaml`; refuses to overwrite existing files |
| `validate [--dir]` | 05 | offline: parse every `forms/*.y{a,}ml|*.json` and `workflows/*`; `ValidateBundle(…, nil)`; prints `file:path: message`; exit 1 on problems |
| `push [--dir] [--dry-run]` | 05 | validate locally, then `POST /definitions/apply?source=cli`; prints table `KIND SLUG VERSION STATUS(created/updated/unchanged)` |
| `pull [--dir] [--check]` | 05 | `GET /definitions`, writes canonical YAML per slug; `--check` writes nothing and exits 1 if any file would change (CI drift check) |
| `diff [--dir]` | 05 | per slug: `+ local only`, `- remote only`, `~ differs`, unchanged omitted |
| `seed --demo` | 09 | apply `examples/openforms` bundle (embedded) with source `seed`, create demo users (§10) if absent; idempotent |

Config resolution for remote commands: flags `--server`, `--api-key` > env `OPENFORMS_URL`, `OPENFORMS_API_KEY` > `openforms.yaml` in cwd. File naming: `<dir>/forms/<slug>.yaml`, `<dir>/workflows/<slug>.yaml`; a file's `slug` must equal its basename (validate reports mismatch).

## 9. Frontend

### 9.1 `@openforms/sdk` (Plan 06)
- `src/generated/*.ts`: types generated from `schemas/*.schema.json` via `json-schema-to-typescript` (`pnpm -C web/packages/sdk gen`); exported as `FormDefinition`, `WorkflowDefinition`, `Field`, `State`, `Transition`, `Action`, etc.
- `src/api-types.ts`: hand-written types for §7.3 shapes: `Submission`, `SubmissionEvent`, `AvailableTransition`, `ApplyItem`, `Job`, `Principal`, `User`, `ApiKey`, `FormSummary`, `WorkflowSummary`, `FormRecord`, `WorkflowRecord`, `VersionInfo`, `PublicStatus`, `Page<T>`, `SubmissionDetail`.
- `src/client.ts`: `class OpenFormsClient { constructor(opts: {baseUrl?: string; apiKey?: string; fetch?: typeof fetch}) }` with one method per endpoint in §7.2 (names: `getPublicForm, submitPublic, getPublicStatus, login, logout, me, listForms, getForm, putForm, formVersions, formVersion, listWorkflows, getWorkflow, putWorkflow, workflowVersions, workflowVersion, validateDefinitions, applyDefinitions, exportDefinitions, listSubmissions, getSubmission, createSubmission, transition, updateFields, comment, assign, csvUrl, listUsers, createUser, updateUser, deleteUser, listApiKeys, createApiKey, revokeApiKey, listJobs, retryJob`). Uses `credentials: "include"`. Throws `OpenFormsError { status; code; message; details: Problem[] }`.
- `src/logic.ts`: `visible(form, data): Record<string, boolean>`, `validateSubmission(form, data): { clean: Record<string, unknown> | null; problems: Problem[] }`, passing `schemas/fixtures/*.json`.

### 9.2 `@openforms/react` (Plan 06)
- `useOpenForm({ client, slug } | { definition, onSubmit })` → `{ form, status: "loading"|"ready"|"submitting"|"submitted"|"error", values, setValue(key, v), visible, errors: Record<string,string>, submit(), result, loadError }`.
- `<OpenForm client slug? definition? onSubmitted?(result) components? className? />` renders fields with default components (`TextInput, TextArea, EmailInput, NumberInput, Select, MultiSelect, Checkbox, DateInput, UrlInput`), overridable via `components` map keyed by field type. Accessible: label `for`, `aria-invalid`, `aria-describedby` for help/errors, error summary focus on failed submit.
- `@openforms/react/styles.css`: styles scoped under `.of-form` using CSS custom properties (`--of-accent`, `--of-radius`, `--of-font`, …).
- `<StatusTracker client submissionId token pollMs? />`: renders state chain + history from PublicStatus; polls (default 5000 ms) until terminal.

### 9.3 Hosted app (Plan 06) — `/f/:slug`, `/s/:id?token=`
- `/f/:slug`: title, description, `<OpenForm>`; on success shows confirmation message and a link "Track your submission" → `/s/:id?token=…`.
- `?embed=1`: no page chrome, transparent background; posts `{type:"openforms:resize", height}` on size change and `{type:"openforms:submitted", id, state}` to `window.parent`.
- `/s/:id`: `<StatusTracker>`.
- 404 / not-public → friendly "This form isn't available" page.

### 9.4 Embed (Plan 06) — `/embed.js`
`<div data-openforms="job-application"></div><script src="https://host/embed.js" async></script>` → injects an iframe `https://host/f/<slug>?embed=1` (origin derived from the script `src`), width 100%, auto-resized by `openforms:resize` messages from that origin only; re-dispatches `openforms:submitted` as a `CustomEvent("openforms:submitted", {detail})` on the container div.

### 9.5 Admin app (Plans 07, 08) — `/admin/*`
Routes: `/admin/login`, `/admin` (→ inbox), `/admin/submissions?form&state&assignee`, `/admin/submissions/:id`, `/admin/forms`, `/admin/forms/new`, `/admin/forms/:slug` (overview: versions, links, CSV), `/admin/forms/:slug/edit`, `/admin/workflows`, `/admin/workflows/new`, `/admin/workflows/:slug/edit`, `/admin/users`, `/admin/api-keys`, `/admin/jobs`.
- **Inbox:** filters (form, state from the selected form's workflow, assignee: anyone/me/unassigned), table (created, form, summary = first two text field values, state badge with workflow color, assignee), cursor "Load more", auto-refresh every 30s.
- **Submission detail:** answers (labels from pinned form version), workflow fields panel (editable, saves via PATCH), transition buttons (disabled with tooltip when `allowed=false`); clicking a transition with `requireFields` opens a dialog to fill them (+ optional comment); assignee picker; comment box; timeline of events (icons per type, actor, relative time). Handles 409 `state_conflict` by refetching and showing "This submission changed while you were viewing it."
- **Users / API keys / Jobs:** admin-only tables with create/edit/revoke/retry; API key plaintext shown once in a copyable dialog.
- **Form editor (Plan 08):** three-pane: field list (add, reorder via up/down, duplicate, delete), inspector for the selected field (all properties incl. options, validation, showIf builder restricted to earlier fields), live preview (`<OpenForm definition>`); tabs "Visual" / "YAML" (YAML editable, round-trips through `yaml`); client-side validation with ajv + server 422 details mapped to inspector fields by `path`; Save → `PUT /forms/{slug}?source=ui`. Banner when the latest version's `source` is `cli`: "This form is managed in code. Changes made here will be overwritten by the next `openforms push` unless you run `openforms pull`."
- **Workflow editor (Plan 08):** diagram (React Flow, dagre left-to-right layout, nodes colored by state color, terminal nodes double border, edges labeled with transition label; click selects), side inspector for states / transitions (from multi-select, to, guard roles tag input, requireFields multi-select, actions list editor per type) / workflow fields / onSubmit actions; YAML tab; same validation & save flow; same code-managed banner.
- **Version history (Plan 08):** on form/workflow overview: list versions, pick two → side-by-side YAML line diff (`diff` npm package), "Restore this version" (PUT that definition with `source=ui`).

**Admin extension points (Plan 07 provides, Plan 08 extends):** `src/routes.tsx` → `AdminRoute = {path; element; adminOnly?}`, `routes: AdminRoute[]`; `src/nav.ts` → `NavItem = {to; label; adminOnly?}`, `navItems`; `src/extensions/OverviewExtras.tsx` → `OverviewExtras({kind: "form"|"workflow"; slug})` (null in Plan 07, version history in Plan 08) rendered on `forms/:slug` and `workflows/:slug` overview pages; test harness `src/test/render.tsx` `renderWithProviders(ui, {route?, path?})`, `src/test/server.ts` (MSW `server`), `src/test/fixtures.ts` (`makeSubmission, makeEvent, makeFormRecord, makeWorkflowRecord, makeFormDefinition, makeWorkflowDefinition, makeUser, makeApiKey, makeJob, makePrincipal`); `src/api.ts` singleton `client`; `src/queryKeys.ts` `qk`.

### 9.6 Demo page (Plan 09) — `/demo`
Served when the demo bundle is seeded (the page itself is always built; if `/api/v1/public/forms/job-application` 404s it shows "Run `openforms seed --demo` to enable the live demo"). Sections:
1. **Hero:** one-sentence pitch, links to GitHub/docs, "Try it below".
2. **Define:** the `job-application.yaml` and `hiring.yaml` sources (imported as raw strings from `examples/`) with syntax highlighting (`shiki`), tabs.
3. **Collect:** live `<OpenForm client slug="job-application">` (headless SDK).
4. **Track & review:** after submitting, `<StatusTracker>` for the new submission plus a "Play the reviewer" panel: logs in via SDK as `reviewer@demo.local` (demo mode only) and shows that submission's available transitions as buttons (requireFields rendered as inputs); performing them updates the tracker live. Link "Open in the admin inbox".
5. **Embed:** live iframe embed of `contact` via `embed.js` + copyable snippet.
6. **CLI:** static code block of `openforms init / validate / push`.

## 10. Demo & packaging (Plan 09)
- Demo users (created by `seed --demo`, password `demo1234`): `admin@demo.local` [admin], `reviewer@demo.local` [reviewer], `manager@demo.local` [hiring-manager]. `/admin/login` shows these credentials when `GET /api/v1/public/config` returns `{"demo": true}` (Plan 09 adds this public endpoint).
- Dockerfile: stage 1 `node:22` → `pnpm install --frozen-lockfile && pnpm -C web build`; stage 2 `golang:1.24` → `go build -o /openforms ./cmd/openforms` with dist embedded; stage 3 `gcr.io/distroless/static-debian12` running `openforms serve`.
- `docker compose --profile app up` runs postgres + mailpit + openforms (demo mode, seeds on start via `OPENFORMS_SEED_DEMO=true`).
- Makefile targets: `dev-db`, `test` (go test ./...), `web` (pnpm build), `web-test`, `build`, `e2e`, `lint`.
- CI (GitHub Actions): Go tests with Postgres service, web tests, build, Playwright e2e.
- Docs: `README.md`, `docs/getting-started.md`, `docs/definitions.md`, `docs/workflows.md`, `docs/api.md`, `docs/cli.md`, `docs/self-hosting.md`.

## 11. Error handling & edge cases (cross-cutting)
- Concurrent transitions on the same submission: row lock + optional `expectedState`; the loser gets 409.
- Actions never run inside the HTTP request; they are enqueued in the same transaction as the state change (transactional outbox), so a rolled-back transition never fires actions.
- Webhook receivers must tolerate duplicate deliveries (at-least-once); `X-OpenForms-Delivery` is the idempotency key.
- Deleting a user sets `assignee_id` NULL; events keep `actor_name`.
- Public submission payloads >1 MiB → 413-mapped `bad_request`; unknown fields silently dropped.
- Form un-published (`public: false`) → public GET/POST return 404; existing status links keep working.
- A form referencing a workflow can't be applied if the workflow is missing (bundle validation).
- Editing a definition in the UI while the CLI owns it is allowed but warned (sync model: API is source of truth; `pull --check` in CI catches drift).

## 12. Testing strategy
- Go: table-driven unit tests for `definition` (incl. fixtures), DB-backed tests for stores/services/engine via `dbtest`, `httptest` handler tests per endpoint group, queue tests using `RunOnce` and an injectable clock, webhook tests with `httptest.Server`, email via a fake `Mailer`.
- TS: Vitest unit tests (SDK logic incl. fixtures, client with mocked fetch), React Testing Library + MSW for components and admin pages.
- E2E (Playwright, against `docker compose` stack + seeded demo): hosted submit → status page; admin login → transition with required field → status page updates; email visible in Mailpit API; webhook received by a local sink; demo page reviewer loop; form editor save creates a new version.

## 13. Plan index

| Plan | Scope | Depends on |
|---|---|---|
| 01 Foundation | repo scaffold, compose, Makefile, config, db + dbtest, migration 00001, auth service, httpapi skeleton (errors, middleware, auth/users/api-keys routes, healthz), webui handler, app wiring, CLI `serve`/`migrate` | none |
| 02 Definition language | `schemas/` JSON Schemas + conformance fixtures, `internal/definition` (types, parsing, semantic validation, canonical hashing, visibility, submission & workflow-field validation) | none |
| 03 Definitions & submissions API | migration 00002, `internal/definitions` Store, `internal/submissions` Service, rate limiter, forms/workflows/definitions/public/submissions routes | 01, 02 |
| 04 Workflow engine & actions | migration 00003, `internal/jobs`, `internal/actions`, `internal/workflow`, transition/fields/comments/assignee/jobs routes, worker in `app.Run` | 03 |
| 05 CLI | `internal/client`, `admin`, `init`, `validate`, `push`, `pull`, `diff` commands | 02 (offline cmds), 03 (remote cmds) |
| 06 SDK, renderer, hosted, embed | `web/` workspace, `@openforms/sdk`, `@openforms/react`, hosted app, embed.js | 02 (schemas/fixtures); API contract §7 (mocked with MSW) |
| 07 Admin app | admin shell, auth, inbox, submission detail, forms/workflows overview, users, API keys, jobs | 06 |
| 08 Visual editors | form editor, workflow editor + diagram, version history & diff | 07 |
| 09 Demo, packaging, docs, E2E | examples bundle, `seed --demo`, `/public/config`, demo page, Dockerfile, compose app profile, CI, docs, Playwright E2E | all |

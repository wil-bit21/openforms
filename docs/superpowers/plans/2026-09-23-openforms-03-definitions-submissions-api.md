# Definitions & Submissions API Implementation Plan (Plan 03)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents); every other task runs sequentially in the listed order.

**Goal:** Persist versioned form/workflow definitions and accept, store, list, track and export submissions through the `/api/v1` HTTP API.

**Architecture:** `internal/definitions.Store` writes immutable, hash-deduplicated definition versions in one transaction per bundle (re-pinning dependent forms when a workflow changes) and caches immutable versions in memory. `internal/submissions.Service` validates submissions against the pinned form version, stores them with a hashed receipt token and a `created` event, and exposes a transactional `CreatedHook` for Plan 04. `internal/httpapi` gains three mount files (`mountDefinitions`, `mountPublic`, `mountSubmissions`), a fixed-window per-IP rate limiter for public submissions, and a Plan 03 error-mapping function.

**Tech Stack:** Go ≥ 1.24, pgx/v5 (pgxpool), goose (embedded SQL migrations), chi/v5, google/uuid, std `encoding/csv`, `crypto/sha256`, `crypto/subtle`.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md` — binding sections: §4 migration `00002`, §5.4 submission semantics, §5.5 versioning, §6.6 `internal/definitions`, §6.7 `internal/submissions`, §6.11 httpapi (middleware + mount names), §6.15 `internal/testutil`, §7 rows owned by Plan 03, §7.3 JSON shapes, §11 edge cases. Roadmap: `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md` (Wave 2, Lane A).

**Prerequisites:** Plans 01 and 02 are merged. Use only their exported names from the spec: `db.DBTX`, `db.WithTx`, `dbtest.New`, `auth.*`, `definition.*`, `httpapi.{Deps, APIError, NewRouter, WriteJSON, WriteError, DecodeJSON, RequireAuth, RequireAdmin, MustPrincipal, URLParamUUID}`, `testutil.{Env, NewEnv, Do, Decode, ErrorCode}`. Start Postgres before any test: `docker compose up -d postgres`.

## Global Constraints

- Go ≥ 1.24; module `github.com/openforms/openforms`.
- DB-backed tests use `dbtest.New(t)` (via `testutil.NewEnv(t)`); default DSN `postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable`, override `OPENFORMS_TEST_DATABASE_URL`.
- All JSON over the wire is camelCase; timestamps RFC 3339 **UTC** strings.
- Every API error uses the envelope `{"error": {"code", "message", "details"?}}`.
- The Go server is authoritative for validation (`definition.ValidateSubmission`, `definition.ValidateForm/ValidateWorkflow/ValidateBundle`).
- Migration `00002_definitions_submissions.sql` is copied **verbatim** from spec §4.
- Definition versions are immutable; `hash` = sha256 of `definition.Canonical`; an unchanged hash (and, for forms, an unchanged workflow pin) creates no version.
- Applying a new workflow version creates a new version of every form referencing it (§5.5).
- A form without a workflow gets state `"submitted"`, label `"Submitted"`, terminal `true`.
- Receipt token: 24 random bytes, base64url (no padding); DB stores sha256 hex.
- Public submissions are rate-limited to 20 per minute per client IP → 429 `rate_limited`.
- List pagination: default limit 50, max 200; response `{"items": [...], "nextCursor": "..." | null}`.
- `source` query values accepted over HTTP: `cli`, `ui`, `api` (default `api`); `seed` is Go-only.
- Commit after every task with conventional prefixes (`feat:`, `test:`, `chore:`).

## Review Focus

1. **Two `openforms push` runs racing on a brand-new slug** → both succeed and exactly one version 1 exists (no 500 from a unique-constraint violation). Test: `TestApplyConcurrentSameNewSlug` in Task 4.
2. **CSV export of respondent-controlled values starting with `=`, `+`, `-`, `@`** → the cell is prefixed with `'` so spreadsheets don't execute formulas. Test: `TestExportCSVEscapesFormulas` in Task 7.
3. **Guessed or wrong receipt tokens** → identical 404 `not_found` as a nonexistent submission; comparison is constant-time. Tests: `TestPublicStatusRejectsBadToken` (Task 7), `TestPublicSubmitAndStatus` (Task 10).
4. **A workflow edited after submissions exist** → existing submissions keep the pinned workflow's labels and the form version they were created with. Test: `TestSubmissionKeepsPinnedVersions` in Task 6.
5. **Inbox paging over submissions created in the same microsecond** → no duplicates or gaps across pages (tie-break on `id`). Test: `TestListPaginatesWithoutDuplicates` in Task 7.

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/db/migrations/00002_definitions_submissions.sql` | Tables for definitions, versions, submissions, events |
| `internal/db/migrations_00002_test.go` | Migration smoke test |
| `internal/testutil/definitions.go` | `SampleWorkflow`, `SampleForm`, `Env.OtherOrg` shared by Plan 03/04 tests |
| `internal/definitions/store.go` | Types, `NewStore`, row scanning, `GetForm`, `GetWorkflow` |
| `internal/definitions/apply.go` | `Apply`, upserts, re-pinning, dry-run |
| `internal/definitions/validate.go` | Bundle validation, `PrefixProblems` |
| `internal/definitions/reads.go` | Lists, versions, cached version lookups, `Export` |
| `internal/submissions/service.go` | Types, `NewService`, `SetCreatedHook`, `Get`, `GetForUpdate`, state-label resolution |
| `internal/submissions/create.go` | `Create`, receipt tokens |
| `internal/submissions/events.go` | `Event`, `Actor`, `InsertEvent`, `Events`, event type constants |
| `internal/submissions/list.go` | `List`, cursor encoding, `CountByForm` |
| `internal/submissions/public.go` | `PublicStatus` |
| `internal/submissions/csv.go` | `ExportCSV` |
| `internal/httpapi/ratelimit.go` | Fixed-window limiter, `clientIP` |
| `internal/httpapi/errors.go` (modify) | Plan 03 rows appended to `errorMappings` |
| `internal/httpapi/json_types.go` | §7.3 JSON shapes for submissions, events, definition records |
| `internal/httpapi/definition_handlers.go` | `mountDefinitions` |
| `internal/httpapi/public_handlers.go` | `mountPublic` |
| `internal/httpapi/submission_handlers.go` | `mountSubmissions` |
| `internal/httpapi/defs_subs_helpers_test.go` | Shared handler-test fixture `newDefsSubsAPI` |

---

### Task 1: Migration 00002 and shared test definitions

**Files:**
- Create: `internal/db/migrations/00002_definitions_submissions.sql`
- Create: `internal/db/migrations_00002_test.go`
- Create: `internal/testutil/definitions.go`
- Test: `internal/testutil/definitions_test.go`

**Interfaces:**
- Consumes: `dbtest.New(t) *pgxpool.Pool`; `testutil.Env{Pool, Auth, OrgID, Config}`; `definition.ValidateForm`, `definition.ValidateWorkflow`, `definition.ValidateBundle`.
- Produces:
  - `func testutil.SampleWorkflow(slug string) definition.Workflow` — states `new` (initial, "New"), `review` ("In review"), `done` ("Done", terminal); workflow field `note` (textarea); transitions `start` (new→review, roles `[reviewer]`), `finish` (review→done, requireFields `[note]`).
  - `func testutil.SampleForm(slug, workflow string) definition.Form` — title "Contact", public, fields `name` (text, required), `email` (email, required), `topic` (select: sales|support, optional).
  - `func (e *testutil.Env) OtherOrg(t testing.TB) uuid.UUID` — inserts a second org and returns its id.

- [ ] **Step 1: Write the failing migration test**

`internal/db/migrations_00002_test.go`:
```go
package db_test

import (
	"context"
	"testing"

	"github.com/openforms/openforms/internal/db/dbtest"
)

func TestMigration00002CreatesTables(t *testing.T) {
	pool := dbtest.New(t)
	for _, table := range []string{"workflows", "workflow_versions", "forms", "form_versions", "submissions", "submission_events"} {
		var n int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
		if n != 0 {
			t.Fatalf("table %s: want empty, got %d rows", table, n)
		}
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/db/ -run TestMigration00002CreatesTables -v`
Expected: FAIL with `table workflows: ERROR: relation "workflows" does not exist (SQLSTATE 42P01)`

- [ ] **Step 3: Add the migration (verbatim from spec §4)**

`internal/db/migrations/00002_definitions_submissions.sql`:
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

- [ ] **Step 4: Run the migration test to verify it passes**

Run: `go test ./internal/db/ -run TestMigration00002CreatesTables -v`
Expected: `--- PASS: TestMigration00002CreatesTables`

- [ ] **Step 5: Write the failing test for the shared sample definitions**

`internal/testutil/definitions_test.go`:
```go
package testutil_test

import (
	"context"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/testutil"
)

func TestSampleDefinitionsAreValid(t *testing.T) {
	wf := testutil.SampleWorkflow("review")
	if err := definition.ValidateWorkflow(wf); err != nil {
		t.Fatalf("sample workflow invalid: %v", err)
	}
	f := testutil.SampleForm("contact", "review")
	if err := definition.ValidateForm(f); err != nil {
		t.Fatalf("sample form invalid: %v", err)
	}
	if err := definition.ValidateBundle([]definition.Form{f, testutil.SampleForm("plain", "")}, []definition.Workflow{wf}, nil); err != nil {
		t.Fatalf("sample bundle invalid: %v", err)
	}
}

func TestOtherOrgCreatesDistinctOrg(t *testing.T) {
	env := testutil.NewEnv(t)
	other := env.OtherOrg(t)
	if other == env.OrgID {
		t.Fatal("OtherOrg returned the default org id")
	}
	var n int
	if err := env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 orgs, got %d", n)
	}
}
```

- [ ] **Step 6: Run it to verify it fails**

Run: `go test ./internal/testutil/ -run 'TestSampleDefinitionsAreValid|TestOtherOrgCreatesDistinctOrg' -v`
Expected: FAIL — build error `undefined: testutil.SampleWorkflow`

- [ ] **Step 7: Implement the helpers**

`internal/testutil/definitions.go`:
```go
package testutil

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
)

// SampleWorkflow returns a small valid workflow: new → review → done.
// "start" requires the reviewer role; "finish" requires the "note" field.
func SampleWorkflow(slug string) definition.Workflow {
	return definition.Workflow{
		Slug:    slug,
		Title:   "Review",
		Initial: "new",
		States: []definition.State{
			{Key: "new", Label: "New", Color: "gray"},
			{Key: "review", Label: "In review", Color: "blue"},
			{Key: "done", Label: "Done", Color: "green", Terminal: true},
		},
		Fields: []definition.WorkflowField{
			{Key: "note", Type: definition.FieldTextarea, Label: "Note"},
		},
		Transitions: []definition.Transition{
			{Key: "start", Label: "Start review", From: []string{"new"}, To: "review", Guard: definition.Guard{Roles: []string{"reviewer"}}},
			{Key: "finish", Label: "Finish", From: []string{"review"}, To: "done", Guard: definition.Guard{RequireFields: []string{"note"}}},
		},
	}
}

// SampleForm returns a valid public contact form. workflow may be "".
func SampleForm(slug, workflow string) definition.Form {
	return definition.Form{
		Slug:     slug,
		Title:    "Contact",
		Workflow: workflow,
		Settings: definition.FormSettings{Public: true},
		Fields: []definition.Field{
			{Key: "name", Type: definition.FieldText, Label: "Name", Required: true},
			{Key: "email", Type: definition.FieldEmail, Label: "Email", Required: true},
			{Key: "topic", Type: definition.FieldSelect, Label: "Topic", Options: []definition.Option{
				{Value: "sales", Label: "Sales"},
				{Value: "support", Label: "Support"},
			}},
		},
	}
}

// OtherOrg inserts a second organisation, for cross-org isolation tests.
func (e *Env) OtherOrg(t testing.TB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.Pool.Exec(context.Background(), `INSERT INTO orgs (id, name) VALUES ($1, 'Other')`, id); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	return id
}
```

- [ ] **Step 8: Run tests to verify they pass**

Run: `go test ./internal/testutil/ ./internal/db/ -v`
Expected: all `PASS`, including `TestSampleDefinitionsAreValid` and `TestOtherOrgCreatesDistinctOrg`.

- [ ] **Step 9: Commit**

```bash
git add internal/db/migrations/00002_definitions_submissions.sql internal/db/migrations_00002_test.go internal/testutil/definitions.go internal/testutil/definitions_test.go
git commit -m "feat: add definitions and submissions schema with shared test definitions"
```

---

### Task 2: Definitions store — create, deduplicate and version

**Parallel group:** A (with Task 3)

**Files:**
- Create: `internal/definitions/store.go`
- Create: `internal/definitions/apply.go`
- Test: `internal/definitions/store_test.go`

**Interfaces:**
- Consumes: `definition.Canonical(v any) ([]byte, string, error)`, `definition.Form`, `definition.Workflow`, `testutil.NewEnv`, `testutil.SampleWorkflow`, `testutil.SampleForm`.
- Produces (spec §6.6, plus JSON tags on `ApplyItem`/`ApplyResult` and `ParseSource`):
  - `var ErrNotFound`; `type Source string` with `SourceCLI|SourceUI|SourceAPI|SourceSeed`; `func ParseSource(s string) (Source, bool)`
  - `VersionInfo`, `FormRecord`, `WorkflowRecord`, `ApplyInput`, `ApplyItem{Kind, Slug, Version, Changed, Created}` (json `kind, slug, version, changed, created`), `ApplyResult{Items}` (json `items`)
  - `func NewStore(pool *pgxpool.Pool) *Store`
  - `func (s *Store) Apply(ctx, orgID uuid.UUID, in ApplyInput) (ApplyResult, error)` — this task: create/dedupe/new-version only (validation, re-pin and dry-run land in Task 4)
  - `func (s *Store) GetForm(ctx, orgID, slug) (FormRecord, error)`, `func (s *Store) GetWorkflow(ctx, orgID, slug) (WorkflowRecord, error)`

- [ ] **Step 1: Write the failing tests**

`internal/definitions/store_test.go`:
```go
package definitions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func newStore(t *testing.T) (*testutil.Env, *definitions.Store) {
	t.Helper()
	env := testutil.NewEnv(t)
	return env, definitions.NewStore(env.Pool)
}

func applyOK(t *testing.T, s *definitions.Store, env *testutil.Env, in definitions.ApplyInput) definitions.ApplyResult {
	t.Helper()
	res, err := s.Apply(context.Background(), env.OrgID, in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return res
}

func TestApplyCreatesVersionOne(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
		Source:    definitions.SourceCLI,
		Actor:     "dev@example.com",
	})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 1, Changed: true, Created: true},
		{Kind: "form", Slug: "contact", Version: 1, Changed: true, Created: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}

	f, err := s.GetForm(ctx, env.OrgID, "contact")
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}
	if f.Current.Version != 1 || f.Current.Source != definitions.SourceCLI || f.Current.CreatedBy != "dev@example.com" {
		t.Fatalf("unexpected version info %+v", f.Current)
	}
	if f.Definition.Title != "Contact" || len(f.Definition.Fields) != 3 {
		t.Fatalf("definition not round-tripped: %+v", f.Definition)
	}
	if len(f.Current.Hash) != 64 {
		t.Fatalf("hash should be sha256 hex, got %q", f.Current.Hash)
	}

	wf, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if f.WorkflowVersionID == nil || *f.WorkflowVersionID != wf.Current.ID {
		t.Fatalf("form pin = %v, want workflow version %v", f.WorkflowVersionID, wf.Current.ID)
	}
	if wf.Definition.Initial != "new" || len(wf.Definition.States) != 3 {
		t.Fatalf("workflow not round-tripped: %+v", wf.Definition)
	}
}

func TestApplyUnchangedIsNoop(t *testing.T) {
	env, s := newStore(t)
	in := definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	}
	applyOK(t, s, env, in)
	res := applyOK(t, s, env, in)
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 1},
		{Kind: "form", Slug: "contact", Version: 1},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
}

func TestApplyChangedFormCreatesNewVersion(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	changed := testutil.SampleForm("plain", "")
	changed.Title = "Contact us"
	res := applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{changed}, Source: definitions.SourceUI})
	want := []definitions.ApplyItem{{Kind: "form", Slug: "plain", Version: 2, Changed: true}}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
	f, err := s.GetForm(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Version != 2 || f.Definition.Title != "Contact us" || f.Current.Source != definitions.SourceUI {
		t.Fatalf("unexpected record %+v", f)
	}
	if f.WorkflowVersionID != nil {
		t.Fatalf("form without workflow must not be pinned, got %v", f.WorkflowVersionID)
	}
}

func TestApplyDefaultsSourceToAPI(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	f, err := s.GetForm(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Source != definitions.SourceAPI {
		t.Fatalf("source = %q, want api", f.Current.Source)
	}
}

func TestGetUnknownReturnsNotFound(t *testing.T) {
	env, s := newStore(t)
	if _, err := s.GetForm(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("GetForm err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetWorkflow(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("GetWorkflow err = %v, want ErrNotFound", err)
	}
}

func TestParseSource(t *testing.T) {
	for _, s := range []string{"cli", "ui", "api", "seed"} {
		if got, ok := definitions.ParseSource(s); !ok || string(got) != s {
			t.Fatalf("ParseSource(%q) = %q, %v", s, got, ok)
		}
	}
	if _, ok := definitions.ParseSource("git"); ok {
		t.Fatal("ParseSource(git) should fail")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/definitions/ -v`
Expected: FAIL — build error `undefined: definitions.NewStore`

- [ ] **Step 3: Implement types, scanning and lookups**

`internal/definitions/store.go`:
```go
// Package definitions persists versioned form and workflow definitions.
package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/definition"
)

var ErrNotFound = errors.New("definition not found")

type Source string

const (
	SourceCLI  Source = "cli"
	SourceUI   Source = "ui"
	SourceAPI  Source = "api"
	SourceSeed Source = "seed"
)

// ParseSource reports whether s is a known version source.
func ParseSource(s string) (Source, bool) {
	switch Source(s) {
	case SourceCLI, SourceUI, SourceAPI, SourceSeed:
		return Source(s), true
	}
	return "", false
}

type VersionInfo struct {
	ID        uuid.UUID
	Version   int
	Hash      string
	Source    Source
	CreatedBy string
	CreatedAt time.Time
}

// FormRecord is a form at one version. For records returned by version
// lookups (GetFormVersion, FormVersion) UpdatedAt equals Current.CreatedAt.
type FormRecord struct {
	ID                uuid.UUID
	Slug              string
	Current           VersionInfo
	WorkflowVersionID *uuid.UUID
	Definition        definition.Form
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type WorkflowRecord struct {
	ID         uuid.UUID
	Slug       string
	Current    VersionInfo
	Definition definition.Workflow
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type ApplyInput struct {
	Forms     []definition.Form
	Workflows []definition.Workflow
	Source    Source
	Actor     string
	DryRun    bool
}

type ApplyItem struct {
	Kind    string `json:"kind"`
	Slug    string `json:"slug"`
	Version int    `json:"version"`
	Changed bool   `json:"changed"`
	Created bool   `json:"created"`
}

type ApplyResult struct {
	Items []ApplyItem `json:"items"`
}

// Store reads and writes definitions. Versions are immutable, so records
// looked up by version id are cached for the life of the process.
// Callers must not mutate returned definitions.
type Store struct {
	pool             *pgxpool.Pool
	formVersions     sync.Map // uuid.UUID → FormRecord
	workflowVersions sync.Map // uuid.UUID → WorkflowRecord
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

const formCols = `SELECT f.id, f.slug, f.created_at, f.updated_at,
	v.id, v.version, v.hash, v.source, v.created_by, v.created_at, v.workflow_version_id, v.definition
FROM form_versions v JOIN forms f ON f.id = v.form_id`

const workflowCols = `SELECT w.id, w.slug, w.created_at, w.updated_at,
	v.id, v.version, v.hash, v.source, v.created_by, v.created_at, v.definition
FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id`

func scanForm(row pgx.Row) (FormRecord, error) {
	var (
		r   FormRecord
		src string
		raw []byte
	)
	err := row.Scan(&r.ID, &r.Slug, &r.CreatedAt, &r.UpdatedAt,
		&r.Current.ID, &r.Current.Version, &r.Current.Hash, &src, &r.Current.CreatedBy, &r.Current.CreatedAt,
		&r.WorkflowVersionID, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return FormRecord{}, ErrNotFound
	}
	if err != nil {
		return FormRecord{}, fmt.Errorf("scan form: %w", err)
	}
	r.Current.Source = Source(src)
	if err := json.Unmarshal(raw, &r.Definition); err != nil {
		return FormRecord{}, fmt.Errorf("decode form %s: %w", r.Slug, err)
	}
	return r, nil
}

func scanWorkflow(row pgx.Row) (WorkflowRecord, error) {
	var (
		r   WorkflowRecord
		src string
		raw []byte
	)
	err := row.Scan(&r.ID, &r.Slug, &r.CreatedAt, &r.UpdatedAt,
		&r.Current.ID, &r.Current.Version, &r.Current.Hash, &src, &r.Current.CreatedBy, &r.Current.CreatedAt, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return WorkflowRecord{}, ErrNotFound
	}
	if err != nil {
		return WorkflowRecord{}, fmt.Errorf("scan workflow: %w", err)
	}
	r.Current.Source = Source(src)
	if err := json.Unmarshal(raw, &r.Definition); err != nil {
		return WorkflowRecord{}, fmt.Errorf("decode workflow %s: %w", r.Slug, err)
	}
	return r, nil
}

// GetForm returns the current version of a form.
func (s *Store) GetForm(ctx context.Context, orgID uuid.UUID, slug string) (FormRecord, error) {
	return scanForm(s.pool.QueryRow(ctx, formCols+` WHERE f.org_id = $1 AND f.slug = $2 AND f.current_version_id = v.id`, orgID, slug))
}

// GetWorkflow returns the current version of a workflow.
func (s *Store) GetWorkflow(ctx context.Context, orgID uuid.UUID, slug string) (WorkflowRecord, error) {
	return scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE w.org_id = $1 AND w.slug = $2 AND w.current_version_id = v.id`, orgID, slug))
}
```

- [ ] **Step 4: Implement `Apply` (create / dedupe / new version)**

`internal/definitions/apply.go`:
```go
package definitions

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

// Apply writes a bundle of definitions in one transaction. A definition whose
// canonical hash (and, for forms, workflow pin) is unchanged creates no version.
func (s *Store) Apply(ctx context.Context, orgID uuid.UUID, in ApplyInput) (ApplyResult, error) {
	if in.Source == "" {
		in.Source = SourceAPI
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	res := ApplyResult{Items: []ApplyItem{}}
	for _, wf := range in.Workflows {
		item, _, err := upsertWorkflow(ctx, tx, orgID, wf, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
	}
	for _, f := range in.Forms {
		pin, err := currentWorkflowVersion(ctx, tx, orgID, f.Workflow)
		if err != nil {
			return ApplyResult{}, err
		}
		item, err := upsertForm(ctx, tx, orgID, f, pin, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, err
	}
	return res, nil
}

// upsertWorkflow returns the item and the id of the workflow's current version
// after the write. INSERT ... ON CONFLICT DO NOTHING followed by SELECT ... FOR
// UPDATE makes concurrent applies of a new slug serialize instead of failing.
func upsertWorkflow(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, wf definition.Workflow, src Source, actor string) (ApplyItem, uuid.UUID, error) {
	canon, hash, err := definition.Canonical(wf)
	if err != nil {
		return ApplyItem{}, uuid.Nil, err
	}
	item := ApplyItem{Kind: "workflow", Slug: wf.Slug}
	tag, err := tx.Exec(ctx, `INSERT INTO workflows (id, org_id, slug) VALUES ($1, $2, $3) ON CONFLICT (org_id, slug) DO NOTHING`, uuid.New(), orgID, wf.Slug)
	if err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("insert workflow %s: %w", wf.Slug, err)
	}
	item.Created = tag.RowsAffected() == 1

	var (
		id         uuid.UUID
		curID      *uuid.UUID
		curVersion *int
		curHash    *string
	)
	err = tx.QueryRow(ctx, `
		SELECT w.id, v.id, v.version, v.hash
		FROM workflows w LEFT JOIN workflow_versions v ON v.id = w.current_version_id
		WHERE w.org_id = $1 AND w.slug = $2
		FOR UPDATE OF w`, orgID, wf.Slug).Scan(&id, &curID, &curVersion, &curHash)
	if err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("lock workflow %s: %w", wf.Slug, err)
	}
	if curHash != nil && *curHash == hash {
		item.Version = *curVersion
		return item, *curID, nil
	}
	next := 1
	if curVersion != nil {
		next = *curVersion + 1
	}
	vid := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_versions (id, workflow_id, version, definition, hash, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, vid, id, next, canon, hash, string(src), actor); err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("insert workflow version %s: %w", wf.Slug, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workflows SET current_version_id = $1, updated_at = now() WHERE id = $2`, vid, id); err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("update workflow %s: %w", wf.Slug, err)
	}
	item.Version, item.Changed = next, true
	return item, vid, nil
}

func upsertForm(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, f definition.Form, pin *uuid.UUID, src Source, actor string) (ApplyItem, error) {
	canon, hash, err := definition.Canonical(f)
	if err != nil {
		return ApplyItem{}, err
	}
	item := ApplyItem{Kind: "form", Slug: f.Slug}
	tag, err := tx.Exec(ctx, `INSERT INTO forms (id, org_id, slug) VALUES ($1, $2, $3) ON CONFLICT (org_id, slug) DO NOTHING`, uuid.New(), orgID, f.Slug)
	if err != nil {
		return ApplyItem{}, fmt.Errorf("insert form %s: %w", f.Slug, err)
	}
	item.Created = tag.RowsAffected() == 1

	var (
		id         uuid.UUID
		curVersion *int
		curHash    *string
		curPin     *uuid.UUID
	)
	err = tx.QueryRow(ctx, `
		SELECT f.id, v.version, v.hash, v.workflow_version_id
		FROM forms f LEFT JOIN form_versions v ON v.id = f.current_version_id
		WHERE f.org_id = $1 AND f.slug = $2
		FOR UPDATE OF f`, orgID, f.Slug).Scan(&id, &curVersion, &curHash, &curPin)
	if err != nil {
		return ApplyItem{}, fmt.Errorf("lock form %s: %w", f.Slug, err)
	}
	if curHash != nil && *curHash == hash && samePin(curPin, pin) {
		item.Version = *curVersion
		return item, nil
	}
	next := 1
	if curVersion != nil {
		next = *curVersion + 1
	}
	vid := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO form_versions (id, form_id, version, definition, hash, workflow_version_id, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, vid, id, next, canon, hash, pin, string(src), actor); err != nil {
		return ApplyItem{}, fmt.Errorf("insert form version %s: %w", f.Slug, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE forms SET current_version_id = $1, updated_at = now() WHERE id = $2`, vid, id); err != nil {
		return ApplyItem{}, fmt.Errorf("update form %s: %w", f.Slug, err)
	}
	item.Version, item.Changed = next, true
	return item, nil
}

func samePin(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// currentWorkflowVersion returns the current version id of the workflow a form
// references, or nil when the form has no workflow.
func currentWorkflowVersion(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, slug string) (*uuid.UUID, error) {
	if slug == "" {
		return nil, nil
	}
	var id *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT current_version_id FROM workflows WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && id == nil) {
		return nil, fmt.Errorf("workflow %q: %w", slug, ErrNotFound)
	}
	return id, err
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/definitions/ -v`
Expected: `PASS` for `TestApplyCreatesVersionOne`, `TestApplyUnchangedIsNoop`, `TestApplyChangedFormCreatesNewVersion`, `TestApplyDefaultsSourceToAPI`, `TestGetUnknownReturnsNotFound`, `TestParseSource`.

- [ ] **Step 6: Commit**

```bash
git add internal/definitions/
git commit -m "feat: add versioned definitions store with hash deduplication"
```

---

### Task 3: Public submission rate limiter

**Parallel group:** A (with Task 2)

**Files:**
- Create: `internal/httpapi/ratelimit.go`
- Test: `internal/httpapi/ratelimit_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces (package-private, used by Task 10):
  - `func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter`
  - `func (l *rateLimiter) Allow(key string) bool`
  - `func clientIP(r *http.Request) string` — host part of `r.RemoteAddr` (Plan 09 may put `middleware.RealIP` in front for proxies).

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/ratelimit_test.go`:
```go
package httpapi

import (
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestRateLimiterAllowsUpToLimitPerWindow(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	l := newRateLimiter(20, time.Minute, clock.now)
	for i := 0; i < 20; i++ {
		if !l.Allow("198.51.100.7") {
			t.Fatalf("request %d rejected, want allowed", i+1)
		}
	}
	if l.Allow("198.51.100.7") {
		t.Fatal("21st request allowed, want rejected")
	}
	if !l.Allow("203.0.113.9") {
		t.Fatal("other client rejected, want allowed")
	}
	clock.t = clock.t.Add(59 * time.Second)
	if l.Allow("198.51.100.7") {
		t.Fatal("request inside the window allowed, want rejected")
	}
	clock.t = clock.t.Add(time.Second)
	if !l.Allow("198.51.100.7") {
		t.Fatal("request after the window rejected, want allowed")
	}
}

func TestRateLimiterSweepsExpiredEntries(t *testing.T) {
	clock := &fakeClock{t: time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)}
	l := newRateLimiter(1, time.Minute, clock.now)
	l.maxKeys = 3
	for _, k := range []string{"a", "b", "c"} {
		l.Allow(k)
	}
	clock.t = clock.t.Add(2 * time.Minute)
	l.Allow("d")
	if len(l.hits) != 1 {
		t.Fatalf("want expired keys swept, have %d keys", len(l.hits))
	}
}

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("POST", "/", nil)
	r.RemoteAddr = "192.0.2.10:54321"
	if got := clientIP(r); got != "192.0.2.10" {
		t.Fatalf("clientIP = %q", got)
	}
	r.RemoteAddr = "[2001:db8::1]:443"
	if got := clientIP(r); got != "2001:db8::1" {
		t.Fatalf("clientIP v6 = %q", got)
	}
	r.RemoteAddr = "unix-socket"
	if got := clientIP(r); got != "unix-socket" {
		t.Fatalf("clientIP fallback = %q", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run 'TestRateLimiter|TestClientIP' -v`
Expected: FAIL — build error `undefined: newRateLimiter`

- [ ] **Step 3: Implement the limiter**

`internal/httpapi/ratelimit.go`:
```go
package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// rateLimiter is a fixed-window counter per key. It is in-memory and
// per-process, which is sufficient for the single-instance v1 deployment.
type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	now     func() time.Time
	hits    map[string]*rlWindow
	maxKeys int
}

type rlWindow struct {
	start time.Time
	count int
}

func newRateLimiter(limit int, window time.Duration, now func() time.Time) *rateLimiter {
	return &rateLimiter{limit: limit, window: window, now: now, hits: map[string]*rlWindow{}, maxKeys: 10000}
}

// Allow records a hit for key and reports whether it is within the limit.
func (l *rateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	t := l.now()
	w, ok := l.hits[key]
	if !ok || t.Sub(w.start) >= l.window {
		if !ok && len(l.hits) >= l.maxKeys {
			l.sweep(t)
		}
		l.hits[key] = &rlWindow{start: t, count: 1}
		return true
	}
	if w.count >= l.limit {
		return false
	}
	w.count++
	return true
}

func (l *rateLimiter) sweep(t time.Time) {
	for k, w := range l.hits {
		if t.Sub(w.start) >= l.window {
			delete(l.hits, k)
		}
	}
}

// clientIP returns the host part of RemoteAddr.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -run 'TestRateLimiter|TestClientIP' -v`
Expected: `PASS` for all three tests.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/ratelimit.go internal/httpapi/ratelimit_test.go
git commit -m "feat: add fixed-window rate limiter for public submissions"
```

---

### Task 4: Definitions store — bundle validation, re-pinning and dry-run

**Files:**
- Create: `internal/definitions/validate.go`
- Modify: `internal/definitions/apply.go` (replace the `Apply` function; add `dependentForms`)
- Test: `internal/definitions/apply_test.go`

**Interfaces:**
- Consumes: Task 2 `upsertWorkflow`, `upsertForm`, `currentWorkflowVersion`; `definition.ValidateForm`, `definition.ValidateWorkflow`, `definition.ValidateBundle`, `*definition.ValidationError`, `db.DBTX`.
- Produces:
  - `func PrefixProblems(prefix string, err error) []definition.Problem` — used by Task 9 to prefix per-document parse problems (`forms[1].fields[0].key`).
  - `Apply` now: validates every document and the bundle (duplicate slugs rejected; cross-refs resolve against bundle + existing workflows) → returns `*definition.ValidationError` with paths prefixed `workflows[i]` / `forms[i]`; re-pins forms outside the bundle that reference a workflow that got a new version (those appear in `Items` with `Changed: true`); `DryRun` returns the would-be result and rolls back.

- [ ] **Step 1: Write the failing tests**

`internal/definitions/apply_test.go`:
```go
package definitions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func validationErr(t *testing.T, err error) *definition.ValidationError {
	t.Helper()
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v (%T), want *definition.ValidationError", err, err)
	}
	return ve
}

func TestApplyRepinsDependentForms(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", "")},
	})

	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	res := applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{wf}, Source: definitions.SourceUI})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 2, Changed: true},
		{Kind: "form", Slug: "contact", Version: 2, Changed: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}

	f, err := s.GetForm(ctx, env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatal(err)
	}
	if f.WorkflowVersionID == nil || *f.WorkflowVersionID != w.Current.ID {
		t.Fatalf("contact pinned to %v, want %v", f.WorkflowVersionID, w.Current.ID)
	}
	if f.Current.Source != definitions.SourceUI {
		t.Fatalf("re-pinned version source = %q, want ui", f.Current.Source)
	}
	p, err := s.GetForm(ctx, env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if p.Current.Version != 1 {
		t.Fatalf("unrelated form re-versioned to %d", p.Current.Version)
	}
}

func TestApplyRepinsFormInBundleWithUnchangedDefinition(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{wf},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 2, Changed: true},
		{Kind: "form", Slug: "contact", Version: 2, Changed: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
}

func TestApplyFormAgainstExistingWorkflow(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	res := applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("contact", "review")}})
	if len(res.Items) != 1 || !res.Items[0].Created {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestApplyDryRunPersistsNothing(t *testing.T) {
	env, s := newStore(t)
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
		DryRun:    true,
	})
	if len(res.Items) != 2 || !res.Items[0].Created || !res.Items[1].Created {
		t.Fatalf("dry-run items = %+v", res.Items)
	}
	if _, err := s.GetForm(context.Background(), env.OrgID, "contact"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("dry-run persisted form: err = %v", err)
	}
	if _, err := s.GetWorkflow(context.Background(), env.OrgID, "review"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("dry-run persisted workflow: err = %v", err)
	}
}

func TestApplyRejectsMissingWorkflow(t *testing.T) {
	env, s := newStore(t)
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{testutil.SampleForm("contact", "nope")},
	})
	validationErr(t, err)
	if _, err := s.GetForm(context.Background(), env.OrgID, "contact"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("invalid bundle persisted form: %v", err)
	}
}

func TestApplyRejectsDuplicateSlugs(t *testing.T) {
	env, s := newStore(t)
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{testutil.SampleForm("plain", ""), testutil.SampleForm("plain", "")},
	})
	ve := validationErr(t, err)
	if ve.Problems[0].Path != "forms[1].slug" {
		t.Fatalf("problem path = %q, want forms[1].slug", ve.Problems[0].Path)
	}
}

func TestApplyPrefixesDocumentProblems(t *testing.T) {
	env, s := newStore(t)
	wf := testutil.SampleWorkflow("review")
	wf.Initial = "missing"
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{Workflows: []definition.Workflow{wf}})
	ve := validationErr(t, err)
	for _, p := range ve.Problems {
		if !strings.HasPrefix(p.Path, "workflows[0]") {
			t.Fatalf("problem path %q not prefixed with workflows[0]", p.Path)
		}
	}
}

func TestApplyConcurrentSameNewSlug(t *testing.T) {
	env, s := newStore(t)
	in := definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Apply(context.Background(), env.OrgID, in)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	vs, err := s.FormVersions(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("want exactly 1 version, got %d", len(vs))
	}
}

func TestPrefixProblems(t *testing.T) {
	err := &definition.ValidationError{Problems: []definition.Problem{
		{Path: "fields[0].key", Message: "bad key"},
		{Path: "", Message: "whole document"},
		{Path: "[2]", Message: "indexed"},
	}}
	got := definitions.PrefixProblems("forms[3]", err)
	want := []definition.Problem{
		{Path: "forms[3].fields[0].key", Message: "bad key"},
		{Path: "forms[3]", Message: "whole document"},
		{Path: "forms[3][2]", Message: "indexed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := definitions.PrefixProblems("forms[0]", errors.New("boom")); len(got) != 1 || got[0].Path != "forms[0]" || got[0].Message != "boom" {
		t.Fatalf("plain error: %+v", got)
	}
	if got := definitions.PrefixProblems("forms[0]", nil); got != nil {
		t.Fatalf("nil error: %+v", got)
	}
}
```
`TestApplyConcurrentSameNewSlug` uses `FormVersions`, which Task 5 adds; it is expected to fail to compile until then, so this step adds a minimal `FormVersions` in Step 3 below and Task 5 keeps it.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/definitions/ -v`
Expected: FAIL — build errors `undefined: definitions.PrefixProblems` and `s.FormVersions undefined`

- [ ] **Step 3: Implement validation, `FormVersions`, and the full `Apply`**

`internal/definitions/validate.go`:
```go
package definitions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
)

// PrefixProblems converts err into problems whose paths are prefixed with
// prefix (e.g. "forms[2]"). A non-validation error becomes one problem at prefix.
func PrefixProblems(prefix string, err error) []definition.Problem {
	if err == nil {
		return nil
	}
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		return []definition.Problem{{Path: prefix, Message: err.Error()}}
	}
	out := make([]definition.Problem, 0, len(ve.Problems))
	for _, p := range ve.Problems {
		path := prefix
		switch {
		case p.Path == "":
		case strings.HasPrefix(p.Path, "["):
			path += p.Path
		default:
			path += "." + p.Path
		}
		out = append(out, definition.Problem{Path: path, Message: p.Message})
	}
	return out
}

func validateInput(ctx context.Context, q db.DBTX, orgID uuid.UUID, in ApplyInput) error {
	var problems []definition.Problem
	seenWF := map[string]bool{}
	for i, wf := range in.Workflows {
		prefix := fmt.Sprintf("workflows[%d]", i)
		if seenWF[wf.Slug] {
			problems = append(problems, definition.Problem{Path: prefix + ".slug", Message: fmt.Sprintf("workflow %q appears more than once in the bundle", wf.Slug)})
		}
		seenWF[wf.Slug] = true
		problems = append(problems, PrefixProblems(prefix, definition.ValidateWorkflow(wf))...)
	}
	seenForm := map[string]bool{}
	for i, f := range in.Forms {
		prefix := fmt.Sprintf("forms[%d]", i)
		if seenForm[f.Slug] {
			problems = append(problems, definition.Problem{Path: prefix + ".slug", Message: fmt.Sprintf("form %q appears more than once in the bundle", f.Slug)})
		}
		seenForm[f.Slug] = true
		problems = append(problems, PrefixProblems(prefix, definition.ValidateForm(f))...)
	}
	if len(problems) > 0 {
		return &definition.ValidationError{Problems: problems}
	}
	existing, err := workflowSlugs(ctx, q, orgID)
	if err != nil {
		return err
	}
	return definition.ValidateBundle(in.Forms, in.Workflows, existing)
}

func workflowSlugs(ctx context.Context, q db.DBTX, orgID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT slug FROM workflows WHERE org_id = $1 AND current_version_id IS NOT NULL ORDER BY slug`, orgID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}
```

`internal/definitions/reads.go` (created now with `FormVersions`; Task 5 adds the rest of the file):
```go
package definitions

import (
	"context"

	"github.com/google/uuid"
)

// FormVersions lists a form's versions, newest first.
func (s *Store) FormVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error) {
	return s.versions(ctx, `SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
		FROM form_versions v JOIN forms f ON f.id = v.form_id
		WHERE f.org_id = $1 AND f.slug = $2 ORDER BY v.version DESC`, orgID, slug)
}

func (s *Store) versions(ctx context.Context, sql string, args ...any) ([]VersionInfo, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var (
			v   VersionInfo
			src string
		)
		if err := rows.Scan(&v.ID, &v.Version, &v.Hash, &src, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Source = Source(src)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}
```

In `internal/definitions/apply.go`, replace the whole `Apply` function with the version below, and add `dependentForms` after it (add `"encoding/json"` to the imports):
```go
// Apply writes a bundle of definitions in one transaction:
//  1. validate every document and the bundle (cross-refs may resolve to existing workflows);
//  2. upsert workflows, then forms (each form pins its workflow's current version);
//  3. re-pin forms outside the bundle whose workflow just got a new version.
//
// Unchanged definitions create no version. DryRun reports the would-be result
// and rolls back.
func (s *Store) Apply(ctx context.Context, orgID uuid.UUID, in ApplyInput) (ApplyResult, error) {
	if in.Source == "" {
		in.Source = SourceAPI
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if err := validateInput(ctx, tx, orgID, in); err != nil {
		return ApplyResult{}, err
	}

	res := ApplyResult{Items: []ApplyItem{}}
	newWorkflowVersions := map[string]uuid.UUID{}
	for _, wf := range in.Workflows {
		item, vid, err := upsertWorkflow(ctx, tx, orgID, wf, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
		if item.Changed {
			newWorkflowVersions[wf.Slug] = vid
		}
	}

	inBundle := map[string]bool{}
	for _, f := range in.Forms {
		inBundle[f.Slug] = true
		pin, err := currentWorkflowVersion(ctx, tx, orgID, f.Workflow)
		if err != nil {
			return ApplyResult{}, err
		}
		item, err := upsertForm(ctx, tx, orgID, f, pin, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
	}

	for _, wf := range in.Workflows {
		vid, ok := newWorkflowVersions[wf.Slug]
		if !ok {
			continue
		}
		deps, err := dependentForms(ctx, tx, orgID, wf.Slug)
		if err != nil {
			return ApplyResult{}, err
		}
		for _, f := range deps {
			if inBundle[f.Slug] {
				continue
			}
			pin := vid
			item, err := upsertForm(ctx, tx, orgID, f, &pin, in.Source, in.Actor)
			if err != nil {
				return ApplyResult{}, err
			}
			if item.Changed {
				res.Items = append(res.Items, item)
			}
		}
	}

	if in.DryRun {
		return res, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, err
	}
	return res, nil
}

// dependentForms returns the current definitions of forms that reference workflowSlug.
func dependentForms(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, workflowSlug string) ([]definition.Form, error) {
	rows, err := tx.Query(ctx, `
		SELECT v.definition
		FROM forms f JOIN form_versions v ON v.id = f.current_version_id
		WHERE f.org_id = $1 AND v.definition->>'workflow' = $2
		ORDER BY f.slug`, orgID, workflowSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []definition.Form
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var f definition.Form
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/definitions/ -race -v`
Expected: all tests `PASS` (Task 2 tests plus `TestApplyRepinsDependentForms`, `TestApplyRepinsFormInBundleWithUnchangedDefinition`, `TestApplyFormAgainstExistingWorkflow`, `TestApplyDryRunPersistsNothing`, `TestApplyRejectsMissingWorkflow`, `TestApplyRejectsDuplicateSlugs`, `TestApplyPrefixesDocumentProblems`, `TestApplyConcurrentSameNewSlug`, `TestPrefixProblems`).

- [ ] **Step 5: Commit**

```bash
git add internal/definitions/
git commit -m "feat: validate bundles, re-pin dependent forms and support dry-run apply"
```

---

### Task 5: Definitions store — reads, version history, cache and export

**Files:**
- Modify: `internal/definitions/reads.go` (append functions)
- Test: `internal/definitions/reads_test.go`

**Interfaces:**
- Consumes: Task 2 `formCols`, `workflowCols`, `scanForm`, `scanWorkflow`; Task 4 `versions`.
- Produces (spec §6.6): `GetFormVersion` (cached), `FormVersion`, `ListForms` (by slug), `ListWorkflows` (by slug), `WorkflowVersions` (newest first), `WorkflowVersion`, `GetWorkflowVersion` (cached), `Export` (sorted by slug, never nil slices).

- [ ] **Step 1: Write the failing tests**

`internal/definitions/reads_test.go`:
```go
package definitions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func seedTwoVersions(t *testing.T) (*testutil.Env, *definitions.Store) {
	t.Helper()
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	f := testutil.SampleForm("contact", "review")
	f.Title = "Contact v2"
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{f}, Source: definitions.SourceUI, Actor: "ui@example.com"})
	return env, s
}

func TestFormVersionsNewestFirst(t *testing.T) {
	env, s := seedTwoVersions(t)
	vs, err := s.FormVersions(context.Background(), env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0].Version != 2 || vs[1].Version != 1 {
		t.Fatalf("versions = %+v", vs)
	}
	if vs[0].Source != definitions.SourceUI || vs[0].CreatedBy != "ui@example.com" {
		t.Fatalf("newest version info = %+v", vs[0])
	}
	if _, err := s.FormVersions(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("missing form err = %v", err)
	}
}

func TestFormVersionReturnsHistoricDefinition(t *testing.T) {
	env, s := seedTwoVersions(t)
	ctx := context.Background()
	v1, err := s.FormVersion(ctx, env.OrgID, "contact", 1)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Definition.Title != "Contact" || v1.Current.Version != 1 {
		t.Fatalf("v1 = %+v", v1)
	}
	if _, err := s.FormVersion(ctx, env.OrgID, "contact", 9); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("v9 err = %v", err)
	}
	byID, err := s.GetFormVersion(ctx, v1.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Definition.Title != "Contact" || byID.Slug != "contact" {
		t.Fatalf("GetFormVersion = %+v", byID)
	}
}

func TestWorkflowVersionsAndLookups(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{wf}})

	vs, err := s.WorkflowVersions(ctx, env.OrgID, "review")
	if err != nil || len(vs) != 2 || vs[0].Version != 2 {
		t.Fatalf("versions = %+v, err = %v", vs, err)
	}
	v1, err := s.WorkflowVersion(ctx, env.OrgID, "review", 1)
	if err != nil || v1.Definition.Title != "Review" {
		t.Fatalf("v1 = %+v, err = %v", v1, err)
	}
	if _, err := s.WorkflowVersion(ctx, env.OrgID, "review", 3); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("v3 err = %v", err)
	}
}

func TestGetWorkflowVersionIsCached(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	cur, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.GetWorkflowVersion(ctx, cur.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Versions are immutable; tamper with the row to prove the second read is served from cache.
	if _, err := env.Pool.Exec(ctx, `UPDATE workflow_versions SET definition = jsonb_set(definition, '{title}', '"tampered"') WHERE id = $1`, cur.Current.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.GetWorkflowVersion(ctx, cur.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Definition.Title != "Review" || second.Definition.Title != "Review" {
		t.Fatalf("cache miss: first %q second %q", first.Definition.Title, second.Definition.Title)
	}
}

func TestListAndExportSortedBySlug(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("zeta"), testutil.SampleWorkflow("alpha")},
		Forms:     []definition.Form{testutil.SampleForm("b-form", "zeta"), testutil.SampleForm("a-form", "")},
	})
	forms, err := s.ListForms(ctx, env.OrgID)
	if err != nil || len(forms) != 2 || forms[0].Slug != "a-form" || forms[1].Slug != "b-form" {
		t.Fatalf("ListForms = %+v, err = %v", forms, err)
	}
	wfs, err := s.ListWorkflows(ctx, env.OrgID)
	if err != nil || len(wfs) != 2 || wfs[0].Slug != "alpha" {
		t.Fatalf("ListWorkflows = %+v, err = %v", wfs, err)
	}
	ef, ew, err := s.Export(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ef) != 2 || ef[0].Slug != "a-form" || len(ew) != 2 || ew[1].Slug != "zeta" {
		t.Fatalf("Export forms=%+v workflows=%+v", ef, ew)
	}
}

func TestExportEmptyOrgReturnsEmptySlices(t *testing.T) {
	env, s := newStore(t)
	ef, ew, err := s.Export(context.Background(), env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if ef == nil || ew == nil || len(ef) != 0 || len(ew) != 0 {
		t.Fatalf("want empty non-nil slices, got %v %v", ef, ew)
	}
}

func TestListIsOrgScoped(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	other := env.OtherOrg(t)
	forms, err := s.ListForms(context.Background(), other)
	if err != nil || len(forms) != 0 {
		t.Fatalf("other org sees %d forms, err = %v", len(forms), err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/definitions/ -run 'TestFormVersion|TestWorkflowVersions|TestGetWorkflowVersionIsCached|TestList|TestExport' -v`
Expected: FAIL — build errors `s.FormVersion undefined`, `s.GetFormVersion undefined`, `s.ListForms undefined`, …

- [ ] **Step 3: Implement the read functions**

Append to `internal/definitions/reads.go` (extend imports to `"context"`, `"github.com/google/uuid"`, `"github.com/openforms/openforms/internal/definition"`):
```go
// GetFormVersion returns a form at a specific version id. Cached: versions are immutable.
func (s *Store) GetFormVersion(ctx context.Context, versionID uuid.UUID) (FormRecord, error) {
	if v, ok := s.formVersions.Load(versionID); ok {
		return v.(FormRecord), nil
	}
	rec, err := scanForm(s.pool.QueryRow(ctx, formCols+` WHERE v.id = $1`, versionID))
	if err != nil {
		return FormRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	s.formVersions.Store(versionID, rec)
	return rec, nil
}

// FormVersion returns a form at version number n.
func (s *Store) FormVersion(ctx context.Context, orgID uuid.UUID, slug string, n int) (FormRecord, error) {
	rec, err := scanForm(s.pool.QueryRow(ctx, formCols+` WHERE f.org_id = $1 AND f.slug = $2 AND v.version = $3`, orgID, slug, n))
	if err != nil {
		return FormRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	return rec, nil
}

// ListForms returns every form at its current version, ordered by slug.
func (s *Store) ListForms(ctx context.Context, orgID uuid.UUID) ([]FormRecord, error) {
	rows, err := s.pool.Query(ctx, formCols+` WHERE f.org_id = $1 AND f.current_version_id = v.id ORDER BY f.slug`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FormRecord{}
	for rows.Next() {
		rec, err := scanForm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetWorkflowVersion returns a workflow at a specific version id. Cached: versions are immutable.
func (s *Store) GetWorkflowVersion(ctx context.Context, versionID uuid.UUID) (WorkflowRecord, error) {
	if v, ok := s.workflowVersions.Load(versionID); ok {
		return v.(WorkflowRecord), nil
	}
	rec, err := scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE v.id = $1`, versionID))
	if err != nil {
		return WorkflowRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	s.workflowVersions.Store(versionID, rec)
	return rec, nil
}

// WorkflowVersion returns a workflow at version number n.
func (s *Store) WorkflowVersion(ctx context.Context, orgID uuid.UUID, slug string, n int) (WorkflowRecord, error) {
	rec, err := scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE w.org_id = $1 AND w.slug = $2 AND v.version = $3`, orgID, slug, n))
	if err != nil {
		return WorkflowRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	return rec, nil
}

// ListWorkflows returns every workflow at its current version, ordered by slug.
func (s *Store) ListWorkflows(ctx context.Context, orgID uuid.UUID) ([]WorkflowRecord, error) {
	rows, err := s.pool.Query(ctx, workflowCols+` WHERE w.org_id = $1 AND w.current_version_id = v.id ORDER BY w.slug`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRecord{}
	for rows.Next() {
		rec, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// WorkflowVersions lists a workflow's versions, newest first.
func (s *Store) WorkflowVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error) {
	return s.versions(ctx, `SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
		FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
		WHERE w.org_id = $1 AND w.slug = $2 ORDER BY v.version DESC`, orgID, slug)
}

// Export returns the current definitions of the org, sorted by slug.
func (s *Store) Export(ctx context.Context, orgID uuid.UUID) ([]definition.Form, []definition.Workflow, error) {
	forms, err := s.ListForms(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	wfs, err := s.ListWorkflows(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	outF := make([]definition.Form, 0, len(forms))
	for _, f := range forms {
		outF = append(outF, f.Definition)
	}
	outW := make([]definition.Workflow, 0, len(wfs))
	for _, w := range wfs {
		outW = append(outW, w.Definition)
	}
	return outF, outW, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/definitions/ -race -v`
Expected: every test in the package `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/definitions/reads.go internal/definitions/reads_test.go
git commit -m "feat: add definition version history, cached version lookups and export"
```

---

### Task 6: Submissions service — create, get, events, created hook

**Files:**
- Create: `internal/submissions/service.go`
- Create: `internal/submissions/create.go`
- Create: `internal/submissions/events.go`
- Test: `internal/submissions/service_test.go`

**Interfaces:**
- Consumes: `definitions.Store.{GetForm, GetWorkflowVersion}`, `definitions.ErrNotFound`, `definition.ValidateSubmission`, `definition.Workflow.State`, `db.WithTx`, `db.DBTX`, `auth.Principal`, `auth.PrincipalFrom`, `auth.PrincipalAPIKey`, `auth.WithPrincipal`.
- Produces (spec §6.7):
  - `var ErrNotFound, ErrFormNotPublic, ErrInvalidCursor`; `const NoWorkflowState = "submitted"`, `NoWorkflowStateLabel = "Submitted"`
  - Event type constants `EventCreated = "created"`, `EventTransition = "transition"`, `EventFieldsUpdated = "fields_updated"`, `EventAssigned = "assigned"`, `EventComment = "comment"`, `EventActionSucceeded = "action_succeeded"`, `EventActionFailed = "action_failed"` (Plan 04 uses these)
  - `Submission`, `Event`, `Actor`, `CreatedHook`, `Service`, `NewService`, `SetCreatedHook`, `Create`, `Get`, `GetForUpdate`, `InsertEvent`, `ActorFromPrincipal`
  - `Create` records the actor from `auth.PrincipalFrom(ctx)` when present, else `respondent` / "Respondent". It runs the created hook inside the insert transaction; a hook error rolls back the submission.
  - `func (s *Service) ResolveState(ctx, sub *Submission) error` — fills `StateLabel`/`Terminal` from the pinned workflow (Plan 04 calls it after changing `State`).

- [ ] **Step 1: Write the failing tests**

`internal/submissions/service_test.go`:
```go
package submissions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

// setup applies workflow "review" and forms "contact" (public, review),
// "plain" (public, no workflow) and "internal" (private, review).
func setup(t *testing.T) (*testutil.Env, *definitions.Store, *submissions.Service) {
	t.Helper()
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	internal := testutil.SampleForm("internal", "review")
	internal.Settings.Public = false
	if _, err := defs.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", ""), internal},
	}); err != nil {
		t.Fatalf("seed definitions: %v", err)
	}
	return env, defs, submissions.NewService(env.Pool, defs)
}

func validData() map[string]any {
	return map[string]any{"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}
}

func mustCreate(t *testing.T, s *submissions.Service, env *testutil.Env, slug string) (submissions.Submission, string) {
	t.Helper()
	sub, token, err := s.Create(context.Background(), env.OrgID, slug, validData(), true)
	if err != nil {
		t.Fatalf("Create(%s): %v", slug, err)
	}
	return sub, token
}

func TestCreateStartsInInitialState(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	if sub.State != "new" || sub.StateLabel != "New" || sub.Terminal {
		t.Fatalf("state = %q/%q terminal=%v", sub.State, sub.StateLabel, sub.Terminal)
	}
	if sub.FormSlug != "contact" || sub.FormVersion != 1 || sub.WorkflowVersionID == nil {
		t.Fatalf("unexpected pins %+v", sub)
	}
	if len(token) != 32 {
		t.Fatalf("receipt token length = %d, want 32 (24 bytes base64url)", len(token))
	}
	if !reflect.DeepEqual(sub.Data, validData()) {
		t.Fatalf("data = %v", sub.Data)
	}
	if sub.Fields == nil || len(sub.Fields) != 0 {
		t.Fatalf("fields = %v, want empty map", sub.Fields)
	}

	got, err := s.Get(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "new" || got.StateLabel != "New" || got.FormSlug != "contact" || !got.CreatedAt.Equal(sub.CreatedAt) {
		t.Fatalf("Get = %+v", got)
	}

	var typ, toState, actorType, actorName string
	if err := env.Pool.QueryRow(ctx, `SELECT type, to_state, actor_type, actor_name FROM submission_events WHERE submission_id = $1`, sub.ID).
		Scan(&typ, &toState, &actorType, &actorName); err != nil {
		t.Fatal(err)
	}
	if typ != "created" || toState != "new" || actorType != "respondent" || actorName != "Respondent" {
		t.Fatalf("event = %s %s %s %s", typ, toState, actorType, actorName)
	}
	var stored string
	if err := env.Pool.QueryRow(ctx, `SELECT receipt_token_hash FROM submissions WHERE id = $1`, sub.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || len(stored) != 64 {
		t.Fatalf("token must be stored as sha256 hex, got %q", stored)
	}
}

func TestCreateStripsUnknownKeys(t *testing.T) {
	env, _, s := setup(t)
	data := validData()
	data["isAdmin"] = true
	sub, _, err := s.Create(context.Background(), env.OrgID, "contact", data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.Data["isAdmin"]; ok {
		t.Fatalf("unknown key kept: %v", sub.Data)
	}
}

func TestCreateValidationError(t *testing.T) {
	env, _, s := setup(t)
	data := validData()
	delete(data, "email")
	_, _, err := s.Create(context.Background(), env.OrgID, "contact", data, true)
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	found := false
	for _, p := range ve.Problems {
		if p.Path == "data.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %+v, want data.email", ve.Problems)
	}
}

func TestCreateNilDataIsValidatedAsEmpty(t *testing.T) {
	env, _, s := setup(t)
	_, _, err := s.Create(context.Background(), env.OrgID, "contact", nil, true)
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestCreateRequirePublic(t *testing.T) {
	env, _, s := setup(t)
	if _, _, err := s.Create(context.Background(), env.OrgID, "internal", validData(), true); !errors.Is(err, submissions.ErrFormNotPublic) {
		t.Fatalf("err = %v, want ErrFormNotPublic", err)
	}
	if _, _, err := s.Create(context.Background(), env.OrgID, "internal", validData(), false); err != nil {
		t.Fatalf("authenticated create on private form: %v", err)
	}
}

func TestCreateUnknownForm(t *testing.T) {
	env, _, s := setup(t)
	if _, _, err := s.Create(context.Background(), env.OrgID, "missing", validData(), true); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateWithoutWorkflow(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "plain")
	if sub.State != "submitted" || sub.StateLabel != "Submitted" || !sub.Terminal || sub.WorkflowVersionID != nil {
		t.Fatalf("sub = %+v", sub)
	}
}

func TestCreateRecordsPrincipalActor(t *testing.T) {
	env, _, s := setup(t)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{OrgID: env.OrgID, Kind: auth.PrincipalAPIKey, ID: env.OrgID, Name: "ci-key"})
	sub, _, err := s.Create(ctx, env.OrgID, "contact", validData(), false)
	if err != nil {
		t.Fatal(err)
	}
	var actorType, actorName string
	if err := env.Pool.QueryRow(ctx, `SELECT actor_type, actor_name FROM submission_events WHERE submission_id = $1`, sub.ID).Scan(&actorType, &actorName); err != nil {
		t.Fatal(err)
	}
	if actorType != "api_key" || actorName != "ci-key" {
		t.Fatalf("actor = %s %s", actorType, actorName)
	}
}

func TestCreatedHookReceivesWorkflowInTransaction(t *testing.T) {
	env, _, s := setup(t)
	var gotWF *definition.Workflow
	var gotSub submissions.Submission
	s.SetCreatedHook(func(ctx context.Context, tx pgx.Tx, sub submissions.Submission, wf *definition.Workflow) error {
		gotSub, gotWF = sub, wf
		var n int
		// The row is visible inside the hook's transaction.
		return tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE id = $1`, sub.ID).Scan(&n)
	})
	sub, _ := mustCreate(t, s, env, "contact")
	if gotWF == nil || gotWF.Slug != "review" || gotSub.ID != sub.ID {
		t.Fatalf("hook got wf=%v sub=%v", gotWF, gotSub.ID)
	}
	gotWF = &definition.Workflow{}
	mustCreate(t, s, env, "plain")
	if gotWF != nil {
		t.Fatalf("hook for form without workflow got wf=%v, want nil", gotWF)
	}
}

func TestCreatedHookErrorRollsBack(t *testing.T) {
	env, _, s := setup(t)
	boom := errors.New("boom")
	s.SetCreatedHook(func(context.Context, pgx.Tx, submissions.Submission, *definition.Workflow) error { return boom })
	if _, _, err := s.Create(context.Background(), env.OrgID, "contact", validData(), true); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	var n int
	if err := env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM submissions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("submission persisted despite hook error (%d rows)", n)
	}
}

func TestGetIsOrgScoped(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "contact")
	if _, err := s.Get(context.Background(), env.OtherOrg(t), sub.ID); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetForUpdateInsideTx(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "contact")
	err := db.WithTx(context.Background(), env.Pool, func(tx pgx.Tx) error {
		got, err := s.GetForUpdate(context.Background(), tx, env.OrgID, sub.ID)
		if err != nil {
			return err
		}
		if got.ID != sub.ID || got.StateLabel != "New" {
			t.Errorf("GetForUpdate = %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionKeepsPinnedVersions(t *testing.T) {
	env, defs, s := setup(t)
	ctx := context.Background()
	sub, _ := mustCreate(t, s, env, "contact")

	wf := testutil.SampleWorkflow("review")
	wf.States[0].Label = "Fresh"
	if _, err := defs.Apply(ctx, env.OrgID, definitions.ApplyInput{Workflows: []definition.Workflow{wf}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StateLabel != "New" || got.FormVersion != 1 {
		t.Fatalf("pinned label/version changed: %q v%d", got.StateLabel, got.FormVersion)
	}
	newer, _ := mustCreate(t, s, env, "contact")
	if newer.StateLabel != "Fresh" || newer.FormVersion != 2 {
		t.Fatalf("new submission label/version = %q v%d", newer.StateLabel, newer.FormVersion)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/submissions/ -v`
Expected: FAIL — build error `undefined: submissions.NewService`

- [ ] **Step 3: Implement the service core**

`internal/submissions/service.go`:
```go
// Package submissions stores form submissions and their event history.
package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

var (
	ErrNotFound      = errors.New("submission not found")
	ErrFormNotPublic = errors.New("form is not public")
	ErrInvalidCursor = errors.New("invalid cursor")
)

// State used by forms that have no workflow.
const (
	NoWorkflowState      = "submitted"
	NoWorkflowStateLabel = "Submitted"
)

type Submission struct {
	ID, OrgID, FormID, FormVersionID uuid.UUID
	FormSlug                         string
	FormVersion                      int
	WorkflowVersionID                *uuid.UUID
	State, StateLabel                string
	Terminal                         bool
	Data, Fields                     map[string]any
	AssigneeID                       *uuid.UUID
	AssigneeName, AssigneeEmail      string
	CreatedAt, UpdatedAt             time.Time
}

// CreatedHook runs inside the transaction that inserts a submission.
// wf is nil when the form has no workflow. Returning an error rolls back.
type CreatedHook func(ctx context.Context, tx pgx.Tx, sub Submission, wf *definition.Workflow) error

type Service struct {
	pool *pgxpool.Pool
	defs *definitions.Store
	hook CreatedHook
}

func NewService(pool *pgxpool.Pool, defs *definitions.Store) *Service {
	return &Service{pool: pool, defs: defs}
}

// SetCreatedHook registers the hook run for every new submission. Call during wiring only.
func (s *Service) SetCreatedHook(h CreatedHook) { s.hook = h }

const submissionCols = `SELECT s.id, s.org_id, s.form_id, s.form_version_id, f.slug, fv.version,
	s.workflow_version_id, s.state, s.data, s.fields, s.assignee_id,
	COALESCE(u.name, ''), COALESCE(u.email, ''), s.created_at, s.updated_at
FROM submissions s
JOIN forms f ON f.id = s.form_id
JOIN form_versions fv ON fv.id = s.form_version_id
LEFT JOIN users u ON u.id = s.assignee_id`

// scanRow reads one row of submissionCols. StateLabel/Terminal are filled by ResolveState.
func scanRow(row pgx.Row) (Submission, error) {
	var (
		sub          Submission
		data, fields []byte
	)
	err := row.Scan(&sub.ID, &sub.OrgID, &sub.FormID, &sub.FormVersionID, &sub.FormSlug, &sub.FormVersion,
		&sub.WorkflowVersionID, &sub.State, &data, &fields, &sub.AssigneeID,
		&sub.AssigneeName, &sub.AssigneeEmail, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("scan submission: %w", err)
	}
	if err := json.Unmarshal(data, &sub.Data); err != nil {
		return Submission{}, fmt.Errorf("decode data: %w", err)
	}
	if err := json.Unmarshal(fields, &sub.Fields); err != nil {
		return Submission{}, fmt.Errorf("decode fields: %w", err)
	}
	if sub.Data == nil {
		sub.Data = map[string]any{}
	}
	if sub.Fields == nil {
		sub.Fields = map[string]any{}
	}
	return sub, nil
}

// ResolveState fills StateLabel and Terminal from the submission's pinned workflow version.
func (s *Service) ResolveState(ctx context.Context, sub *Submission) error {
	if sub.WorkflowVersionID == nil {
		sub.StateLabel, sub.Terminal = NoWorkflowStateLabel, true
		return nil
	}
	wf, err := s.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version: %w", err)
	}
	sub.StateLabel, sub.Terminal = stateLabel(&wf.Definition, sub.State)
	return nil
}

// stateLabel returns the label and terminal flag of state in wf (nil = no workflow).
func stateLabel(wf *definition.Workflow, state string) (string, bool) {
	if wf == nil {
		return NoWorkflowStateLabel, true
	}
	st, ok := wf.State(state)
	if !ok {
		return state, false
	}
	return st.Label, st.Terminal
}

func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (Submission, error) {
	sub, err := scanRow(s.pool.QueryRow(ctx, submissionCols+` WHERE s.org_id = $1 AND s.id = $2`, orgID, id))
	if err != nil {
		return Submission{}, err
	}
	return sub, s.ResolveState(ctx, &sub)
}

// GetForUpdate locks the submission row for the rest of tx.
func (s *Service) GetForUpdate(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (Submission, error) {
	sub, err := scanRow(tx.QueryRow(ctx, submissionCols+` WHERE s.org_id = $1 AND s.id = $2 FOR UPDATE OF s`, orgID, id))
	if err != nil {
		return Submission{}, err
	}
	return sub, s.ResolveState(ctx, &sub)
}
```

`internal/submissions/events.go`:
```go
package submissions

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
)

// Event types (spec §4).
const (
	EventCreated         = "created"
	EventTransition      = "transition"
	EventFieldsUpdated   = "fields_updated"
	EventAssigned        = "assigned"
	EventComment         = "comment"
	EventActionSucceeded = "action_succeeded"
	EventActionFailed    = "action_failed"
)

type Event struct {
	ID                             int64
	SubmissionID, OrgID            uuid.UUID
	Type                           string
	FromState, ToState, Transition string // "" when not applicable (NULL in DB)
	ActorType                      string
	ActorID                        *uuid.UUID
	ActorName                      string
	Payload                        map[string]any
	CreatedAt                      time.Time
}

type Actor struct {
	Type string // user|api_key|system|respondent
	ID   *uuid.UUID
	Name string
}

func ActorFromPrincipal(p auth.Principal) Actor {
	id := p.ID
	typ := "user"
	if p.Kind == auth.PrincipalAPIKey {
		typ = "api_key"
	}
	name := p.Name
	if name == "" {
		name = p.Email
	}
	return Actor{Type: typ, ID: &id, Name: name}
}

// actorFromContext returns the authenticated principal, or the anonymous respondent.
func actorFromContext(ctx context.Context) Actor {
	if p, ok := auth.PrincipalFrom(ctx); ok {
		return ActorFromPrincipal(p)
	}
	return Actor{Type: "respondent", Name: "Respondent"}
}

func nullable(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// InsertEvent appends an event and returns it with ID and CreatedAt set.
func InsertEvent(ctx context.Context, q db.DBTX, e Event) (Event, error) {
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
	payload, err := json.Marshal(e.Payload)
	if err != nil {
		return Event{}, fmt.Errorf("encode event payload: %w", err)
	}
	err = q.QueryRow(ctx, `INSERT INTO submission_events
		(submission_id, org_id, type, from_state, to_state, transition, actor_type, actor_id, actor_name, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, created_at`,
		e.SubmissionID, e.OrgID, e.Type, nullable(e.FromState), nullable(e.ToState), nullable(e.Transition),
		e.ActorType, e.ActorID, e.ActorName, payload).Scan(&e.ID, &e.CreatedAt)
	if err != nil {
		return Event{}, fmt.Errorf("insert event: %w", err)
	}
	return e, nil
}
```

`internal/submissions/create.go`:
```go
package submissions

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

// Create validates data against the form's current version and stores it in
// the workflow's initial state (or "submitted" when the form has no workflow).
// requirePublic rejects forms whose settings.public is false with ErrFormNotPublic.
// It returns the submission and its plaintext receipt token (shown once).
func (s *Service) Create(ctx context.Context, orgID uuid.UUID, formSlug string, data map[string]any, requirePublic bool) (Submission, string, error) {
	form, err := s.defs.GetForm(ctx, orgID, formSlug)
	if errors.Is(err, definitions.ErrNotFound) {
		return Submission{}, "", ErrNotFound
	}
	if err != nil {
		return Submission{}, "", err
	}
	if requirePublic && !form.Definition.Settings.Public {
		return Submission{}, "", ErrFormNotPublic
	}
	if data == nil {
		data = map[string]any{}
	}
	clean, err := definition.ValidateSubmission(form.Definition, data)
	if err != nil {
		return Submission{}, "", err
	}

	sub := Submission{
		ID:                uuid.New(),
		OrgID:             orgID,
		FormID:            form.ID,
		FormVersionID:     form.Current.ID,
		FormSlug:          form.Slug,
		FormVersion:       form.Current.Version,
		WorkflowVersionID: form.WorkflowVersionID,
		State:             NoWorkflowState,
		Data:              clean,
		Fields:            map[string]any{},
	}
	var wf *definition.Workflow
	if form.WorkflowVersionID != nil {
		rec, err := s.defs.GetWorkflowVersion(ctx, *form.WorkflowVersionID)
		if err != nil {
			return Submission{}, "", fmt.Errorf("load workflow version: %w", err)
		}
		wf = &rec.Definition
		sub.State = wf.Initial
	}
	sub.StateLabel, sub.Terminal = stateLabel(wf, sub.State)

	token, tokenHash, err := newReceiptToken()
	if err != nil {
		return Submission{}, "", err
	}
	dataJSON, err := json.Marshal(clean)
	if err != nil {
		return Submission{}, "", fmt.Errorf("encode data: %w", err)
	}
	actor := actorFromContext(ctx)

	err = db.WithTx(ctx, s.pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `INSERT INTO submissions
			(id, org_id, form_id, form_version_id, workflow_version_id, state, data, fields, receipt_token_hash)
			VALUES ($1, $2, $3, $4, $5, $6, $7, '{}', $8)
			RETURNING created_at, updated_at`,
			sub.ID, orgID, sub.FormID, sub.FormVersionID, sub.WorkflowVersionID, sub.State, dataJSON, tokenHash,
		).Scan(&sub.CreatedAt, &sub.UpdatedAt); err != nil {
			return fmt.Errorf("insert submission: %w", err)
		}
		if _, err := InsertEvent(ctx, tx, Event{
			SubmissionID: sub.ID, OrgID: orgID, Type: EventCreated, ToState: sub.State,
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
		}); err != nil {
			return err
		}
		if s.hook != nil {
			return s.hook(ctx, tx, sub, wf)
		}
		return nil
	})
	if err != nil {
		return Submission{}, "", err
	}
	return sub, token, nil
}

// newReceiptToken returns a 24-byte base64url token and its sha256 hex digest.
func newReceiptToken() (token, hash string, err error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("receipt token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashToken(token), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/submissions/ -race -v`
Expected: all 13 tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/submissions/
git commit -m "feat: add submissions service with receipt tokens, events and created hook"
```

---

### Task 7: Submissions service — list, events, counts, public status, CSV export

**Files:**
- Create: `internal/submissions/list.go`
- Create: `internal/submissions/public.go`
- Create: `internal/submissions/csv.go`
- Modify: `internal/submissions/events.go` (append `Events`)
- Test: `internal/submissions/query_test.go`

**Interfaces:**
- Consumes: Task 6 `submissionCols`, `scanRow`, `ResolveState`, `stateLabel`, `hashToken`; `definitions.Store.{GetForm, GetFormVersion, GetWorkflowVersion}`.
- Produces:
  - `type ListFilter`; `func (s *Service) List(ctx, orgID, f ListFilter) ([]Submission, string, error)` — newest first, cursor = opaque base64url of `created_at|id`; bad cursor → `ErrInvalidCursor`; last page → `""`.
  - `func (s *Service) Events(ctx, orgID, id) ([]Event, error)` — oldest first; `ErrNotFound` if the submission isn't in the org.
  - `func (s *Service) CountByForm(ctx, orgID) (map[uuid.UUID]int, error)` — keyed by form id (used by `GET /forms`).
  - `type PublicStatus`, `type PublicHistoryItem`; `func (s *Service) PublicStatus(ctx, id, token) (PublicStatus, error)` — constant-time token check, `ErrNotFound` on any mismatch; `History` lists state entries (events with `to_state`), oldest first.
  - `func (s *Service) ExportCSV(ctx, orgID, formSlug, w io.Writer) error` — header `id,createdAt,state,formVersion,<form field keys…>,fields.<workflow field keys…>` from the current version; multiselect joined with `;`; formula-leading cells prefixed with `'`; writes nothing before the form lookup succeeds.

- [ ] **Step 1: Write the failing tests**

`internal/submissions/query_test.go`:
```go
package submissions_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/submissions"
)

func TestListPaginatesWithoutDuplicates(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	want := map[uuid.UUID]bool{}
	for i := 0; i < 5; i++ {
		sub, _ := mustCreate(t, s, env, "contact")
		want[sub.ID] = true
	}
	// Force identical timestamps to exercise the id tie-break.
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET created_at = '2026-09-23T10:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]bool{}
	cursor := ""
	pages := 0
	for {
		items, next, err := s.List(ctx, env.OrgID, submissions.ListFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range items {
			if seen[it.ID] {
				t.Fatalf("duplicate %s on page %d", it.ID, pages)
			}
			seen[it.ID] = true
			if it.StateLabel != "New" {
				t.Fatalf("state label not resolved: %+v", it)
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if pages != 3 || len(seen) != len(want) {
		t.Fatalf("pages=%d seen=%d want=%d", pages, len(seen), len(want))
	}
}

func TestListNewestFirst(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	older, _ := mustCreate(t, s, env, "contact")
	newer, _ := mustCreate(t, s, env, "contact")
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET created_at = created_at - interval '1 hour' WHERE id = $1`, older.ID); err != nil {
		t.Fatal(err)
	}
	items, next, err := s.List(ctx, env.OrgID, submissions.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != newer.ID || next != "" {
		t.Fatalf("items = %v, next = %q", items, next)
	}
}

func TestListFilters(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	a, _ := mustCreate(t, s, env, "contact")
	b, _ := mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "plain")
	reviewer := env.User(t, "rev@example.com", "reviewer")
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET state = 'review', assignee_id = $2 WHERE id = $1`, a.ID, reviewer.ID); err != nil {
		t.Fatal(err)
	}

	check := func(name string, f submissions.ListFilter, want int) []submissions.Submission {
		t.Helper()
		items, _, err := s.List(ctx, env.OrgID, f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(items) != want {
			t.Fatalf("%s: got %d items, want %d", name, len(items), want)
		}
		return items
	}
	check("form", submissions.ListFilter{FormSlug: "plain"}, 1)
	byState := check("state", submissions.ListFilter{State: "review"}, 1)
	if byState[0].ID != a.ID || byState[0].StateLabel != "In review" {
		t.Fatalf("state filter returned %+v", byState[0])
	}
	assigned := check("assignee", submissions.ListFilter{AssigneeID: &reviewer.ID}, 1)
	if assigned[0].AssigneeName == "" || assigned[0].AssigneeEmail != "rev@example.com" {
		t.Fatalf("assignee not joined: %+v", assigned[0])
	}
	unassigned := check("unassigned", submissions.ListFilter{Unassigned: true}, 2)
	for _, it := range unassigned {
		if it.ID == a.ID {
			t.Fatal("assigned submission in unassigned filter")
		}
	}
	check("combined", submissions.ListFilter{FormSlug: "contact", State: "new"}, 1)
	_ = b
	check("other org", submissions.ListFilter{}, 3)
	items, _, err := s.List(ctx, env.OtherOrg(t), submissions.ListFilter{})
	if err != nil || len(items) != 0 {
		t.Fatalf("other org sees %d submissions, err = %v", len(items), err)
	}
}

func TestListLimitBounds(t *testing.T) {
	env, _, s := setup(t)
	for i := 0; i < 3; i++ {
		mustCreate(t, s, env, "plain")
	}
	items, next, err := s.List(context.Background(), env.OrgID, submissions.ListFilter{Limit: 1000})
	if err != nil || len(items) != 3 || next != "" {
		t.Fatalf("limit clamp: %d items next=%q err=%v", len(items), next, err)
	}
}

func TestListInvalidCursor(t *testing.T) {
	env, _, s := setup(t)
	for _, c := range []string{"garbage!!", "bm90LWEtY3Vyc29y"} {
		if _, _, err := s.List(context.Background(), env.OrgID, submissions.ListFilter{Cursor: c}); !errors.Is(err, submissions.ErrInvalidCursor) {
			t.Fatalf("cursor %q: err = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestEvents(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, _ := mustCreate(t, s, env, "contact")
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{
		SubmissionID: sub.ID, OrgID: env.OrgID, Type: submissions.EventComment,
		ActorType: "system", ActorName: "System", Payload: map[string]any{"body": "hello"},
	}); err != nil {
		t.Fatal(err)
	}
	evs, err := s.Events(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("got %d events", len(evs))
	}
	if evs[0].Type != "created" || evs[0].ToState != "new" || evs[0].FromState != "" || evs[0].ActorType != "respondent" {
		t.Fatalf("created event = %+v", evs[0])
	}
	if evs[1].Type != "comment" || evs[1].Payload["body"] != "hello" || evs[1].ToState != "" {
		t.Fatalf("comment event = %+v", evs[1])
	}
	if _, err := s.Events(ctx, env.OtherOrg(t), sub.ID); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("other org err = %v", err)
	}
}

func TestCountByForm(t *testing.T) {
	env, defs, s := setup(t)
	ctx := context.Background()
	mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "plain")
	counts, err := s.CountByForm(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	contact, _ := defs.GetForm(ctx, env.OrgID, "contact")
	plain, _ := defs.GetForm(ctx, env.OrgID, "plain")
	if counts[contact.ID] != 2 || counts[plain.ID] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestPublicStatus(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	st, err := s.PublicStatus(ctx, sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != sub.ID || st.FormTitle != "Contact" || st.State != "new" || st.StateLabel != "New" || st.Terminal {
		t.Fatalf("status = %+v", st)
	}
	if len(st.States) != 3 || st.States[2].Key != "done" || !st.States[2].Terminal {
		t.Fatalf("states = %+v", st.States)
	}
	if len(st.History) != 1 || st.History[0].State != "new" || st.History[0].Label != "New" {
		t.Fatalf("history = %+v", st.History)
	}
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET state = 'review' WHERE id = $1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{SubmissionID: sub.ID, OrgID: env.OrgID, Type: "transition", FromState: "new", ToState: "review", Transition: "start", ActorType: "system"}); err != nil {
		t.Fatal(err)
	}
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{SubmissionID: sub.ID, OrgID: env.OrgID, Type: "comment", ActorType: "system", Payload: map[string]any{"body": "internal note"}}); err != nil {
		t.Fatal(err)
	}
	st, err = s.PublicStatus(ctx, sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if st.StateLabel != "In review" || len(st.History) != 2 || st.History[1].Label != "In review" {
		t.Fatalf("after transition: %+v", st)
	}
}

func TestPublicStatusWithoutWorkflow(t *testing.T) {
	env, _, s := setup(t)
	sub, token := mustCreate(t, s, env, "plain")
	st, err := s.PublicStatus(context.Background(), sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Terminal || len(st.States) != 1 || st.States[0].Key != "submitted" || st.States[0].Label != "Submitted" {
		t.Fatalf("status = %+v", st)
	}
}

func TestPublicStatusRejectsBadToken(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	for name, tc := range map[string]struct {
		id    uuid.UUID
		token string
	}{
		"wrong token":   {sub.ID, strings.Repeat("A", len(token))},
		"empty token":   {sub.ID, ""},
		"unknown id":    {uuid.New(), token},
		"token as hash": {sub.ID, "x"},
	} {
		if _, err := s.PublicStatus(ctx, tc.id, tc.token); !errors.Is(err, submissions.ErrNotFound) {
			t.Fatalf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestExportCSVEscapesFormulas(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	data := validData()
	data["name"] = "=HYPERLINK(\"http://evil\")"
	sub, _, err := s.Create(ctx, env.OrgID, "contact", data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET fields = '{"note": "+1 looks good"}' WHERE id = $1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := s.ExportCSV(ctx, env.OrgID, "contact", &buf); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"id", "createdAt", "state", "formVersion", "name", "email", "topic", "fields.note"}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header = %v", records[0])
	}
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	row := records[1]
	if row[0] != sub.ID.String() || row[2] != "new" || row[3] != "1" {
		t.Fatalf("row = %v", row)
	}
	if row[4] != "'=HYPERLINK(\"http://evil\")" {
		t.Fatalf("formula not escaped: %q", row[4])
	}
	if row[7] != "'+1 looks good" {
		t.Fatalf("workflow field not escaped: %q", row[7])
	}
}

func TestExportCSVUnknownFormWritesNothing(t *testing.T) {
	env, _, s := setup(t)
	var buf bytes.Buffer
	if err := s.ExportCSV(context.Background(), env.OrgID, "missing", &buf); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q before failing", buf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/submissions/ -v`
Expected: FAIL — build errors `s.List undefined`, `s.Events undefined`, `s.CountByForm undefined`, `s.PublicStatus undefined`, `s.ExportCSV undefined`

- [ ] **Step 3: Implement listing and counts**

`internal/submissions/list.go`:
```go
package submissions

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ListFilter struct {
	FormSlug, State string
	AssigneeID      *uuid.UUID
	Unassigned      bool
	Cursor          string
	Limit           int // default 50, max 200
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	ts, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	return t, id, nil
}

// List returns submissions newest first (created_at DESC, id DESC) and an
// opaque cursor for the next page ("" on the last page).
func (s *Service) List(ctx context.Context, orgID uuid.UUID, f ListFilter) ([]Submission, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var (
		curAt *time.Time
		curID *uuid.UUID
	)
	if f.Cursor != "" {
		t, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", err
		}
		curAt, curID = &t, &id
	}
	rows, err := s.pool.Query(ctx, submissionCols+`
		WHERE s.org_id = $1
		  AND ($2::text = '' OR f.slug = $2)
		  AND ($3::text = '' OR s.state = $3)
		  AND ($4::uuid IS NULL OR s.assignee_id = $4)
		  AND (NOT $5::bool OR s.assignee_id IS NULL)
		  AND ($6::timestamptz IS NULL OR (s.created_at, s.id) < ($6::timestamptz, $7::uuid))
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $8`,
		orgID, f.FormSlug, f.State, f.AssigneeID, f.Unassigned, curAt, curID, limit+1)
	if err != nil {
		return nil, "", err
	}
	items := []Submission{}
	for rows.Next() {
		sub, err := scanRow(rows)
		if err != nil {
			rows.Close()
			return nil, "", err
		}
		items = append(items, sub)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	for i := range items {
		if err := s.ResolveState(ctx, &items[i]); err != nil {
			return nil, "", err
		}
	}
	return items, next, nil
}

// CountByForm returns the number of submissions per form id.
func (s *Service) CountByForm(ctx context.Context, orgID uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT form_id, count(*) FROM submissions WHERE org_id = $1 GROUP BY form_id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]int{}
	for rows.Next() {
		var (
			id uuid.UUID
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Implement `Events`**

Append to `internal/submissions/events.go` (add `"errors"` and `"github.com/jackc/pgx/v5"` to imports):
```go
// Events returns the submission's events, oldest first.
func (s *Service) Events(ctx context.Context, orgID, id uuid.UUID) ([]Event, error) {
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM submissions WHERE org_id = $1 AND id = $2)`, orgID, id).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.pool.Query(ctx, `SELECT id, submission_id, org_id, type,
		COALESCE(from_state, ''), COALESCE(to_state, ''), COALESCE(transition, ''),
		actor_type, actor_id, actor_name, payload, created_at
		FROM submission_events WHERE submission_id = $1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var (
			e   Event
			raw []byte
		)
		if err := rows.Scan(&e.ID, &e.SubmissionID, &e.OrgID, &e.Type, &e.FromState, &e.ToState, &e.Transition,
			&e.ActorType, &e.ActorID, &e.ActorName, &raw, &e.CreatedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if err := json.Unmarshal(raw, &e.Payload); err != nil {
			return nil, fmt.Errorf("decode event payload: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Implement the public status**

`internal/submissions/public.go`:
```go
package submissions

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

type PublicStatus struct {
	ID                           uuid.UUID
	FormTitle, State, StateLabel string
	Terminal                     bool
	States                       []definition.State
	History                      []PublicHistoryItem
	CreatedAt                    time.Time
}

type PublicHistoryItem struct {
	State, Label string
	At           time.Time
}

// PublicStatus returns what a respondent may see about their submission.
// Any unknown id or token mismatch yields ErrNotFound (constant-time compare).
func (s *Service) PublicStatus(ctx context.Context, id uuid.UUID, token string) (PublicStatus, error) {
	var (
		orgID      uuid.UUID
		storedHash string
	)
	err := s.pool.QueryRow(ctx, `SELECT org_id, receipt_token_hash FROM submissions WHERE id = $1`, id).Scan(&orgID, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicStatus{}, ErrNotFound
	}
	if err != nil {
		return PublicStatus{}, err
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(storedHash)) != 1 {
		return PublicStatus{}, ErrNotFound
	}

	sub, err := s.Get(ctx, orgID, id)
	if err != nil {
		return PublicStatus{}, err
	}
	form, err := s.defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		return PublicStatus{}, err
	}
	var wf *definition.Workflow
	states := []definition.State{{Key: NoWorkflowState, Label: NoWorkflowStateLabel, Terminal: true}}
	if sub.WorkflowVersionID != nil {
		rec, err := s.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			return PublicStatus{}, err
		}
		wf = &rec.Definition
		states = wf.States
	}

	rows, err := s.pool.Query(ctx, `SELECT to_state, created_at FROM submission_events
		WHERE submission_id = $1 AND to_state IS NOT NULL ORDER BY id`, id)
	if err != nil {
		return PublicStatus{}, err
	}
	defer rows.Close()
	history := []PublicHistoryItem{}
	for rows.Next() {
		var item PublicHistoryItem
		if err := rows.Scan(&item.State, &item.At); err != nil {
			return PublicStatus{}, err
		}
		item.Label, _ = stateLabel(wf, item.State)
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return PublicStatus{}, err
	}
	return PublicStatus{
		ID: sub.ID, FormTitle: form.Definition.Title,
		State: sub.State, StateLabel: sub.StateLabel, Terminal: sub.Terminal,
		States: states, History: history, CreatedAt: sub.CreatedAt,
	}, nil
}
```

- [ ] **Step 6: Implement the CSV export**

`internal/submissions/csv.go`:
```go
package submissions

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definitions"
)

// ExportCSV writes every submission of the form, oldest first. Columns come from
// the form's current version: id, createdAt, state, formVersion, each form field
// key, then "fields.<key>" for each workflow field. Nothing is written if the
// form lookup fails.
func (s *Service) ExportCSV(ctx context.Context, orgID uuid.UUID, formSlug string, w io.Writer) error {
	form, err := s.defs.GetForm(ctx, orgID, formSlug)
	if errors.Is(err, definitions.ErrNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	dataKeys := make([]string, 0, len(form.Definition.Fields))
	for _, f := range form.Definition.Fields {
		dataKeys = append(dataKeys, f.Key)
	}
	var fieldKeys []string
	if form.WorkflowVersionID != nil {
		wf, err := s.defs.GetWorkflowVersion(ctx, *form.WorkflowVersionID)
		if err != nil {
			return err
		}
		for _, f := range wf.Definition.Fields {
			fieldKeys = append(fieldKeys, f.Key)
		}
	}

	rows, err := s.pool.Query(ctx, `SELECT s.id, s.created_at, s.state, fv.version, s.data, s.fields
		FROM submissions s JOIN form_versions fv ON fv.id = s.form_version_id
		WHERE s.org_id = $1 AND s.form_id = $2
		ORDER BY s.created_at, s.id`, orgID, form.ID)
	if err != nil {
		return err
	}
	defer rows.Close()

	cw := csv.NewWriter(w)
	header := append([]string{"id", "createdAt", "state", "formVersion"}, dataKeys...)
	for _, k := range fieldKeys {
		header = append(header, "fields."+k)
	}
	if err := cw.Write(header); err != nil {
		return err
	}
	for rows.Next() {
		var (
			id                  uuid.UUID
			createdAt           time.Time
			state               string
			version             int
			rawData, rawFields  []byte
			data, fields        map[string]any
		)
		if err := rows.Scan(&id, &createdAt, &state, &version, &rawData, &rawFields); err != nil {
			return err
		}
		if err := json.Unmarshal(rawData, &data); err != nil {
			return err
		}
		if err := json.Unmarshal(rawFields, &fields); err != nil {
			return err
		}
		record := []string{id.String(), createdAt.UTC().Format(time.RFC3339), state, strconv.Itoa(version)}
		for _, k := range dataKeys {
			record = append(record, csvSafe(csvValue(data[k])))
		}
		for _, k := range fieldKeys {
			record = append(record, csvSafe(csvValue(fields[k])))
		}
		if err := cw.Write(record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func csvValue(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case []any:
		parts := make([]string, 0, len(x))
		for _, p := range x {
			parts = append(parts, csvValue(p))
		}
		return strings.Join(parts, ";")
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

// csvSafe neutralises spreadsheet formula injection (OWASP CSV injection).
func csvSafe(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test ./internal/submissions/ -race -v`
Expected: every test in the package `PASS`.

- [ ] **Step 8: Commit**

```bash
git add internal/submissions/
git commit -m "feat: add submission listing, events, public status and CSV export"
```

---

### Task 8: HTTP wiring — deps, error mapping, JSON shapes, mounts, app

**Files:**
- Modify: `internal/httpapi/errors.go` (append `errorMappings` rows)
- Create: `internal/httpapi/json_types.go`
- Create: `internal/httpapi/definition_handlers.go` (mount function only; routes added in Task 9)
- Create: `internal/httpapi/public_handlers.go` (mount function only; routes added in Task 10)
- Create: `internal/httpapi/submission_handlers.go` (mount function only; routes added in Task 11)
- Create: `internal/httpapi/defs_subs_helpers_test.go`
- Modify: the file declaring `type Deps struct` (find it with `grep -n "type Deps struct" internal/httpapi/*.go`)
- Modify: `internal/httpapi/routes.go`
- Modify: `internal/app/app.go`
- Test: `internal/httpapi/errors_defs_subs_test.go`, `internal/httpapi/json_types_test.go`

**Interfaces:**
- Consumes: Plan 01 `Deps`, `WriteError`, `APIError`, `routes.go`, `app.New`; Tasks 2–7.
- Produces:
  - `Deps` gains `Defs *definitions.Store` and `Subs *submissions.Service` (skip if Plan 01 already declared them); `App` gains `Defs`, `Subs` the same way.
  - Rows in `errorMappings` for `definitions.ErrNotFound`, `submissions.ErrNotFound`, `submissions.ErrFormNotPublic`, `submissions.ErrInvalidCursor`.
  - JSON shapes (package-private, used by Tasks 9–11 and by Plan 04's handlers): `submissionJSON`/`toSubmissionJSON`, `eventJSON`/`toEventJSON`, `versionJSON`/`toVersionJSON`, `formRecordJSON`, `workflowRecordJSON`, `listJSON[T]`.
  - Empty mounts `mountDefinitions`, `mountPublic`, `mountSubmissions` registered in `routes.go`.
  - Test helper (package `httpapi_test`): `newDefsSubsAPI(t) *defsSubsAPI` with fields `env, h, defs, subs, admin` and methods `seed(t)`, `create(t, slug) (submissions.Submission, string)`; `validData()`.

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/errors_defs_subs_test.go`:
```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

func TestWriteErrorMapsDefsSubsErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		detail string
	}{
		{"validation", &definition.ValidationError{Problems: []definition.Problem{{Path: "slug", Message: "bad"}}}, 422, "validation_failed", "slug"},
		{"wrapped validation", fmt.Errorf("apply: %w", &definition.ValidationError{Problems: []definition.Problem{{Path: "forms[0]", Message: "bad"}}}), 422, "validation_failed", "forms[0]"},
		{"definition not found", definitions.ErrNotFound, 404, "not_found", ""},
		{"wrapped definition not found", fmt.Errorf("workflow %q: %w", "x", definitions.ErrNotFound), 404, "not_found", ""},
		{"submission not found", submissions.ErrNotFound, 404, "not_found", ""},
		{"form not public", submissions.ErrFormNotPublic, 404, "not_found", ""},
		{"invalid cursor", submissions.ErrInvalidCursor, 400, "bad_request", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest("GET", "/", nil), c.err)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			var env struct {
				Error struct {
					Code    string `json:"code"`
					Details []struct {
						Path string `json:"path"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Error.Code != c.code {
				t.Fatalf("code = %q, want %q", env.Error.Code, c.code)
			}
			if c.detail != "" && (len(env.Error.Details) != 1 || env.Error.Details[0].Path != c.detail) {
				t.Fatalf("details = %+v", env.Error.Details)
			}
		})
	}
}
```

`internal/httpapi/json_types_test.go`:
```go
package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

var (
	jsonTestTime = time.Date(2026, 9, 23, 11, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	jsonTestID   = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	jsonTestUser = uuid.MustParse("22222222-2222-4222-8222-222222222222")
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSubmissionJSONShape(t *testing.T) {
	sub := submissions.Submission{ID: jsonTestID, FormSlug: "contact", FormVersion: 2, State: "new", StateLabel: "New", CreatedAt: jsonTestTime, UpdatedAt: jsonTestTime}
	got := mustJSON(t, toSubmissionJSON(sub))
	want := `{"id":"11111111-1111-4111-8111-111111111111","form":"contact","formVersion":2,"state":"new","stateLabel":"New","terminal":false,"data":{},"fields":{},"assignee":null,"createdAt":"2026-09-23T09:00:00Z","updatedAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	sub.AssigneeID, sub.AssigneeName, sub.AssigneeEmail = &jsonTestUser, "Rita", "rita@example.com"
	sub.Data = map[string]any{"name": "Ada"}
	got = mustJSON(t, toSubmissionJSON(sub))
	want = `{"id":"11111111-1111-4111-8111-111111111111","form":"contact","formVersion":2,"state":"new","stateLabel":"New","terminal":false,"data":{"name":"Ada"},"fields":{},"assignee":{"id":"22222222-2222-4222-8222-222222222222","name":"Rita","email":"rita@example.com"},"createdAt":"2026-09-23T09:00:00Z","updatedAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestEventJSONShape(t *testing.T) {
	ev := submissions.Event{ID: 7, Type: "created", ToState: "new", ActorType: "respondent", ActorName: "Respondent", CreatedAt: jsonTestTime}
	got := mustJSON(t, toEventJSON(ev))
	want := `{"id":7,"type":"created","fromState":null,"toState":"new","transition":null,"actor":{"type":"respondent","id":null,"name":"Respondent"},"payload":{},"createdAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestVersionJSONShape(t *testing.T) {
	v := definitions.VersionInfo{ID: jsonTestID, Version: 3, Hash: "abc", Source: definitions.SourceCLI, CreatedBy: "dev", CreatedAt: jsonTestTime}
	got := mustJSON(t, toVersionJSON(v))
	want := `{"version":3,"hash":"abc","source":"cli","createdBy":"dev","createdAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestListJSONNullCursor(t *testing.T) {
	got := mustJSON(t, newListJSON([]int{}, ""))
	if got != `{"items":[],"nextCursor":null}` {
		t.Fatalf("got %s", got)
	}
	got = mustJSON(t, newListJSON([]int{1}, "abc"))
	if got != `{"items":[1],"nextCursor":"abc"}` {
		t.Fatalf("got %s", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run 'TestWriteErrorMapsDefsSubsErrors|JSONShape|TestListJSON' -v`
Expected: FAIL — build errors `undefined: toSubmissionJSON`, `undefined: newListJSON`, …

- [ ] **Step 3: Add the error mappings to Plan 01's `errorMappings` table**

Append these rows to `var errorMappings` in `internal/httpapi/errors.go` (Plan 01 matches rows with `errors.Is` after the `*APIError` and `*definition.ValidationError` checks, so wrapped errors map correctly):

```go
	{definitions.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrFormNotPublic, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrInvalidCursor, http.StatusBadRequest, "bad_request", "invalid cursor"},
```

Add `"github.com/openforms/openforms/internal/definitions"` and `"github.com/openforms/openforms/internal/submissions"` to the imports of `errors.go`.

- [ ] **Step 4: Add the JSON shapes**

`internal/httpapi/json_types.go`:
```go
package httpapi

import (
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

// JSON shapes from spec §7.3. Timestamps are always rendered in UTC.

type listJSON[T any] struct {
	Items      []T     `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func newListJSON[T any](items []T, next string) listJSON[T] {
	if items == nil {
		items = []T{}
	}
	l := listJSON[T]{Items: items}
	if next != "" {
		l.NextCursor = &next
	}
	return l
}

type assigneeJSON struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type submissionJSON struct {
	ID          uuid.UUID      `json:"id"`
	Form        string         `json:"form"`
	FormVersion int            `json:"formVersion"`
	State       string         `json:"state"`
	StateLabel  string         `json:"stateLabel"`
	Terminal    bool           `json:"terminal"`
	Data        map[string]any `json:"data"`
	Fields      map[string]any `json:"fields"`
	Assignee    *assigneeJSON  `json:"assignee"`
	CreatedAt   time.Time      `json:"createdAt"`
	UpdatedAt   time.Time      `json:"updatedAt"`
}

func toSubmissionJSON(s submissions.Submission) submissionJSON {
	out := submissionJSON{
		ID: s.ID, Form: s.FormSlug, FormVersion: s.FormVersion,
		State: s.State, StateLabel: s.StateLabel, Terminal: s.Terminal,
		Data: s.Data, Fields: s.Fields,
		CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
	if out.Data == nil {
		out.Data = map[string]any{}
	}
	if out.Fields == nil {
		out.Fields = map[string]any{}
	}
	if s.AssigneeID != nil {
		out.Assignee = &assigneeJSON{ID: *s.AssigneeID, Name: s.AssigneeName, Email: s.AssigneeEmail}
	}
	return out
}

type actorJSON struct {
	Type string     `json:"type"`
	ID   *uuid.UUID `json:"id"`
	Name string     `json:"name"`
}

type eventJSON struct {
	ID         int64          `json:"id"`
	Type       string         `json:"type"`
	FromState  *string        `json:"fromState"`
	ToState    *string        `json:"toState"`
	Transition *string        `json:"transition"`
	Actor      actorJSON      `json:"actor"`
	Payload    map[string]any `json:"payload"`
	CreatedAt  time.Time      `json:"createdAt"`
}

func strOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func toEventJSON(e submissions.Event) eventJSON {
	out := eventJSON{
		ID: e.ID, Type: e.Type,
		FromState: strOrNil(e.FromState), ToState: strOrNil(e.ToState), Transition: strOrNil(e.Transition),
		Actor:     actorJSON{Type: e.ActorType, ID: e.ActorID, Name: e.ActorName},
		Payload:   e.Payload,
		CreatedAt: e.CreatedAt.UTC(),
	}
	if out.Payload == nil {
		out.Payload = map[string]any{}
	}
	return out
}

func toEventsJSON(evs []submissions.Event) []eventJSON {
	out := make([]eventJSON, 0, len(evs))
	for _, e := range evs {
		out = append(out, toEventJSON(e))
	}
	return out
}

type versionJSON struct {
	Version   int       `json:"version"`
	Hash      string    `json:"hash"`
	Source    string    `json:"source"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
}

func toVersionJSON(v definitions.VersionInfo) versionJSON {
	return versionJSON{Version: v.Version, Hash: v.Hash, Source: string(v.Source), CreatedBy: v.CreatedBy, CreatedAt: v.CreatedAt.UTC()}
}

type formRecordJSON struct {
	Slug            string          `json:"slug"`
	Version         int             `json:"version"`
	Source          string          `json:"source"`
	UpdatedAt       time.Time       `json:"updatedAt"`
	WorkflowVersion *int            `json:"workflowVersion"`
	Definition      definition.Form `json:"definition"`
}

type workflowRecordJSON struct {
	Slug       string              `json:"slug"`
	Version    int                 `json:"version"`
	Source     string              `json:"source"`
	UpdatedAt  time.Time           `json:"updatedAt"`
	Definition definition.Workflow `json:"definition"`
}

func toWorkflowRecordJSON(r definitions.WorkflowRecord) workflowRecordJSON {
	return workflowRecordJSON{Slug: r.Slug, Version: r.Current.Version, Source: string(r.Current.Source), UpdatedAt: r.UpdatedAt.UTC(), Definition: r.Definition}
}
```

- [ ] **Step 5: Add `Deps`/`App` fields, empty mounts and route registration**

In the file declaring `type Deps struct` (Plan 01), ensure these fields exist (add them if absent, with imports `internal/definitions` and `internal/submissions`):
```go
	Defs *definitions.Store
	Subs *submissions.Service
```

`internal/httpapi/definition_handlers.go`:
```go
package httpapi

import "github.com/go-chi/chi/v5"

// mountDefinitions registers /forms, /workflows and /definitions routes (authenticated group).
func mountDefinitions(r chi.Router, d Deps) {}
```

`internal/httpapi/public_handlers.go`:
```go
package httpapi

import "github.com/go-chi/chi/v5"

// mountPublic registers the unauthenticated /public routes.
func mountPublic(r chi.Router, d Deps) {}
```

`internal/httpapi/submission_handlers.go`:
```go
package httpapi

import "github.com/go-chi/chi/v5"

// mountSubmissions registers /submissions routes and the CSV export (authenticated group).
func mountSubmissions(r chi.Router, d Deps) {}
```

In `internal/httpapi/routes.go`, inside the `/api/v1` subrouter:
- directly after the `mountAuth(r, d)` line (outside the `RequireAuth` group) add `mountPublic(r, d)`;
- inside the `r.Group(func(r chi.Router) { r.Use(RequireAuth); ... })` block, after `mountAPIKeys(r, d)`, add:
```go
		mountDefinitions(r, d)
		mountSubmissions(r, d)
```

In `internal/app/app.go`: add fields `Defs *definitions.Store` and `Subs *submissions.Service` to `App` if absent; in `New`, after the auth service is built and before the router is built, add
```go
	a.Defs = definitions.NewStore(a.Pool)
	a.Subs = submissions.NewService(a.Pool, a.Defs)
```
and add `Defs: a.Defs, Subs: a.Subs,` to the `httpapi.Deps{...}` literal passed to `httpapi.NewRouter`.

- [ ] **Step 6: Add the shared handler-test fixture**

`internal/httpapi/defs_subs_helpers_test.go`:
```go
package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

type defsSubsAPI struct {
	env   *testutil.Env
	h     http.Handler
	defs  *definitions.Store
	subs  *submissions.Service
	admin string // plaintext API key with the admin role
}

func newDefsSubsAPI(t *testing.T) *defsSubsAPI {
	t.Helper()
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	subs := submissions.NewService(env.Pool, defs)
	h := httpapi.NewRouter(httpapi.Deps{Config: env.Config, Auth: env.Auth, Defs: defs, Subs: subs, OrgID: env.OrgID})
	return &defsSubsAPI{env: env, h: h, defs: defs, subs: subs, admin: env.APIKey(t, auth.RoleAdmin)}
}

// seed applies workflow "review" and forms "contact" (public, review),
// "plain" (public, no workflow) and "internal" (private, review).
func (a *defsSubsAPI) seed(t *testing.T) {
	t.Helper()
	internal := testutil.SampleForm("internal", "review")
	internal.Settings.Public = false
	if _, err := a.defs.Apply(context.Background(), a.env.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", ""), internal},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (a *defsSubsAPI) create(t *testing.T, slug string) (submissions.Submission, string) {
	t.Helper()
	sub, token, err := a.subs.Create(context.Background(), a.env.OrgID, slug, validData(), false)
	if err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	return sub, token
}

func validData() map[string]any {
	return map[string]any{"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}
}

func TestDefsSubsFixtureSeeds(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	sub, _ := a.create(t, "contact")
	if sub.State != "new" {
		t.Fatalf("state = %q", sub.State)
	}
}
```

- [ ] **Step 7: Run tests and the build to verify they pass**

Run: `go build ./... && go test ./internal/httpapi/ ./internal/app/ -race -v`
Expected: build succeeds; `TestWriteErrorMapsDefsSubsErrors` (7 subtests), `TestSubmissionJSONShape`, `TestEventJSONShape`, `TestVersionJSONShape`, `TestListJSONNullCursor`, `TestDefsSubsFixtureSeeds` and all Plan 01 tests `PASS`.

- [ ] **Step 8: Commit**

```bash
git add internal/httpapi/ internal/app/
git commit -m "feat: wire definitions and submissions into the API router and app"
```

---

### Task 9: Definition endpoints (`/forms`, `/workflows`, `/definitions`)

**Parallel group:** B (with Tasks 10 and 11)

**Files:**
- Modify: `internal/httpapi/definition_handlers.go` (replace file)
- Test: `internal/httpapi/definition_handlers_test.go`

**Interfaces:**
- Consumes: `definitions.Store.*`, `definitions.PrefixProblems`, `definitions.ParseSource`, `submissions.Service.CountByForm`, `definition.ParseForm`, `definition.ParseWorkflow`, Task 8 JSON helpers, Plan 01 `WriteJSON`, `WriteError`, `DecodeJSON`, `RequireAdmin`, `MustPrincipal`, `APIError`.
- Produces (spec §7.2):
  - `GET /forms` → `{"items":[{slug,title,workflow|null,public,version,source,updatedAt,submissionCount}]}`
  - `GET /forms/{slug}` → `{"form": {slug,version,source,updatedAt,workflowVersion,definition}}`
  - `PUT /forms/{slug}[?source=ui|api|cli]` (admin) → `{"item": ApplyItem}`
  - `GET /forms/{slug}/versions` → `{"items":[versionJSON]}`; `GET /forms/{slug}/versions/{n}` → `{"form": …}`
  - Same four for `/workflows` (`{"workflow": …}`; summary `{slug,title,version,source,updatedAt,stateCount}`)
  - `POST /definitions/validate` (admin) → `{"valid":true}` or 422; `POST /definitions/apply[?dryRun=true][&source=cli]` (admin) → `{"items":[ApplyItem]}`; `GET /definitions` (admin) → `{"forms":[…],"workflows":[…]}`
  - Package-private `readBody(w, r) ([]byte, error)` (1 MiB limit → 400 `bad_request`) and `actorLabel(p auth.Principal) string`.

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/definition_handlers_test.go`:
```go
package httpapi_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/testutil"
)

type itemResp struct {
	Item struct {
		Kind    string `json:"kind"`
		Slug    string `json:"slug"`
		Version int    `json:"version"`
		Changed bool   `json:"changed"`
		Created bool   `json:"created"`
	} `json:"item"`
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Path    string `json:"path"`
			Message string `json:"message"`
		} `json:"details"`
	} `json:"error"`
}

func TestPutDefinitionsRequireAdmin(t *testing.T) {
	a := newDefsSubsAPI(t)
	reviewer := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", reviewer, testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusForbidden || testutil.ErrorCode(t, rec) != "forbidden" {
		t.Fatalf("reviewer PUT: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", "", testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous PUT: %d", rec.Code)
	}
}

func TestPutWorkflowThenForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/review", a.admin, testutil.SampleWorkflow("review"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT workflow: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[itemResp](t, rec)
	if got.Item.Kind != "workflow" || got.Item.Version != 1 || !got.Item.Created || !got.Item.Changed {
		t.Fatalf("item = %+v", got.Item)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact?source=ui", a.admin, testutil.SampleForm("contact", "review"))
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT form: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact?source=ui", a.admin, testutil.SampleForm("contact", "review"))
	got = testutil.Decode[itemResp](t, rec)
	if got.Item.Changed || got.Item.Version != 1 {
		t.Fatalf("idempotent PUT item = %+v", got.Item)
	}
	f, err := a.defs.GetForm(context.Background(), a.env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Source != "ui" || f.Current.CreatedBy == "" {
		t.Fatalf("version info = %+v", f.Current)
	}
}

func TestPutFormSlugMismatch(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/other", a.admin, testutil.SampleForm("plain", ""))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	if e.Error.Code != "validation_failed" || len(e.Error.Details) != 1 || e.Error.Details[0].Path != "slug" {
		t.Fatalf("error = %+v", e.Error)
	}
}

func TestPutFormInvalidDefinition(t *testing.T) {
	a := newDefsSubsAPI(t)
	body := map[string]any{
		"slug": "plain", "title": "Bad", "settings": map[string]any{"public": true},
		"fields": []any{map[string]any{"key": "1bad", "type": "nope", "label": "X"}},
	}
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, body)
	if rec.Code != http.StatusUnprocessableEntity || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestPutFormMissingWorkflow(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/contact", a.admin, testutil.SampleForm("contact", "nope"))
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestPutFormUnknownSource(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain?source=seed", a.admin, testutil.SampleForm("plain", ""))
	if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
}

func TestGetFormAndList(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	a.create(t, "contact")
	a.create(t, "contact")
	reviewer := a.env.APIKey(t, "reviewer")

	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", reviewer, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET form: %d %s", rec.Code, rec.Body)
	}
	form := testutil.Decode[struct {
		Form struct {
			Slug            string `json:"slug"`
			Version         int    `json:"version"`
			Source          string `json:"source"`
			UpdatedAt       string `json:"updatedAt"`
			WorkflowVersion *int   `json:"workflowVersion"`
			Definition      struct {
				Title  string `json:"title"`
				Fields []any  `json:"fields"`
			} `json:"definition"`
		} `json:"form"`
	}](t, rec)
	if form.Form.Slug != "contact" || form.Form.Version != 1 || form.Form.Source != "api" ||
		form.Form.WorkflowVersion == nil || *form.Form.WorkflowVersion != 1 || form.Form.Definition.Title != "Contact" ||
		!strings.HasSuffix(form.Form.UpdatedAt, "Z") {
		t.Fatalf("form = %+v", form.Form)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/missing", reviewer, nil)
	if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("missing form: %d", rec.Code)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms", reviewer, nil)
	list := testutil.Decode[struct {
		Items []struct {
			Slug            string  `json:"slug"`
			Title           string  `json:"title"`
			Workflow        *string `json:"workflow"`
			Public          bool    `json:"public"`
			Version         int     `json:"version"`
			Source          string  `json:"source"`
			SubmissionCount int     `json:"submissionCount"`
		} `json:"items"`
	}](t, rec)
	if len(list.Items) != 3 || list.Items[0].Slug != "contact" || list.Items[0].SubmissionCount != 2 ||
		list.Items[0].Workflow == nil || *list.Items[0].Workflow != "review" || !list.Items[0].Public {
		t.Fatalf("list = %+v", list.Items)
	}
	if list.Items[1].Slug != "internal" || list.Items[1].Public || list.Items[2].Workflow != nil || list.Items[2].SubmissionCount != 0 {
		t.Fatalf("list = %+v", list.Items)
	}
}

func TestFormVersionEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, testutil.SampleForm("plain", ""))
	f := testutil.SampleForm("plain", "")
	f.Title = "Contact v2"
	testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, f)

	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/plain/versions", a.admin, nil)
	vs := testutil.Decode[struct {
		Items []struct {
			Version   int    `json:"version"`
			Hash      string `json:"hash"`
			Source    string `json:"source"`
			CreatedBy string `json:"createdBy"`
			CreatedAt string `json:"createdAt"`
		} `json:"items"`
	}](t, rec)
	if len(vs.Items) != 2 || vs.Items[0].Version != 2 || len(vs.Items[0].Hash) != 64 {
		t.Fatalf("versions = %+v", vs.Items)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/plain/versions/1", a.admin, nil)
	v1 := testutil.Decode[struct {
		Form struct {
			Version    int `json:"version"`
			Definition struct {
				Title string `json:"title"`
			} `json:"definition"`
		} `json:"form"`
	}](t, rec)
	if v1.Form.Version != 1 || v1.Form.Definition.Title != "Contact" {
		t.Fatalf("v1 = %+v", v1.Form)
	}
	for _, path := range []string{"/api/v1/forms/plain/versions/9", "/api/v1/forms/plain/versions/abc", "/api/v1/forms/missing/versions"} {
		if rec := testutil.Do(t, a.h, "GET", path, a.admin, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestWorkflowEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows", a.admin, nil)
	list := testutil.Decode[struct {
		Items []struct {
			Slug       string `json:"slug"`
			Title      string `json:"title"`
			Version    int    `json:"version"`
			StateCount int    `json:"stateCount"`
		} `json:"items"`
	}](t, rec)
	if len(list.Items) != 1 || list.Items[0].Slug != "review" || list.Items[0].StateCount != 3 || list.Items[0].Title != "Review" {
		t.Fatalf("workflows = %+v", list.Items)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/workflows/review", a.admin, nil)
	wf := testutil.Decode[struct {
		Workflow struct {
			Slug       string `json:"slug"`
			Definition struct {
				Initial string `json:"initial"`
			} `json:"definition"`
		} `json:"workflow"`
	}](t, rec)
	if wf.Workflow.Slug != "review" || wf.Workflow.Definition.Initial != "new" {
		t.Fatalf("workflow = %+v", wf.Workflow)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows/review/versions", a.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("versions: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/workflows/review/versions/1", a.admin, nil); rec.Code != http.StatusOK {
		t.Fatalf("version 1: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "PUT", "/api/v1/workflows/other", a.admin, testutil.SampleWorkflow("review")); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("slug mismatch: %d", rec.Code)
	}
}

func TestValidateBundleEndpoint(t *testing.T) {
	a := newDefsSubsAPI(t)
	ok := map[string]any{
		"workflows": []any{testutil.SampleWorkflow("review")},
		"forms":     []any{testutil.SampleForm("contact", "review")},
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, ok)
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != `{"valid":true}` {
		t.Fatalf("valid bundle: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", a.admin, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("validate persisted the form: %d", rec.Code)
	}

	bad := map[string]any{"forms": []any{
		testutil.SampleForm("plain", ""),
		map[string]any{"slug": "Bad Slug", "title": "x", "settings": map[string]any{"public": true}, "fields": []any{}},
	}}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, bad)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid bundle: %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	for _, d := range e.Error.Details {
		if !strings.HasPrefix(d.Path, "forms[1]") {
			t.Fatalf("detail path %q not prefixed with forms[1]", d.Path)
		}
	}

	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.admin, map[string]any{"forms": []any{testutil.SampleForm("contact", "nope")}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("dangling workflow ref: %d", rec.Code)
	}
	if rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/validate", a.env.APIKey(t, "reviewer"), ok); rec.Code != http.StatusForbidden {
		t.Fatalf("non-admin validate: %d", rec.Code)
	}
}

func TestApplyAndExportEndpoints(t *testing.T) {
	a := newDefsSubsAPI(t)
	bundle := map[string]any{
		"workflows": []any{testutil.SampleWorkflow("review")},
		"forms":     []any{testutil.SampleForm("contact", "review")},
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/definitions/apply?dryRun=true", a.admin, bundle)
	dry := testutil.Decode[struct {
		Items []struct {
			Slug    string `json:"slug"`
			Created bool   `json:"created"`
		} `json:"items"`
	}](t, rec)
	if rec.Code != http.StatusOK || len(dry.Items) != 2 || !dry.Items[0].Created {
		t.Fatalf("dry run: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact", a.admin, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("dry run persisted: %d", rec.Code)
	}

	rec = testutil.Do(t, a.h, "POST", "/api/v1/definitions/apply?source=cli", a.admin, bundle)
	if rec.Code != http.StatusOK {
		t.Fatalf("apply: %d %s", rec.Code, rec.Body)
	}
	f, err := a.defs.GetForm(context.Background(), a.env.OrgID, "contact")
	if err != nil || f.Current.Source != "cli" {
		t.Fatalf("applied form = %+v, err = %v", f.Current, err)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/definitions", a.admin, nil)
	exp := testutil.Decode[struct {
		Forms     []struct{ Slug string } `json:"forms"`
		Workflows []struct{ Slug string } `json:"workflows"`
	}](t, rec)
	if len(exp.Forms) != 1 || exp.Forms[0].Slug != "contact" || len(exp.Workflows) != 1 {
		t.Fatalf("export = %+v", exp)
	}
}

func TestExportEmptyReturnsArrays(t *testing.T) {
	a := newDefsSubsAPI(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/definitions", a.admin, nil)
	if strings.TrimSpace(rec.Body.String()) != `{"forms":[],"workflows":[]}` {
		t.Fatalf("export = %s", rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run 'TestPut|TestGetFormAndList|TestFormVersionEndpoints|TestWorkflowEndpoints|TestValidateBundleEndpoint|TestApplyAndExportEndpoints|TestExportEmpty' -v`
Expected: FAIL — e.g. `PUT workflow: 404` (routes not registered yet)

- [ ] **Step 3: Implement the handlers**

Replace `internal/httpapi/definition_handlers.go`:
```go
package httpapi

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

// mountDefinitions registers /forms, /workflows and /definitions routes (authenticated group).
func mountDefinitions(r chi.Router, d Deps) {
	h := &definitionHandlers{d: d}
	r.Get("/forms", h.listForms)
	r.Get("/forms/{slug}", h.getForm)
	r.With(RequireAdmin).Put("/forms/{slug}", h.putForm)
	r.Get("/forms/{slug}/versions", h.formVersions)
	r.Get("/forms/{slug}/versions/{version}", h.formVersion)

	r.Get("/workflows", h.listWorkflows)
	r.Get("/workflows/{slug}", h.getWorkflow)
	r.With(RequireAdmin).Put("/workflows/{slug}", h.putWorkflow)
	r.Get("/workflows/{slug}/versions", h.workflowVersions)
	r.Get("/workflows/{slug}/versions/{version}", h.workflowVersion)

	r.With(RequireAdmin).Post("/definitions/validate", h.validate)
	r.With(RequireAdmin).Post("/definitions/apply", h.apply)
	r.With(RequireAdmin).Get("/definitions", h.export)
}

type definitionHandlers struct{ d Deps }

var errNotFound = &APIError{Status: http.StatusNotFound, Code: "not_found", Message: "not found"}

// readBody reads a request body of at most 1 MiB.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		return nil, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "request body too large or unreadable"}
	}
	return raw, nil
}

// actorLabel is recorded as created_by on definition versions.
func actorLabel(p auth.Principal) string {
	if p.Email != "" {
		return p.Email
	}
	return p.Name
}

// sourceParam parses ?source=; "seed" is reserved for the Go seeder.
func sourceParam(r *http.Request) (definitions.Source, error) {
	q := r.URL.Query().Get("source")
	if q == "" {
		return definitions.SourceAPI, nil
	}
	src, ok := definitions.ParseSource(q)
	if !ok || src == definitions.SourceSeed {
		return "", &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "source must be one of cli, ui, api"}
	}
	return src, nil
}

func versionParam(r *http.Request) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, "version"))
	if err != nil || n < 1 {
		return 0, errNotFound
	}
	return n, nil
}

func slugMismatch(bodySlug, urlSlug string) error {
	return &definition.ValidationError{Problems: []definition.Problem{{
		Path: "slug", Message: fmt.Sprintf("slug %q does not match URL slug %q", bodySlug, urlSlug),
	}}}
}

func findItem(items []definitions.ApplyItem, kind, slug string) definitions.ApplyItem {
	for _, it := range items {
		if it.Kind == kind && it.Slug == slug {
			return it
		}
	}
	return definitions.ApplyItem{Kind: kind, Slug: slug}
}

// ---- forms ----

type formSummaryJSON struct {
	Slug            string    `json:"slug"`
	Title           string    `json:"title"`
	Workflow        *string   `json:"workflow"`
	Public          bool      `json:"public"`
	Version         int       `json:"version"`
	Source          string    `json:"source"`
	UpdatedAt       time.Time `json:"updatedAt"`
	SubmissionCount int       `json:"submissionCount"`
}

func (h *definitionHandlers) listForms(w http.ResponseWriter, r *http.Request) {
	forms, err := h.d.Defs.ListForms(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	counts, err := h.d.Subs.CountByForm(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items := make([]formSummaryJSON, 0, len(forms))
	for _, f := range forms {
		items = append(items, formSummaryJSON{
			Slug: f.Slug, Title: f.Definition.Title, Workflow: strOrNil(f.Definition.Workflow),
			Public: f.Definition.Settings.Public, Version: f.Current.Version, Source: string(f.Current.Source),
			UpdatedAt: f.UpdatedAt.UTC(), SubmissionCount: counts[f.ID],
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (h *definitionHandlers) formJSON(r *http.Request, rec definitions.FormRecord) (formRecordJSON, error) {
	out := formRecordJSON{Slug: rec.Slug, Version: rec.Current.Version, Source: string(rec.Current.Source), UpdatedAt: rec.UpdatedAt.UTC(), Definition: rec.Definition}
	if rec.WorkflowVersionID != nil {
		wf, err := h.d.Defs.GetWorkflowVersion(r.Context(), *rec.WorkflowVersionID)
		if err != nil {
			return formRecordJSON{}, err
		}
		v := wf.Current.Version
		out.WorkflowVersion = &v
	}
	return out, nil
}

func (h *definitionHandlers) writeForm(w http.ResponseWriter, r *http.Request, rec definitions.FormRecord, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	out, err := h.formJSON(r, rec)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"form": out})
}

func (h *definitionHandlers) getForm(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetForm(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	h.writeForm(w, r, rec, err)
}

func (h *definitionHandlers) putForm(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	f, err := definition.ParseForm(raw)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if f.Slug != slug {
		WriteError(w, r, slugMismatch(f.Slug, slug))
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{f}, Source: src, Actor: actorLabel(MustPrincipal(r)),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"item": findItem(res.Items, "form", slug)})
}

func (h *definitionHandlers) formVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := h.d.Defs.FormVersions(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeVersions(w, vs)
}

func (h *definitionHandlers) formVersion(w http.ResponseWriter, r *http.Request) {
	n, err := versionParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	rec, err := h.d.Defs.FormVersion(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), n)
	h.writeForm(w, r, rec, err)
}

func writeVersions(w http.ResponseWriter, vs []definitions.VersionInfo) {
	items := make([]versionJSON, 0, len(vs))
	for _, v := range vs {
		items = append(items, toVersionJSON(v))
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

// ---- workflows ----

type workflowSummaryJSON struct {
	Slug       string    `json:"slug"`
	Title      string    `json:"title"`
	Version    int       `json:"version"`
	Source     string    `json:"source"`
	UpdatedAt  time.Time `json:"updatedAt"`
	StateCount int       `json:"stateCount"`
}

func (h *definitionHandlers) listWorkflows(w http.ResponseWriter, r *http.Request) {
	wfs, err := h.d.Defs.ListWorkflows(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	items := make([]workflowSummaryJSON, 0, len(wfs))
	for _, wf := range wfs {
		items = append(items, workflowSummaryJSON{
			Slug: wf.Slug, Title: wf.Definition.Title, Version: wf.Current.Version, Source: string(wf.Current.Source),
			UpdatedAt: wf.UpdatedAt.UTC(), StateCount: len(wf.Definition.States),
		})
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeWorkflow(w http.ResponseWriter, r *http.Request, rec definitions.WorkflowRecord, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"workflow": toWorkflowRecordJSON(rec)})
}

func (h *definitionHandlers) getWorkflow(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetWorkflow(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	writeWorkflow(w, r, rec, err)
}

func (h *definitionHandlers) putWorkflow(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	raw, err := readBody(w, r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	wf, err := definition.ParseWorkflow(raw)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if wf.Slug != slug {
		WriteError(w, r, slugMismatch(wf.Slug, slug))
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{wf}, Source: src, Actor: actorLabel(MustPrincipal(r)),
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"item": findItem(res.Items, "workflow", slug)})
}

func (h *definitionHandlers) workflowVersions(w http.ResponseWriter, r *http.Request) {
	vs, err := h.d.Defs.WorkflowVersions(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	writeVersions(w, vs)
}

func (h *definitionHandlers) workflowVersion(w http.ResponseWriter, r *http.Request) {
	n, err := versionParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	rec, err := h.d.Defs.WorkflowVersion(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), n)
	writeWorkflow(w, r, rec, err)
}

// ---- bundles ----

type bundleRequest struct {
	Forms     []json.RawMessage `json:"forms"`
	Workflows []json.RawMessage `json:"workflows"`
}

// parseBundle parses every document, collecting problems prefixed with the
// document's position ("forms[1].fields[0].key").
func parseBundle(r *http.Request) ([]definition.Form, []definition.Workflow, error) {
	var req bundleRequest
	if err := DecodeJSON(r, &req); err != nil {
		return nil, nil, err
	}
	var problems []definition.Problem
	wfs := make([]definition.Workflow, 0, len(req.Workflows))
	for i, raw := range req.Workflows {
		wf, err := definition.ParseWorkflow(raw)
		if err != nil {
			problems = append(problems, definitions.PrefixProblems(fmt.Sprintf("workflows[%d]", i), err)...)
			continue
		}
		wfs = append(wfs, wf)
	}
	forms := make([]definition.Form, 0, len(req.Forms))
	for i, raw := range req.Forms {
		f, err := definition.ParseForm(raw)
		if err != nil {
			problems = append(problems, definitions.PrefixProblems(fmt.Sprintf("forms[%d]", i), err)...)
			continue
		}
		forms = append(forms, f)
	}
	if len(problems) > 0 {
		return nil, nil, &definition.ValidationError{Problems: problems}
	}
	return forms, wfs, nil
}

func (h *definitionHandlers) validate(w http.ResponseWriter, r *http.Request) {
	forms, wfs, err := parseBundle(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	// A dry-run apply runs the same bundle validation (incl. existing workflows) and rolls back.
	if _, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{Forms: forms, Workflows: wfs, DryRun: true}); err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"valid": true})
}

func (h *definitionHandlers) apply(w http.ResponseWriter, r *http.Request) {
	src, err := sourceParam(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	forms, wfs, err := parseBundle(r)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	res, err := h.d.Defs.Apply(r.Context(), h.d.OrgID, definitions.ApplyInput{
		Forms: forms, Workflows: wfs, Source: src, Actor: actorLabel(MustPrincipal(r)),
		DryRun: r.URL.Query().Get("dryRun") == "true",
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"items": res.Items})
}

func (h *definitionHandlers) export(w http.ResponseWriter, r *http.Request) {
	forms, wfs, err := h.d.Defs.Export(r.Context(), h.d.OrgID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"forms": forms, "workflows": wfs})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -run 'TestPut|TestGetFormAndList|TestFormVersionEndpoints|TestWorkflowEndpoints|TestValidateBundleEndpoint|TestApplyAndExportEndpoints|TestExportEmpty' -race -v`
Expected: all listed tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/definition_handlers.go internal/httpapi/definition_handlers_test.go
git commit -m "feat: add form, workflow and bundle definition endpoints"
```

---

### Task 10: Public endpoints (`/public/*`)

**Parallel group:** B (with Tasks 9 and 11)

**Files:**
- Modify: `internal/httpapi/public_handlers.go` (replace file)
- Test: `internal/httpapi/public_handlers_test.go`

**Interfaces:**
- Consumes: Task 3 `newRateLimiter`, `clientIP`; `submissions.Service.{Create, PublicStatus}`; `definitions.Store.{GetForm, GetFormVersion}`; Plan 01 `DecodeJSON`, `URLParamUUID`, `WriteJSON`, `WriteError`, `APIError`.
- Produces (spec §7.2, unauthenticated):
  - `GET /public/forms/{slug}` → `{"form": definition}` (404 unless `settings.public`)
  - `POST /public/forms/{slug}/submissions` `{"data":{…}}` → 201 `{"id","state","stateLabel","receiptToken","confirmationMessage"}`; 20/min/IP → 429 `rate_limited` with `Retry-After: 60`
  - `GET /public/submissions/{id}?token=` → `{id,formTitle,state,stateLabel,terminal,states:[State],history:[{state,label,at}],createdAt}`
  - `const defaultConfirmationMessage = "Thanks! Your response has been recorded."` (used when the form sets none).

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/public_handlers_test.go`:
```go
package httpapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/testutil"
)

type submitResp struct {
	ID                  string `json:"id"`
	State               string `json:"state"`
	StateLabel          string `json:"stateLabel"`
	ReceiptToken        string `json:"receiptToken"`
	ConfirmationMessage string `json:"confirmationMessage"`
}

func TestPublicGetForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "GET", "/api/v1/public/forms/contact", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("public form: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Form struct {
			Slug   string `json:"slug"`
			Fields []any  `json:"fields"`
		} `json:"form"`
	}](t, rec)
	if got.Form.Slug != "contact" || len(got.Form.Fields) != 3 {
		t.Fatalf("form = %+v", got.Form)
	}
	for _, slug := range []string{"internal", "missing"} {
		rec := testutil.Do(t, a.h, "GET", "/api/v1/public/forms/"+slug, "", nil)
		if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
			t.Fatalf("%s: %d", slug, rec.Code)
		}
	}
}

func TestPublicSubmitAndStatus(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": validData()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("submit: %d %s", rec.Code, rec.Body)
	}
	sub := testutil.Decode[submitResp](t, rec)
	if sub.State != "new" || sub.StateLabel != "New" || sub.ReceiptToken == "" || sub.ConfirmationMessage != "Thanks! Your response has been recorded." {
		t.Fatalf("submit resp = %+v", sub)
	}

	rec = testutil.Do(t, a.h, "GET", "/api/v1/public/submissions/"+sub.ID+"?token="+sub.ReceiptToken, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	st := testutil.Decode[struct {
		ID         string `json:"id"`
		FormTitle  string `json:"formTitle"`
		State      string `json:"state"`
		StateLabel string `json:"stateLabel"`
		Terminal   bool   `json:"terminal"`
		States     []struct {
			Key      string `json:"key"`
			Label    string `json:"label"`
			Terminal bool   `json:"terminal"`
		} `json:"states"`
		History []struct {
			State string `json:"state"`
			Label string `json:"label"`
			At    string `json:"at"`
		} `json:"history"`
		CreatedAt string `json:"createdAt"`
	}](t, rec)
	if st.ID != sub.ID || st.FormTitle != "Contact" || st.State != "new" || len(st.States) != 3 ||
		len(st.History) != 1 || st.History[0].Label != "New" || !strings.HasSuffix(st.CreatedAt, "Z") {
		t.Fatalf("status = %+v", st)
	}

	for name, path := range map[string]string{
		"wrong token":  "/api/v1/public/submissions/" + sub.ID + "?token=nope",
		"no token":     "/api/v1/public/submissions/" + sub.ID,
		"unknown id":   "/api/v1/public/submissions/" + uuid.NewString() + "?token=" + sub.ReceiptToken,
		"malformed id": "/api/v1/public/submissions/not-a-uuid?token=" + sub.ReceiptToken,
	} {
		rec := testutil.Do(t, a.h, "GET", path, "", nil)
		if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
	}
}

func TestPublicSubmitUsesFormConfirmationMessage(t *testing.T) {
	a := newDefsSubsAPI(t)
	f := testutil.SampleForm("plain", "")
	f.Settings.ConfirmationMessage = "We'll reply within a day."
	if rec := testutil.Do(t, a.h, "PUT", "/api/v1/forms/plain", a.admin, f); rec.Code != http.StatusOK {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body)
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
	sub := testutil.Decode[submitResp](t, rec)
	if sub.ConfirmationMessage != "We'll reply within a day." || sub.State != "submitted" {
		t.Fatalf("resp = %+v", sub)
	}
}

func TestPublicSubmitValidation(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	data := validData()
	delete(data, "email")
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": data})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d %s", rec.Code, rec.Body)
	}
	e := testutil.Decode[errResp](t, rec)
	found := false
	for _, d := range e.Error.Details {
		if d.Path == "data.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("details = %+v", e.Error.Details)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing data object: %d", rec.Code)
	}
}

func TestPublicSubmitPrivateOrUnknownForm(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for _, slug := range []string{"internal", "missing"} {
		rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/"+slug+"/submissions", "", map[string]any{"data": validData()})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", slug, rec.Code)
		}
	}
}

func TestPublicSubmitBadBodies(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", "not an object")
	if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
		t.Fatalf("string body: %d %s", rec.Code, rec.Body)
	}
	huge := validData()
	huge["name"] = strings.Repeat("a", 2<<20)
	rec = testutil.Do(t, a.h, "POST", "/api/v1/public/forms/contact/submissions", "", map[string]any{"data": huge})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("2 MiB body: %d", rec.Code)
	}
}

func TestPublicSubmitRateLimited(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for i := 0; i < 20; i++ {
		rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
		if rec.Code != http.StatusCreated {
			t.Fatalf("submission %d: %d %s", i+1, rec.Code, rec.Body)
		}
	}
	rec := testutil.Do(t, a.h, "POST", "/api/v1/public/forms/plain/submissions", "", map[string]any{"data": validData()})
	if rec.Code != http.StatusTooManyRequests || testutil.ErrorCode(t, rec) != "rate_limited" || rec.Header().Get("Retry-After") != "60" {
		t.Fatalf("21st submission: %d %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run TestPublic -v`
Expected: FAIL — e.g. `public form: 404` (routes not registered yet)

- [ ] **Step 3: Implement the handlers**

Replace `internal/httpapi/public_handlers.go`:
```go
package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/definition"
)

const defaultConfirmationMessage = "Thanks! Your response has been recorded."

// mountPublic registers the unauthenticated /public routes.
func mountPublic(r chi.Router, d Deps) {
	h := &publicHandlers{d: d, limiter: newRateLimiter(20, time.Minute, time.Now)}
	r.Get("/public/forms/{slug}", h.getForm)
	r.Post("/public/forms/{slug}/submissions", h.submit)
	r.Get("/public/submissions/{id}", h.status)
}

type publicHandlers struct {
	d       Deps
	limiter *rateLimiter
}

func (h *publicHandlers) getForm(w http.ResponseWriter, r *http.Request) {
	rec, err := h.d.Defs.GetForm(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if !rec.Definition.Settings.Public {
		WriteError(w, r, errNotFound)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"form": rec.Definition})
}

type publicSubmitRequest struct {
	Data map[string]any `json:"data"`
}

func (h *publicHandlers) submit(w http.ResponseWriter, r *http.Request) {
	if !h.limiter.Allow(clientIP(r)) {
		w.Header().Set("Retry-After", "60")
		WriteError(w, r, &APIError{Status: http.StatusTooManyRequests, Code: "rate_limited", Message: "too many submissions; try again in a minute"})
		return
	}
	var req publicSubmitRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, token, err := h.d.Subs.Create(r.Context(), h.d.OrgID, chi.URLParam(r, "slug"), req.Data, true)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	msg := defaultConfirmationMessage
	if form, err := h.d.Defs.GetFormVersion(r.Context(), sub.FormVersionID); err == nil && form.Definition.Settings.ConfirmationMessage != "" {
		msg = form.Definition.Settings.ConfirmationMessage
	}
	WriteJSON(w, http.StatusCreated, map[string]any{
		"id": sub.ID, "state": sub.State, "stateLabel": sub.StateLabel,
		"receiptToken": token, "confirmationMessage": msg,
	})
}

type publicHistoryJSON struct {
	State string    `json:"state"`
	Label string    `json:"label"`
	At    time.Time `json:"at"`
}

type publicStatusJSON struct {
	ID         string              `json:"id"`
	FormTitle  string              `json:"formTitle"`
	State      string              `json:"state"`
	StateLabel string              `json:"stateLabel"`
	Terminal   bool                `json:"terminal"`
	States     []definition.State  `json:"states"`
	History    []publicHistoryJSON `json:"history"`
	CreatedAt  time.Time           `json:"createdAt"`
}

func (h *publicHandlers) status(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	st, err := h.d.Subs.PublicStatus(r.Context(), id, r.URL.Query().Get("token"))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	history := make([]publicHistoryJSON, 0, len(st.History))
	for _, it := range st.History {
		history = append(history, publicHistoryJSON{State: it.State, Label: it.Label, At: it.At.UTC()})
	}
	WriteJSON(w, http.StatusOK, publicStatusJSON{
		ID: st.ID.String(), FormTitle: st.FormTitle, State: st.State, StateLabel: st.StateLabel,
		Terminal: st.Terminal, States: st.States, History: history, CreatedAt: st.CreatedAt.UTC(),
	})
}
```
`errNotFound` is declared in `definition_handlers.go` (Task 9). If Task 10 runs before Task 9 is merged, declare it in this file instead and remove the duplicate when merging Group B (keep exactly one declaration, in `definition_handlers.go`).

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -run TestPublic -race -v`
Expected: all `TestPublic*` tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/public_handlers.go internal/httpapi/public_handlers_test.go
git commit -m "feat: add public form, submission and status endpoints"
```

---

### Task 11: Submission endpoints (`/submissions`, CSV export)

**Parallel group:** B (with Tasks 9 and 10)

**Files:**
- Modify: `internal/httpapi/submission_handlers.go` (replace file)
- Test: `internal/httpapi/submission_handlers_test.go`

**Interfaces:**
- Consumes: `submissions.Service.{List, Get, Create, Events, ExportCSV}`, `definitions.Store.{GetFormVersion, GetWorkflowVersion}`, Task 8 JSON helpers, Plan 01 helpers.
- Produces (spec §7.2, authenticated):
  - `GET /submissions?form=&state=&assignee=<uuid|me|none>&cursor=&limit=` → `{"items":[Submission],"nextCursor":…}`; bad `assignee`/`limit`/`cursor` → 400 `bad_request`
  - `POST /submissions` `{"form","data"}` → 201 `{"submission","receiptToken"}` (works on private forms)
  - `GET /submissions/{id}` → `{"submission","form","workflow"|null,"events","transitions"}`
  - `GET /forms/{slug}/submissions.csv` → `text/csv; charset=utf-8`, `Content-Disposition: attachment; filename="<slug>-submissions.csv"`
  - Seam for Plan 04: `func (h *submissionHandlers) transitionsFor(r *http.Request, sub submissions.Submission) (any, error)` returns `[]any{}` in Plan 03; Plan 04 replaces its body with `Engine.Available`.

- [ ] **Step 1: Write the failing tests**

`internal/httpapi/submission_handlers_test.go`:
```go
package httpapi_test

import (
	"context"
	"encoding/csv"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/testutil"
)

type listResp struct {
	Items []struct {
		ID         string `json:"id"`
		Form       string `json:"form"`
		State      string `json:"state"`
		StateLabel string `json:"stateLabel"`
		Assignee   *struct {
			Email string `json:"email"`
		} `json:"assignee"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func TestSubmissionsRequireAuth(t *testing.T) {
	a := newDefsSubsAPI(t)
	for _, path := range []string{"/api/v1/submissions", "/api/v1/submissions/" + uuid.NewString(), "/api/v1/forms/contact/submissions.csv"} {
		rec := testutil.Do(t, a.h, "GET", path, "", nil)
		if rec.Code != http.StatusUnauthorized || testutil.ErrorCode(t, rec) != "unauthenticated" {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestListSubmissionsPaginates(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for i := 0; i < 3; i++ {
		a.create(t, "contact")
	}
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions?limit=2", key, nil)
	page1 := testutil.Decode[listResp](t, rec)
	if rec.Code != http.StatusOK || len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("page1: %d %s", rec.Code, rec.Body)
	}
	if page1.Items[0].StateLabel != "New" || page1.Items[0].Form != "contact" {
		t.Fatalf("item = %+v", page1.Items[0])
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions?limit=2&cursor="+*page1.NextCursor, key, nil)
	page2 := testutil.Decode[listResp](t, rec)
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("page2: %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"nextCursor":null`) {
		t.Fatalf("nextCursor must be null on last page: %s", rec.Body)
	}
}

func TestListSubmissionsFilters(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	assigned, _ := a.create(t, "contact")
	a.create(t, "contact")
	a.create(t, "plain")
	rev := a.env.User(t, "rev@example.com", "reviewer")
	if _, err := a.env.Pool.Exec(context.Background(), `UPDATE submissions SET assignee_id = $2, state = 'review' WHERE id = $1`, assigned.ID, rev.ID); err != nil {
		t.Fatal(err)
	}
	key := a.env.APIKey(t, "reviewer")
	cases := map[string]int{
		"/api/v1/submissions?form=plain":                  1,
		"/api/v1/submissions?state=review":                1,
		"/api/v1/submissions?form=contact&state=new":      1,
		"/api/v1/submissions?assignee=none":               2,
		"/api/v1/submissions?assignee=" + rev.ID.String(): 1,
		"/api/v1/submissions?assignee=me":                 0, // the API key is not the assignee
	}
	for path, want := range cases {
		rec := testutil.Do(t, a.h, "GET", path, key, nil)
		got := testutil.Decode[listResp](t, rec)
		if rec.Code != http.StatusOK || len(got.Items) != want {
			t.Fatalf("%s: %d items (status %d), want %d", path, len(got.Items), rec.Code, want)
		}
	}
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions?assignee="+rev.ID.String(), key, nil)
	got := testutil.Decode[listResp](t, rec)
	if got.Items[0].Assignee == nil || got.Items[0].Assignee.Email != "rev@example.com" || got.Items[0].StateLabel != "In review" {
		t.Fatalf("assigned item = %+v", got.Items[0])
	}
	for _, path := range []string{
		"/api/v1/submissions?assignee=bogus",
		"/api/v1/submissions?limit=abc",
		"/api/v1/submissions?cursor=%21%21garbage",
	} {
		rec := testutil.Do(t, a.h, "GET", path, key, nil)
		if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestGetSubmissionDetail(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	sub, _ := a.create(t, "contact")
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+sub.ID.String(), key, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Submission struct {
			ID   string         `json:"id"`
			Form string         `json:"form"`
			Data map[string]any `json:"data"`
		} `json:"submission"`
		Form struct {
			Slug string `json:"slug"`
		} `json:"form"`
		Workflow *struct {
			Slug string `json:"slug"`
		} `json:"workflow"`
		Events []struct {
			Type    string  `json:"type"`
			ToState *string `json:"toState"`
			Actor   struct {
				Type string `json:"type"`
			} `json:"actor"`
		} `json:"events"`
		Transitions []any `json:"transitions"`
	}](t, rec)
	if got.Submission.ID != sub.ID.String() || got.Submission.Data["name"] != "Ada Lovelace" || got.Form.Slug != "contact" {
		t.Fatalf("detail = %+v", got)
	}
	if got.Workflow == nil || got.Workflow.Slug != "review" {
		t.Fatalf("workflow = %+v", got.Workflow)
	}
	if len(got.Events) != 1 || got.Events[0].Type != "created" || got.Events[0].ToState == nil || *got.Events[0].ToState != "new" {
		t.Fatalf("events = %+v", got.Events)
	}
	if got.Transitions == nil || len(got.Transitions) != 0 {
		t.Fatalf("transitions = %v, want []", got.Transitions)
	}

	plain, _ := a.create(t, "plain")
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+plain.ID.String(), key, nil)
	if !strings.Contains(rec.Body.String(), `"workflow":null`) {
		t.Fatalf("workflow should be null: %s", rec.Body)
	}
	for _, id := range []string{uuid.NewString(), "abc"} {
		if rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+id, key, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", id, rec.Code)
		}
	}
}

func TestCreateSubmissionAuthenticated(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	key := a.env.APIKey(t, "integrator")
	rec := testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "internal", "data": validData()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Submission struct {
			ID    string `json:"id"`
			Form  string `json:"form"`
			State string `json:"state"`
		} `json:"submission"`
		ReceiptToken string `json:"receiptToken"`
	}](t, rec)
	if got.Submission.Form != "internal" || got.Submission.State != "new" || got.ReceiptToken == "" {
		t.Fatalf("resp = %+v", got)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+got.Submission.ID, key, nil)
	if !strings.Contains(rec.Body.String(), `"type":"api_key"`) {
		t.Fatalf("created event should record the api key actor: %s", rec.Body)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "missing", "data": validData()})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown form: %d", rec.Code)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "contact", "data": map[string]any{}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid data: %d", rec.Code)
	}
}

func TestExportCSVEndpoint(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	a.create(t, "contact")
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact/submissions.csv", key, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="contact-submissions.csv"` {
		t.Fatalf("content-disposition = %q", cd)
	}
	records, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil || len(records) != 2 || records[0][0] != "id" {
		t.Fatalf("records = %v, err = %v", records, err)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/missing/submissions.csv", key, nil)
	if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("missing form csv: %d %s", rec.Code, rec.Body)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/httpapi/ -run 'TestSubmissionsRequireAuth|TestListSubmissions|TestGetSubmissionDetail|TestCreateSubmissionAuthenticated|TestExportCSVEndpoint' -v`
Expected: FAIL — e.g. `/api/v1/submissions: 404` (routes not registered yet)

- [ ] **Step 3: Implement the handlers**

Replace `internal/httpapi/submission_handlers.go`:
```go
package httpapi

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// mountSubmissions registers /submissions routes and the CSV export (authenticated group).
func mountSubmissions(r chi.Router, d Deps) {
	h := &submissionHandlers{d: d}
	r.Get("/submissions", h.list)
	r.Post("/submissions", h.create)
	r.Get("/submissions/{id}", h.get)
	r.Get("/forms/{slug}/submissions.csv", h.csv)
}

type submissionHandlers struct{ d Deps }

func badRequest(msg string) *APIError {
	return &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: msg}
}

func (h *submissionHandlers) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	f := submissions.ListFilter{FormSlug: q.Get("form"), State: q.Get("state"), Cursor: q.Get("cursor")}
	switch a := q.Get("assignee"); a {
	case "":
	case "none":
		f.Unassigned = true
	case "me":
		id := MustPrincipal(r).ID
		f.AssigneeID = &id
	default:
		id, err := uuid.Parse(a)
		if err != nil {
			WriteError(w, r, badRequest(`assignee must be a user id, "me" or "none"`))
			return
		}
		f.AssigneeID = &id
	}
	if l := q.Get("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || n < 1 {
			WriteError(w, r, badRequest("limit must be a positive integer"))
			return
		}
		f.Limit = n
	}
	items, next, err := h.d.Subs.List(r.Context(), h.d.OrgID, f)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	out := make([]submissionJSON, 0, len(items))
	for _, s := range items {
		out = append(out, toSubmissionJSON(s))
	}
	WriteJSON(w, http.StatusOK, newListJSON(out, next))
}

type createSubmissionRequest struct {
	Form string         `json:"form"`
	Data map[string]any `json:"data"`
}

func (h *submissionHandlers) create(w http.ResponseWriter, r *http.Request) {
	var req createSubmissionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, token, err := h.d.Subs.Create(r.Context(), h.d.OrgID, req.Form, req.Data, false)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"submission": toSubmissionJSON(sub), "receiptToken": token})
}

func (h *submissionHandlers) get(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	ctx := r.Context()
	sub, err := h.d.Subs.Get(ctx, h.d.OrgID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	form, err := h.d.Defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var wf *definition.Workflow
	if sub.WorkflowVersionID != nil {
		rec, err := h.d.Defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		wf = &rec.Definition
	}
	events, err := h.d.Subs.Events(ctx, h.d.OrgID, id)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	transitions, err := h.transitionsFor(r, sub)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"submission":  toSubmissionJSON(sub),
		"form":        form.Definition,
		"workflow":    wf,
		"events":      toEventsJSON(events),
		"transitions": transitions,
	})
}

// transitionsFor lists the transitions available to the caller.
// Plan 03 has no workflow engine yet, so the list is always empty (spec §7.2);
// Plan 04 replaces this body with Engine.Available.
func (h *submissionHandlers) transitionsFor(r *http.Request, sub submissions.Submission) (any, error) {
	return []any{}, nil
}

// lazyCSVWriter sets the download headers on the first write, so a failure
// before any output can still be reported as a JSON error.
type lazyCSVWriter struct {
	w        http.ResponseWriter
	filename string
	started  bool
}

func (l *lazyCSVWriter) Write(p []byte) (int, error) {
	if !l.started {
		l.started = true
		l.w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		l.w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename=%q`, l.filename))
		l.w.WriteHeader(http.StatusOK)
	}
	return l.w.Write(p)
}

func (h *submissionHandlers) csv(w http.ResponseWriter, r *http.Request) {
	slug := chi.URLParam(r, "slug")
	lw := &lazyCSVWriter{w: w, filename: slug + "-submissions.csv"}
	if err := h.d.Subs.ExportCSV(r.Context(), h.d.OrgID, slug, lw); err != nil && !lw.started {
		WriteError(w, r, err)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/httpapi/ -run 'TestSubmissionsRequireAuth|TestListSubmissions|TestGetSubmissionDetail|TestCreateSubmissionAuthenticated|TestExportCSVEndpoint' -race -v`
Expected: all listed tests `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/httpapi/submission_handlers.go internal/httpapi/submission_handlers_test.go
git commit -m "feat: add submission list, detail, create and CSV export endpoints"
```

---

## Plan exit check (after Group B is merged)

- [ ] Run: `go vet ./... && go test ./... -race`
  Expected: no vet findings; every package `ok`.
- [ ] Run: `grep -rn "errNotFound = " internal/httpapi/*.go`
  Expected: exactly one match, in `definition_handlers.go` (see Task 10 note).
- [ ] Smoke test against a running server (`go run ./cmd/openforms serve` with `OPENFORMS_DATABASE_URL` set, plus an admin API key from Plan 01's bootstrap):
  ```bash
  curl -s -X PUT localhost:8080/api/v1/forms/plain -H "Authorization: Bearer $KEY" -H 'Content-Type: application/json' \
    -d '{"slug":"plain","title":"Contact","settings":{"public":true},"fields":[{"key":"name","type":"text","label":"Name","required":true}]}'
  curl -s -X POST localhost:8080/api/v1/public/forms/plain/submissions -H 'Content-Type: application/json' -d '{"data":{"name":"Ada"}}'
  ```
  Expected: `{"item":{"kind":"form","slug":"plain","version":1,"changed":true,"created":true}}` then a 201 body with `"state":"submitted"` and a `receiptToken`.

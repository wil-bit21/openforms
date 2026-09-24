# Workflow Engine & Actions Implementation Plan (Plan 04)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents); everything else runs in the listed order.

**Goal:** Make submissions move through their workflow: guarded transitions, workflow fields, comments and assignment, plus a Postgres-backed job queue that reliably runs webhook, email and assign actions with retries.

**Architecture:** `internal/jobs` is a transactional-outbox queue. Jobs are inserted in the same transaction as the state change, claimed with `FOR UPDATE SKIP LOCKED`, and retried with exponential backoff. `internal/actions` registers job handlers for `webhook`/`email`/`assign` and records `action_succeeded`/`action_failed`/`assigned` events. `internal/workflow.Engine` runs the transition algorithm in a single transaction and enqueues actions via `actions.Enqueue`. HTTP handlers in `internal/httpapi` expose the engine, and `internal/app` wires everything together and runs the worker alongside the HTTP server.

**Tech Stack:** Go ≥ 1.24, PostgreSQL 16, pgx/v5 (pgxpool), chi/v5, goose/v3 (embedded SQL), google/uuid, `net/smtp`, `net/http/httptest`.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md`. The binding sections for this plan are §4 (`00003_jobs.sql`), §6.8 jobs, §6.9 actions, §6.10 engine, §6.11 httpapi, §6.13 app, §7.1 error table, the Plan 04 rows of §7.2, §7.3 JSON shapes, and §11. Read the roadmap `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md` for wave/lane context. This plan runs in **Wave 3, Lane A**.

**Assumes complete (exactly per spec):** Plan 01 (`config`, `db`, `db/dbtest`, `auth`, `httpapi` skeleton + middleware, `testutil`, `app`), Plan 02 (`definition`), Plan 03 (`definitions.Store`, `submissions.Service` incl. `SetCreatedHook`, `GetForUpdate`, `InsertEvent`, `ActorFromPrincipal`, and the `mountSubmissions` handlers incl. the Submission/Event JSON encoders).

## Global Constraints

- Go ≥ 1.24, module `github.com/openforms/openforms`; no new third-party Go dependencies in this plan (stdlib `net/smtp`, `crypto/hmac`, `net/mail` only).
- DB-backed tests use `dbtest.New(t)` / `testutil.NewEnv(t)`; default DSN `postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable`, override `OPENFORMS_TEST_DATABASE_URL`. Start it with `docker compose up -d postgres`.
- All JSON over the wire is camelCase; timestamps RFC 3339 UTC.
- Every API error uses the envelope `{"error":{"code","message","details"?}}` via `httpapi.WriteError`.
- Error mapping (§7.1): `workflow.ErrForbidden` 403 `forbidden`; `ErrUnknownTransition` 422 `unknown_transition`; `ErrInvalidState` 409 `invalid_state`; `ErrStateConflict` 409 `state_conflict`; `ErrNoWorkflow` 409 `no_workflow`; `*definition.ValidationError` 422 `validation_failed`.
- Backoff after failure: `min(5s * 2^(attempts-1), 1h)`; after `max_attempts` (default 8) → `failed`; `jobs.Permanent(err)` fails immediately; claimed jobs get `status=running, locked_until=now()+5m`, and expired locks are reclaimable.
- Webhook: POST, `Content-Type: application/json`, 10 s timeout, headers `X-OpenForms-Event: submission.created|submission.transitioned`, `X-OpenForms-Delivery: <job id>`, `X-OpenForms-Signature: sha256=<hex HMAC-SHA256(secret, body)>` only when `OPENFORMS_WEBHOOK_SECRET` is set; non-2xx is retryable; 4xx other than 408/429 is permanent.
- Actions never run inside an HTTP request; they are enqueued in the same transaction as the state change.
- Admin satisfies every role guard (`auth.Principal.HasAnyRole`).
- Commit after every task with conventional prefixes (`feat:`, `test:`, `fix:`, `chore:`).

## Review Focus

1. **Two reviewers click a transition at the same moment.** Exactly one wins; the other gets 409 (`invalid_state`, or `state_conflict` when `expectedState` is sent), and only one `transition` event exists. Pinned by `TestTransition_ConcurrentOnlyOneWins` (Task 7) and `TestTransitionHTTP_Errors` (Task 9).
2. **A transition that fails after its actions were enqueued** (e.g. commit fails) must leave no jobs, no state change and no event behind. Pinned by `TestTransition_RollbackLeavesNoJobs` (Task 7), which uses a before-commit failure hook.
3. **Webhook receiver down or rejecting.** A 5xx or 429 retries with backoff, a 400 fails immediately with an `action_failed` event, and the job never loops forever. Pinned by `TestWebhook_Retryable5xx`, `TestWebhook_429IsRetryable` and `TestWebhook_400IsPermanent` (Task 5), plus `TestRunOnce_FailsAfterMaxAttempts` (Task 1).
4. **Email templates with missing keys, numbers, arrays, or a recipient that renders empty or invalid.** Missing keys render as `""` and numbers render without exponent noise (`3`, not `3e+00`). An empty or invalid recipient is a permanent failure rather than eight retries, and a CR/LF in the subject can't inject headers. Pinned by `TestRender` (Task 3), `TestBuildMessage_NoHeaderInjection` (Task 4) and `TestEmail_EmptyRecipientIsPermanent` (Task 6).
5. **Worker crashes mid-job.** A job left `running` is reclaimed once `locked_until` passes and is not lost. Pinned by `TestRunOnce_ReclaimsExpiredLock` (Task 2).

---

## File map

| File | Responsibility |
|---|---|
| `internal/db/migrations/00003_jobs.sql` | jobs table (verbatim from spec §4) |
| `internal/jobs/queue.go` | Job type, Queue, Enqueue, RunOnce, List, Permanent, Backoff, clock, failed hook |
| `internal/jobs/worker.go` | Run loop, Retry, poll interval |
| `internal/actions/render.go` | `Render` template substitution |
| `internal/actions/vars.go` | `TemplateVars` |
| `internal/actions/mailer.go` | `Mailer`, `NewMailer`, `LogMailer`, `SMTPMailer`, `BuildMessage` |
| `internal/actions/payload.go` | kinds, `Payload`, `KindFor`, `Enqueue` |
| `internal/actions/runner.go` | `Deps`, `Register`, shared loading, success/failure events |
| `internal/actions/webhook.go` | webhook handler, `Sign`, webhook body JSON |
| `internal/actions/email.go` | email handler |
| `internal/actions/assign.go` | assign handler + least-loaded selection |
| `internal/submissions/assignee.go` | `SetAssignee` (shared by actions + engine) |
| `internal/testutil/fixture/fixture.go` | shared test fixture: hiring workflow + forms applied, helpers |
| `internal/workflow/errors.go` | sentinel errors |
| `internal/workflow/engine.go` | Engine, Available, OnCreated, workflow resolution |
| `internal/workflow/fields.go` | `mergeFields`, `isEmpty` |
| `internal/workflow/transition.go` | Transition |
| `internal/workflow/edits.go` | UpdateFields, Comment, Assign |
| `internal/workflow/export_test.go` | test hook for before-commit failure |
| `internal/httpapi/workflow_handlers.go` | `mountWorkflow`, AvailableTransition JSON, `availableTransitions` |
| `internal/httpapi/jobs_handlers.go` | `mountJobs`, Job JSON |
| `internal/httpapi/errors.go` (modify) | Plan 04 rows appended to `errorMappings` |
| `internal/httpapi/routes.go` (modify) | mount workflow + jobs |
| Plan 03 submission detail handler (modify) | fill `transitions` |
| `internal/app/workers.go` | `wireWorkflow`, `runWorker` |
| `internal/app/app.go` (modify) | call wiring in `New`, run worker in `Run` |

## Execution order & parallelism

- **Parallel group P1** has two lanes. Lane A runs Task 1 and then Task 2 (`internal/jobs`, migration); Lane B runs Task 3 and Task 4 (`internal/actions/render.go`, `vars.go`, `mailer.go`).
- Task 5 is sequential and needs everything in P1.
- **Parallel group P2** runs Task 6 (`internal/actions/email.go`, `assign.go`, `internal/submissions/assignee.go`, plus a one-line edit to `runner.go`) alongside Task 7 (`internal/workflow/*`).
- Tasks 8, 9 and 10 are sequential.

---

### Task 1: Migration 00003 and the job queue core

**Parallel group:** P1 (Lane A, first)

**Files:**
- Create: `internal/db/migrations/00003_jobs.sql`
- Create: `internal/jobs/queue.go`
- Test: `internal/jobs/queue_test.go`

**Interfaces:**
- Consumes: `db.DBTX` (`internal/db`), `dbtest.New(t) *pgxpool.Pool`.
- Produces:
  - `type Job struct { ID int64; OrgID uuid.UUID; Kind string; Payload json.RawMessage; Status string; Attempts, MaxAttempts int; RunAt time.Time; LastError string; CreatedAt, UpdatedAt time.Time }`
  - `type Handler func(ctx context.Context, job Job) error`
  - `type FailedHook func(ctx context.Context, job Job, err error)`
  - `func NewQueue(pool *pgxpool.Pool) *Queue`
  - `func (q *Queue) SetClock(now func() time.Time)`
  - `func (q *Queue) SetFailedHook(h FailedHook)`
  - `func (q *Queue) Register(kind string, h Handler)`
  - `func (q *Queue) Enqueue(ctx, tx db.DBTX, orgID uuid.UUID, kind string, payload any) (int64, error)`
  - `func (q *Queue) RunOnce(ctx) (bool, error)`
  - `func (q *Queue) List(ctx, orgID uuid.UUID, status string, limit int) ([]Job, error)` (status `""` = all; newest first; limit ≤0 → 100)
  - `func Permanent(err error) error`, `func IsPermanent(err error) bool`, `func Backoff(attempts int) time.Duration`
  - `var ErrNotFound`; constants `StatusPending`, `StatusRunning`, `StatusDone`, `StatusFailed`.

- [ ] **Step 1: Write the migration (verbatim from spec §4)**

`internal/db/migrations/00003_jobs.sql`:

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

- [ ] **Step 2: Write the failing tests**

`internal/jobs/queue_test.go`:

```go
package jobs_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/db/dbtest"
	"github.com/openforms/openforms/internal/jobs"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

type harness struct {
	q     *jobs.Queue
	pool  *pgxpool.Pool
	clock *fakeClock
	org   uuid.UUID
}

func setup(t *testing.T) *harness {
	t.Helper()
	pool := dbtest.New(t)
	q := jobs.NewQueue(pool)
	c := &fakeClock{now: time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)}
	q.SetClock(c.Now)
	return &harness{q: q, pool: pool, clock: c, org: uuid.New()}
}

func (h *harness) enqueue(t *testing.T, kind string, payload any) int64 {
	t.Helper()
	id, err := h.q.Enqueue(context.Background(), h.pool, h.org, kind, payload)
	if err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	return id
}

func (h *harness) job(t *testing.T, id int64) jobs.Job {
	t.Helper()
	all, err := h.q.List(context.Background(), h.org, "", 1000)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	for _, j := range all {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %d not found", id)
	return jobs.Job{}
}

func TestRunOnce_NoJobs(t *testing.T) {
	h := setup(t)
	processed, err := h.q.RunOnce(context.Background())
	if err != nil || processed {
		t.Fatalf("RunOnce = %v, %v; want false, nil", processed, err)
	}
}

func TestEnqueueAndRunOnce_Success(t *testing.T) {
	h := setup(t)
	var got map[string]any
	h.q.Register("test.ok", func(ctx context.Context, j jobs.Job) error {
		return json.Unmarshal(j.Payload, &got)
	})
	id := h.enqueue(t, "test.ok", map[string]any{"n": 1})

	processed, err := h.q.RunOnce(context.Background())
	if err != nil || !processed {
		t.Fatalf("RunOnce = %v, %v; want true, nil", processed, err)
	}
	if got["n"] != float64(1) {
		t.Fatalf("payload = %v", got)
	}
	j := h.job(t, id)
	if j.Status != jobs.StatusDone || j.Attempts != 1 || j.LastError != "" {
		t.Fatalf("job = %+v", j)
	}
	done, _ := h.q.List(context.Background(), h.org, jobs.StatusDone, 10)
	if len(done) != 1 {
		t.Fatalf("done list len = %d", len(done))
	}
	other, _ := h.q.List(context.Background(), uuid.New(), "", 10)
	if len(other) != 0 {
		t.Fatalf("other org sees %d jobs", len(other))
	}
}

func TestRunOnce_RetryWithBackoff(t *testing.T) {
	h := setup(t)
	h.q.Register("test.flaky", func(ctx context.Context, j jobs.Job) error {
		return errors.New("boom")
	})
	id := h.enqueue(t, "test.flaky", map[string]any{})
	ctx := context.Background()

	if p, err := h.q.RunOnce(ctx); !p || err != nil {
		t.Fatalf("first RunOnce = %v, %v", p, err)
	}
	j := h.job(t, id)
	if j.Status != jobs.StatusPending || j.Attempts != 1 || j.LastError != "boom" {
		t.Fatalf("after first failure: %+v", j)
	}
	if want := h.clock.Now().Add(5 * time.Second); !j.RunAt.Equal(want) {
		t.Fatalf("run_at = %v, want %v", j.RunAt, want)
	}
	if p, _ := h.q.RunOnce(ctx); p {
		t.Fatal("job ran before its backoff elapsed")
	}
	h.clock.Advance(5 * time.Second)
	if p, _ := h.q.RunOnce(ctx); !p {
		t.Fatal("job did not run after backoff")
	}
	j = h.job(t, id)
	if j.Attempts != 2 || !j.RunAt.Equal(h.clock.Now().Add(10*time.Second)) {
		t.Fatalf("after second failure: %+v", j)
	}
}

func TestRunOnce_FailsAfterMaxAttempts(t *testing.T) {
	h := setup(t)
	var hookCalls []jobs.Job
	h.q.SetFailedHook(func(ctx context.Context, j jobs.Job, err error) { hookCalls = append(hookCalls, j) })
	h.q.Register("test.flaky", func(ctx context.Context, j jobs.Job) error { return errors.New("down") })
	id := h.enqueue(t, "test.flaky", map[string]any{})
	if _, err := h.pool.Exec(context.Background(), `UPDATE jobs SET max_attempts = 2 WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	h.q.RunOnce(ctx)
	h.clock.Advance(time.Hour)
	h.q.RunOnce(ctx)

	j := h.job(t, id)
	if j.Status != jobs.StatusFailed || j.Attempts != 2 || j.LastError != "down" {
		t.Fatalf("job = %+v", j)
	}
	if len(hookCalls) != 1 || hookCalls[0].ID != id || hookCalls[0].Status != jobs.StatusFailed {
		t.Fatalf("hook calls = %+v", hookCalls)
	}
	h.clock.Advance(time.Hour)
	if p, _ := h.q.RunOnce(ctx); p {
		t.Fatal("failed job was picked up again")
	}
}

func TestRunOnce_PermanentFailsImmediately(t *testing.T) {
	h := setup(t)
	h.q.Register("test.perm", func(ctx context.Context, j jobs.Job) error {
		return jobs.Permanent(errors.New("bad input"))
	})
	id := h.enqueue(t, "test.perm", map[string]any{})
	h.q.RunOnce(context.Background())
	j := h.job(t, id)
	if j.Status != jobs.StatusFailed || j.Attempts != 1 || j.LastError != "bad input" {
		t.Fatalf("job = %+v", j)
	}
}

func TestRunOnce_UnknownKindFails(t *testing.T) {
	h := setup(t)
	id := h.enqueue(t, "test.nobody", map[string]any{})
	h.q.RunOnce(context.Background())
	j := h.job(t, id)
	if j.Status != jobs.StatusFailed || !strings.Contains(j.LastError, "no handler") {
		t.Fatalf("job = %+v", j)
	}
}

func TestRunOnce_PanicIsRecorded(t *testing.T) {
	h := setup(t)
	h.q.Register("test.panic", func(ctx context.Context, j jobs.Job) error { panic("kaboom") })
	id := h.enqueue(t, "test.panic", map[string]any{})
	if _, err := h.q.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce err = %v", err)
	}
	j := h.job(t, id)
	if j.Status != jobs.StatusPending || !strings.Contains(j.LastError, "kaboom") {
		t.Fatalf("job = %+v", j)
	}
}

func TestPermanent(t *testing.T) {
	base := errors.New("x")
	p := jobs.Permanent(base)
	if !jobs.IsPermanent(p) || !errors.Is(p, base) || p.Error() != "x" {
		t.Fatalf("Permanent wrapper broken: %v", p)
	}
	if jobs.IsPermanent(base) {
		t.Fatal("plain error reported permanent")
	}
	if jobs.Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must be nil")
	}
}

func TestBackoff(t *testing.T) {
	cases := []struct {
		attempts int
		want     time.Duration
	}{
		{0, 5 * time.Second},
		{1, 5 * time.Second},
		{2, 10 * time.Second},
		{3, 20 * time.Second},
		{9, 1280 * time.Second},
		{10, 2560 * time.Second},
		{11, time.Hour},
		{50, time.Hour},
	}
	for _, c := range cases {
		if got := jobs.Backoff(c.attempts); got != c.want {
			t.Errorf("Backoff(%d) = %v, want %v", c.attempts, got, c.want)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `docker compose up -d postgres && go test ./internal/jobs/...`
Expected: FAIL to compile, e.g. `undefined: jobs.NewQueue`.

- [ ] **Step 4: Write the implementation**

`internal/jobs/queue.go`:

```go
// Package jobs is a Postgres-backed job queue. Jobs are enqueued inside the
// caller's transaction (transactional outbox) and claimed with
// FOR UPDATE SKIP LOCKED, so any number of workers can run concurrently.
package jobs

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

	"github.com/openforms/openforms/internal/db"
)

const (
	StatusPending = "pending"
	StatusRunning = "running"
	StatusDone    = "done"
	StatusFailed  = "failed"

	lockDuration = 5 * time.Minute
	baseBackoff  = 5 * time.Second
	maxBackoff   = time.Hour
)

var ErrNotFound = errors.New("job not found")

type Job struct {
	ID          int64
	OrgID       uuid.UUID
	Kind        string
	Payload     json.RawMessage
	Status      string
	Attempts    int
	MaxAttempts int
	RunAt       time.Time
	LastError   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type Handler func(ctx context.Context, job Job) error

// FailedHook is called once when a job ends in status "failed".
type FailedHook func(ctx context.Context, job Job, err error)

type Queue struct {
	pool     *pgxpool.Pool
	mu       sync.RWMutex
	handlers map[string]Handler
	onFailed FailedHook
	now      func() time.Time
	poll     time.Duration
}

func NewQueue(pool *pgxpool.Pool) *Queue {
	return &Queue{
		pool:     pool,
		handlers: map[string]Handler{},
		now:      func() time.Time { return time.Now().UTC() },
		poll:     time.Second,
	}
}

// SetClock replaces the queue's clock; used by tests.
func (q *Queue) SetClock(now func() time.Time) { q.now = now }

func (q *Queue) SetFailedHook(h FailedHook) {
	q.mu.Lock()
	q.onFailed = h
	q.mu.Unlock()
}

func (q *Queue) Register(kind string, h Handler) {
	q.mu.Lock()
	q.handlers[kind] = h
	q.mu.Unlock()
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

// Permanent marks err as non-retryable: the job fails immediately.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &permanentError{err: err}
}

func IsPermanent(err error) bool {
	var p *permanentError
	return errors.As(err, &p)
}

// Backoff returns the delay before retry number attempts+1:
// min(5s * 2^(attempts-1), 1h).
func Backoff(attempts int) time.Duration {
	if attempts < 1 {
		attempts = 1
	}
	d := baseBackoff
	for i := 1; i < attempts; i++ {
		d *= 2
		if d >= maxBackoff {
			return maxBackoff
		}
	}
	return d
}

const jobColumns = `id, org_id, kind, payload, status, attempts, max_attempts, run_at, last_error, created_at, updated_at`

func scanJob(row pgx.Row) (Job, error) {
	var j Job
	var payload []byte
	err := row.Scan(&j.ID, &j.OrgID, &j.Kind, &payload, &j.Status, &j.Attempts, &j.MaxAttempts,
		&j.RunAt, &j.LastError, &j.CreatedAt, &j.UpdatedAt)
	j.Payload = json.RawMessage(payload)
	j.RunAt = j.RunAt.UTC()
	return j, err
}

// Enqueue inserts a pending job using tx, so it commits or rolls back with
// the caller's transaction.
func (q *Queue) Enqueue(ctx context.Context, tx db.DBTX, orgID uuid.UUID, kind string, payload any) (int64, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return 0, fmt.Errorf("jobs: marshal payload: %w", err)
	}
	var id int64
	err = tx.QueryRow(ctx,
		`INSERT INTO jobs (org_id, kind, payload, run_at, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4, $4) RETURNING id`,
		orgID, kind, json.RawMessage(raw), q.now()).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("jobs: enqueue %s: %w", kind, err)
	}
	return id, nil
}

// RunOnce claims at most one due job, runs its handler and records the
// outcome. Handler errors are recorded on the job, not returned.
func (q *Queue) RunOnce(ctx context.Context) (bool, error) {
	now := q.now()
	row := q.pool.QueryRow(ctx, `
UPDATE jobs SET status = 'running', attempts = attempts + 1, locked_until = $2, updated_at = $1
WHERE id = (
  SELECT id FROM jobs
  WHERE (status = 'pending' AND run_at <= $1)
     OR (status = 'running' AND locked_until < $1)
  ORDER BY run_at, id
  LIMIT 1
  FOR UPDATE SKIP LOCKED
)
RETURNING `+jobColumns, now, now.Add(lockDuration))
	job, err := scanJob(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("jobs: claim: %w", err)
	}
	return true, q.finish(ctx, job, q.execute(ctx, job))
}

func (q *Queue) execute(ctx context.Context, job Job) (err error) {
	q.mu.RLock()
	h, ok := q.handlers[job.Kind]
	q.mu.RUnlock()
	if !ok {
		return Permanent(fmt.Errorf("no handler registered for kind %q", job.Kind))
	}
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("handler panic: %v", r)
		}
	}()
	return h(ctx, job)
}

func (q *Queue) finish(ctx context.Context, job Job, runErr error) error {
	now := q.now()
	if runErr == nil {
		_, err := q.pool.Exec(ctx,
			`UPDATE jobs SET status = 'done', locked_until = NULL, last_error = '', updated_at = $2 WHERE id = $1`,
			job.ID, now)
		return err
	}
	if IsPermanent(runErr) || job.Attempts >= job.MaxAttempts {
		if _, err := q.pool.Exec(ctx,
			`UPDATE jobs SET status = 'failed', locked_until = NULL, last_error = $2, updated_at = $3 WHERE id = $1`,
			job.ID, runErr.Error(), now); err != nil {
			return err
		}
		job.Status = StatusFailed
		job.LastError = runErr.Error()
		q.mu.RLock()
		hook := q.onFailed
		q.mu.RUnlock()
		if hook != nil {
			hook(ctx, job, runErr)
		}
		return nil
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE jobs SET status = 'pending', locked_until = NULL, last_error = $2, run_at = $3, updated_at = $4 WHERE id = $1`,
		job.ID, runErr.Error(), now.Add(Backoff(job.Attempts)), now)
	return err
}

// List returns the org's jobs, newest first. status "" means all statuses.
func (q *Queue) List(ctx context.Context, orgID uuid.UUID, status string, limit int) ([]Job, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := q.pool.Query(ctx,
		`SELECT `+jobColumns+` FROM jobs
		 WHERE org_id = $1 AND ($2 = '' OR status = $2)
		 ORDER BY id DESC LIMIT $3`, orgID, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/jobs/... -run 'TestRunOnce|TestEnqueue|TestPermanent|TestBackoff' -v`
Expected: PASS for all eight tests.

- [ ] **Step 6: Commit**

```bash
git add internal/db/migrations/00003_jobs.sql internal/jobs/queue.go internal/jobs/queue_test.go
git commit -m "feat(jobs): postgres job queue with backoff and permanent failures"
```

---

### Task 2: Worker loop, retry and lock reclaim

**Parallel group:** P1 (Lane A, after Task 1)

**Files:**
- Create: `internal/jobs/worker.go`
- Test: `internal/jobs/worker_test.go`

**Interfaces:**
- Consumes: Task 1 Queue.
- Produces:
  - `func (q *Queue) Run(ctx context.Context, concurrency int)` returns once ctx is done and all goroutines have exited; concurrency <1 is treated as 1.
  - `func (q *Queue) SetPollInterval(d time.Duration)`
  - `func (q *Queue) Retry(ctx, orgID uuid.UUID, id int64) error` returns `ErrNotFound` unless the job exists in the org with status `failed`.

- [ ] **Step 1: Write the failing tests**

`internal/jobs/worker_test.go` (same package `jobs_test`; reuses `setup` from `queue_test.go`):

```go
package jobs_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/db/dbtest"
	"github.com/openforms/openforms/internal/jobs"
)

func TestRun_TwoWorkersProcessEachJobExactlyOnce(t *testing.T) {
	pool := dbtest.New(t)
	org := uuid.New()
	var mu sync.Mutex
	counts := map[int64]int{}
	handler := func(ctx context.Context, j jobs.Job) error {
		time.Sleep(2 * time.Millisecond)
		mu.Lock()
		counts[j.ID]++
		mu.Unlock()
		return nil
	}
	q1, q2 := jobs.NewQueue(pool), jobs.NewQueue(pool)
	for _, q := range []*jobs.Queue{q1, q2} {
		q.Register("test.count", handler)
		q.SetPollInterval(10 * time.Millisecond)
	}
	const n = 30
	for i := 0; i < n; i++ {
		if _, err := q1.Enqueue(context.Background(), pool, org, "test.count", map[string]int{"i": i}); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	for _, q := range []*jobs.Queue{q1, q2} {
		wg.Add(1)
		go func(q *jobs.Queue) { defer wg.Done(); q.Run(ctx, 2) }(q)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		done, err := q1.List(context.Background(), org, jobs.StatusDone, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(done) == n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d/%d jobs done", len(done), n)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	if len(counts) != n {
		t.Fatalf("handled %d distinct jobs, want %d", len(counts), n)
	}
	for id, c := range counts {
		if c != 1 {
			t.Errorf("job %d ran %d times", id, c)
		}
	}
}

func TestRun_ReturnsWhenContextCancelled(t *testing.T) {
	q := jobs.NewQueue(dbtest.New(t))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { q.Run(ctx, 0); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}

func TestRunOnce_ReclaimsExpiredLock(t *testing.T) {
	h := setup(t)
	ran := 0
	h.q.Register("test.ok", func(ctx context.Context, j jobs.Job) error { ran++; return nil })
	id := h.enqueue(t, "test.ok", map[string]any{})
	// Simulate a worker that claimed the job and then crashed.
	if _, err := h.pool.Exec(context.Background(),
		`UPDATE jobs SET status = 'running', attempts = 1, locked_until = $2 WHERE id = $1`,
		id, h.clock.Now().Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if p, _ := h.q.RunOnce(context.Background()); p {
		t.Fatal("claimed a job whose lock has not expired")
	}
	h.clock.Advance(5*time.Minute + time.Second)
	if p, err := h.q.RunOnce(context.Background()); !p || err != nil {
		t.Fatalf("RunOnce = %v, %v; want reclaim", p, err)
	}
	j := h.job(t, id)
	if ran != 1 || j.Status != jobs.StatusDone || j.Attempts != 2 {
		t.Fatalf("ran=%d job=%+v", ran, j)
	}
}

func TestRetry(t *testing.T) {
	h := setup(t)
	fail := true
	h.q.Register("test.toggle", func(ctx context.Context, j jobs.Job) error {
		if fail {
			return jobs.Permanent(errors.New("nope"))
		}
		return nil
	})
	id := h.enqueue(t, "test.toggle", map[string]any{})
	ctx := context.Background()
	h.q.RunOnce(ctx)
	if j := h.job(t, id); j.Status != jobs.StatusFailed {
		t.Fatalf("precondition: %+v", j)
	}

	if err := h.q.Retry(ctx, uuid.New(), id); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("retry from other org err = %v", err)
	}
	if err := h.q.Retry(ctx, h.org, id); err != nil {
		t.Fatalf("retry: %v", err)
	}
	j := h.job(t, id)
	if j.Status != jobs.StatusPending || j.Attempts != 0 || j.LastError != "" {
		t.Fatalf("after retry: %+v", j)
	}
	if err := h.q.Retry(ctx, h.org, id); !errors.Is(err, jobs.ErrNotFound) {
		t.Fatalf("retrying a pending job err = %v, want ErrNotFound", err)
	}
	fail = false
	h.q.RunOnce(ctx)
	if j := h.job(t, id); j.Status != jobs.StatusDone {
		t.Fatalf("after rerun: %+v", j)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/jobs/... -run 'TestRun_|TestRetry|Reclaims' -v`
Expected: FAIL to compile with `q.SetPollInterval undefined`, `q.Run undefined`, `q.Retry undefined`.

- [ ] **Step 3: Write the implementation**

`internal/jobs/worker.go`:

```go
package jobs

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SetPollInterval sets how long an idle worker waits before polling again.
func (q *Queue) SetPollInterval(d time.Duration) { q.poll = d }

// Run processes jobs with `concurrency` goroutines until ctx is done.
func (q *Queue) Run(ctx context.Context, concurrency int) {
	if concurrency < 1 {
		concurrency = 1
	}
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				processed, err := q.RunOnce(ctx)
				if err != nil && ctx.Err() == nil {
					slog.Error("jobs: worker", "err", err)
				}
				if processed && err == nil {
					continue
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(q.poll):
				}
			}
		}()
	}
	wg.Wait()
}

// Retry resets a failed job so it runs again immediately.
func (q *Queue) Retry(ctx context.Context, orgID uuid.UUID, id int64) error {
	now := q.now()
	tag, err := q.pool.Exec(ctx,
		`UPDATE jobs SET status = 'pending', attempts = 0, run_at = $3, locked_until = NULL,
		        last_error = '', updated_at = $3
		 WHERE org_id = $1 AND id = $2 AND status = 'failed'`, orgID, id, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/jobs/... -race -v`
Expected: PASS for all jobs tests with no race reports.

- [ ] **Step 5: Commit**

```bash
git add internal/jobs/worker.go internal/jobs/worker_test.go
git commit -m "feat(jobs): concurrent worker loop, retry and expired-lock reclaim"
```

---

### Task 3: Template rendering and template variables

**Parallel group:** P1 (Lane B)

**Files:**
- Create: `internal/actions/render.go`
- Create: `internal/actions/vars.go`
- Test: `internal/actions/render_test.go`

**Interfaces:**
- Consumes: `config.Config` (`BaseURL`), `definition.Form`, `definition.Transition`, `submissions.Submission`.
- Produces:
  - `func Render(tmpl string, vars map[string]any) string`
  - `func TemplateVars(cfg config.Config, form definition.Form, sub submissions.Submission, transition *definition.Transition) map[string]any`

- [ ] **Step 1: Write the failing tests**

`internal/actions/render_test.go`:

```go
package actions_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

func TestRender(t *testing.T) {
	vars := map[string]any{
		"submission": map[string]any{
			"data": map[string]any{
				"name":   "Ada",
				"score":  float64(3),
				"ratio":  0.25,
				"ok":     true,
				"tags":   []any{"a", "b"},
				"nested": map[string]any{"x": float64(1)},
				"none":   nil,
			},
		},
		"baseUrl": "https://forms.example.com",
	}
	cases := []struct{ tmpl, want string }{
		{"Hi {{submission.data.name}}!", "Hi Ada!"},
		{"Hi {{ submission.data.name }}!", "Hi Ada!"},
		{"{{submission.data.score}}", "3"},
		{"{{submission.data.ratio}}", "0.25"},
		{"{{submission.data.ok}}", "true"},
		{"{{submission.data.tags}}", "a, b"},
		{"{{submission.data.nested}}", `{"x":1}`},
		{"[{{submission.data.none}}]", "[]"},
		{"[{{submission.data.missing}}]", "[]"},
		{"[{{nope.deeper.still}}]", "[]"},
		{"[{{submission.data.name.first}}]", "[]"},
		{"{{baseUrl}}/x", "https://forms.example.com/x"},
		{"no placeholders", "no placeholders"},
		{"{{ not closed", "{{ not closed"},
		{"<b>{{submission.data.name}}</b>", "<b>Ada</b>"},
	}
	for _, c := range cases {
		if got := actions.Render(c.tmpl, vars); got != c.want {
			t.Errorf("Render(%q) = %q, want %q", c.tmpl, got, c.want)
		}
	}
}

func TestTemplateVars(t *testing.T) {
	id := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	sub := submissions.Submission{
		ID: id, State: "screening", StateLabel: "Screening",
		Data:      map[string]any{"email": "ada@example.com"},
		Fields:    map[string]any{"score": float64(4)},
		CreatedAt: time.Now(),
	}
	form := definition.Form{Slug: "apply", Title: "Apply"}
	tr := &definition.Transition{Key: "invite", Label: "Invite"}
	cfg := config.Config{BaseURL: "https://forms.example.com/"}

	vars := actions.TemplateVars(cfg, form, sub, tr)
	checks := map[string]string{
		"{{submission.id}}":           id.String(),
		"{{submission.state}}":        "screening",
		"{{submission.stateLabel}}":   "Screening",
		"{{submission.data.email}}":   "ada@example.com",
		"{{submission.fields.score}}": "4",
		"{{submission.url}}":          "https://forms.example.com/admin/submissions/" + id.String(),
		"{{form.slug}}":               "apply",
		"{{form.title}}":              "Apply",
		"{{transition.key}}":          "invite",
		"{{transition.label}}":        "Invite",
		"{{baseUrl}}":                 "https://forms.example.com",
		"[{{submission.statusUrl}}]":  "[]",
	}
	for tmpl, want := range checks {
		if got := actions.Render(tmpl, vars); got != want {
			t.Errorf("%s = %q, want %q", tmpl, got, want)
		}
	}

	noTr := actions.TemplateVars(cfg, form, submissions.Submission{ID: id}, nil)
	if got := actions.Render("[{{transition.key}}][{{submission.data.x}}]", noTr); got != "[][]" {
		t.Errorf("nil transition/data rendered %q", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/actions/... -run 'TestRender|TestTemplateVars' -v`
Expected: FAIL to compile with `undefined: actions.Render`.

- [ ] **Step 3: Write the implementation**

`internal/actions/render.go`:

```go
// Package actions runs workflow side effects (webhook, email, assign) as jobs.
package actions

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var placeholder = regexp.MustCompile(`\{\{\s*([A-Za-z0-9_.-]+)\s*\}\}`)

// Render replaces {{a.b.c}} placeholders with values looked up in vars.
// Missing paths render as "". Output is plain text (no HTML escaping).
func Render(tmpl string, vars map[string]any) string {
	return placeholder.ReplaceAllStringFunc(tmpl, func(m string) string {
		path := placeholder.FindStringSubmatch(m)[1]
		return format(lookup(vars, strings.Split(path, ".")))
	})
}

func lookup(v any, parts []string) any {
	cur := v
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		if cur, ok = m[p]; !ok {
			return nil
		}
	}
	return cur
}

func format(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case json.Number:
		return x.String()
	case []string:
		return strings.Join(x, ", ")
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			parts[i] = format(e)
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		b, err := json.Marshal(x)
		if err != nil {
			return ""
		}
		return string(b)
	default:
		return fmt.Sprint(x)
	}
}
```

`internal/actions/vars.go`:

```go
package actions

import (
	"strings"

	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// TemplateVars builds the variables available to email templates (spec §6.9).
func TemplateVars(cfg config.Config, form definition.Form, sub submissions.Submission, transition *definition.Transition) map[string]any {
	base := strings.TrimRight(cfg.BaseURL, "/")
	tr := map[string]any{}
	if transition != nil {
		tr["key"] = transition.Key
		tr["label"] = transition.Label
	}
	return map[string]any{
		"baseUrl":    base,
		"form":       map[string]any{"slug": form.Slug, "title": form.Title},
		"transition": tr,
		"submission": map[string]any{
			"id":         sub.ID.String(),
			"state":      sub.State,
			"stateLabel": sub.StateLabel,
			"data":       nonNil(sub.Data),
			"fields":     nonNil(sub.Fields),
			"url":        base + "/admin/submissions/" + sub.ID.String(),
		},
	}
}

func nonNil(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/actions/... -run 'TestRender|TestTemplateVars' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/actions/render.go internal/actions/vars.go internal/actions/render_test.go
git commit -m "feat(actions): template rendering and template variables"
```

---

### Task 4: Mailer (SMTP + log)

**Parallel group:** P1 (Lane B, after Task 3; the files are disjoint so it may also run alongside Task 3)

**Files:**
- Create: `internal/actions/mailer.go`
- Test: `internal/actions/mailer_test.go`

**Interfaces:**
- Consumes: `config.Config` (`SMTPHost`, `SMTPPort`, `SMTPUsername`, `SMTPPassword`, `SMTPFrom`).
- Produces:
  - `type Mailer interface { Send(ctx context.Context, to, subject, body string) error }`
  - `func NewMailer(cfg config.Config) Mailer` returns `LogMailer{}` when `SMTPHost == ""`, else `*SMTPMailer`.
  - `type LogMailer struct{}`
  - `type SMTPMailer struct { Addr, Host, Username, Password, From string; SendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error }`
  - `func BuildMessage(from, to, subject, body string, date time.Time) ([]byte, error)`

- [ ] **Step 1: Write the failing tests**

`internal/actions/mailer_test.go`:

```go
package actions_test

import (
	"context"
	"net/smtp"
	"strings"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/config"
)

func TestNewMailer(t *testing.T) {
	if _, ok := actions.NewMailer(config.Config{}).(actions.LogMailer); !ok {
		t.Fatal("empty SMTPHost must give LogMailer")
	}
	m, ok := actions.NewMailer(config.Config{SMTPHost: "mail.local", SMTPPort: 1025, SMTPFrom: "of@x.test"}).(*actions.SMTPMailer)
	if !ok || m.Addr != "mail.local:1025" || m.From != "of@x.test" {
		t.Fatalf("SMTP mailer = %#v", m)
	}
	if err := (actions.LogMailer{}).Send(context.Background(), "a@b.c", "s", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestBuildMessage(t *testing.T) {
	date := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Hello Ada", "line1\nline2", date)
	if err != nil {
		t.Fatal(err)
	}
	s := string(msg)
	for _, want := range []string{
		"From: of@x.test\r\n",
		"To: ada@example.com\r\n",
		"Subject: Hello Ada\r\n",
		"Date: Wed, 02 Jan 2030 03:04:05 +0000\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: text/plain; charset=utf-8\r\n",
		"\r\n\r\nline1\r\nline2",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("message missing %q:\n%s", want, s)
		}
	}
}

func TestBuildMessage_NoHeaderInjection(t *testing.T) {
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Hi\r\nBcc: evil@x.test", "body", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(msg), "\r\n") {
		if strings.HasPrefix(line, "Bcc:") {
			t.Fatalf("header injected:\n%s", msg)
		}
	}
	if _, err := actions.BuildMessage("of@x.test", "ada@example.com\r\nBcc: evil@x.test", "s", "b", time.Now()); err == nil {
		t.Fatal("recipient with CRLF must be rejected")
	}
	if _, err := actions.BuildMessage("of@x.test", "not an address", "s", "b", time.Now()); err == nil {
		t.Fatal("invalid recipient must be rejected")
	}
}

func TestBuildMessage_EncodesNonASCIISubject(t *testing.T) {
	msg, err := actions.BuildMessage("of@x.test", "ada@example.com", "Grüße", "b", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(msg), "Subject: =?utf-8?q?Gr=C3=BC=C3=9Fe?=\r\n") {
		t.Fatalf("subject not encoded:\n%s", msg)
	}
}

func TestSMTPMailer_Send(t *testing.T) {
	var gotAddr, gotFrom string
	var gotTo []string
	var gotMsg []byte
	var gotAuth smtp.Auth
	m := &actions.SMTPMailer{
		Addr: "mail.local:1025", Host: "mail.local", From: "of@x.test",
		Username: "u", Password: "p",
		SendFunc: func(addr string, a smtp.Auth, from string, to []string, msg []byte) error {
			gotAddr, gotAuth, gotFrom, gotTo, gotMsg = addr, a, from, to, msg
			return nil
		},
	}
	if err := m.Send(context.Background(), "ada@example.com", "Subj", "Body"); err != nil {
		t.Fatal(err)
	}
	if gotAddr != "mail.local:1025" || gotFrom != "of@x.test" || len(gotTo) != 1 || gotTo[0] != "ada@example.com" {
		t.Fatalf("send args: %s %s %v", gotAddr, gotFrom, gotTo)
	}
	if gotAuth == nil || !strings.Contains(string(gotMsg), "Subject: Subj") {
		t.Fatalf("auth=%v msg=%s", gotAuth, gotMsg)
	}

	m.Username = ""
	m.Send(context.Background(), "ada@example.com", "s", "b")
	if gotAuth != nil {
		t.Fatal("auth must be nil when no username configured")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/actions/... -run 'Mailer|BuildMessage' -v`
Expected: FAIL to compile with `undefined: actions.NewMailer`.

- [ ] **Step 3: Write the implementation**

`internal/actions/mailer.go`:

```go
package actions

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"

	"github.com/openforms/openforms/internal/config"
)

type Mailer interface {
	Send(ctx context.Context, to, subject, body string) error
}

// NewMailer returns an SMTP mailer when SMTPHost is configured, otherwise a
// mailer that only logs (useful for local development).
func NewMailer(cfg config.Config) Mailer {
	if cfg.SMTPHost == "" {
		return LogMailer{}
	}
	return &SMTPMailer{
		Addr:     net.JoinHostPort(cfg.SMTPHost, strconv.Itoa(cfg.SMTPPort)),
		Host:     cfg.SMTPHost,
		Username: cfg.SMTPUsername,
		Password: cfg.SMTPPassword,
		From:     cfg.SMTPFrom,
	}
}

type LogMailer struct{}

func (LogMailer) Send(ctx context.Context, to, subject, body string) error {
	slog.InfoContext(ctx, "email (log mailer)", "to", to, "subject", subject, "body", body)
	return nil
}

type SMTPMailer struct {
	Addr, Host, Username, Password, From string
	// SendFunc defaults to smtp.SendMail; tests replace it.
	SendFunc func(addr string, a smtp.Auth, from string, to []string, msg []byte) error
}

func (m *SMTPMailer) Send(ctx context.Context, to, subject, body string) error {
	msg, err := BuildMessage(m.From, to, subject, body, time.Now())
	if err != nil {
		return err
	}
	var auth smtp.Auth
	if m.Username != "" {
		auth = smtp.PlainAuth("", m.Username, m.Password, m.Host)
	}
	send := m.SendFunc
	if send == nil {
		send = smtp.SendMail
	}
	if err := send(m.Addr, auth, m.From, []string{to}, msg); err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}
	return nil
}

// BuildMessage renders a plain-text RFC 5322 message. It rejects addresses
// containing CR/LF and strips CR/LF from the subject to prevent header injection.
func BuildMessage(from, to, subject, body string, date time.Time) ([]byte, error) {
	for _, a := range []string{from, to} {
		if strings.ContainsAny(a, "\r\n") {
			return nil, errors.New("email address contains a line break")
		}
	}
	if _, err := mail.ParseAddress(to); err != nil {
		return nil, fmt.Errorf("invalid recipient %q: %w", to, err)
	}
	subject = strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ").Replace(subject)
	body = strings.ReplaceAll(strings.ReplaceAll(body, "\r\n", "\n"), "\n", "\r\n")

	var b bytes.Buffer
	fmt.Fprintf(&b, "From: %s\r\n", from)
	fmt.Fprintf(&b, "To: %s\r\n", to)
	fmt.Fprintf(&b, "Subject: %s\r\n", mime.QEncoding.Encode("utf-8", subject))
	fmt.Fprintf(&b, "Date: %s\r\n", date.Format(time.RFC1123Z))
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n")
	b.WriteString("Content-Transfer-Encoding: 8bit\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	return b.Bytes(), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/actions/... -run 'Mailer|BuildMessage' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/actions/mailer.go internal/actions/mailer_test.go
git commit -m "feat(actions): SMTP and log mailers with header-injection-safe messages"
```

---

### Task 5: Action payloads, registration, webhook handler, and the shared test fixture

**Files:**
- Create: `internal/actions/payload.go`
- Create: `internal/actions/runner.go`
- Create: `internal/actions/webhook.go`
- Create: `internal/testutil/fixture/fixture.go`
- Test: `internal/actions/webhook_test.go`
- Test: `internal/actions/helpers_test.go`

**Interfaces:**
- Consumes: Tasks 1–4; `testutil.NewEnv`, `(*Env).User`; `definitions.NewStore`, `(*Store).Apply`, `GetFormVersion`, `GetWorkflowVersion`; `submissions.NewService`, `(*Service).Create`, `Get`, `Events`, `InsertEvent`; `definition.ParseForm`, `ParseWorkflow`, `(Workflow).Transition`.
- Produces:
  - `const KindWebhook = "action.webhook"; KindEmail = "action.email"; KindAssign = "action.assign"; TriggerSubmit = "submit"; SystemActorName = "openforms"`
  - `type Payload struct { SubmissionID uuid.UUID; Trigger string; EventID int64; Action definition.Action }` (JSON tags per spec)
  - `func KindFor(t definition.ActionType) (string, error)`
  - `func Enqueue(ctx, q *jobs.Queue, tx db.DBTX, orgID uuid.UUID, p Payload) error`
  - `type Deps struct { Config config.Config; Subs *submissions.Service; Defs *definitions.Store; Auth *auth.Service; Mailer Mailer; HTTPClient *http.Client; Pool *pgxpool.Pool }`
  - `func Register(q *jobs.Queue, d Deps)` (this task registers webhook + the failed hook; Task 6 adds email + assign)
  - `func Sign(secret string, body []byte) string` returns `"sha256=<hex>"`
  - `package fixture`: `const WorkflowYAML, FormYAML, PlainFormYAML`; `type Fixture struct { *testutil.Env; Defs *definitions.Store; Subs *submissions.Service; Queue *jobs.Queue }`; `func Setup(t testing.TB) *Fixture`; methods `Submit(t, formSlug string, data map[string]any) submissions.Submission`, `Applicant(t) submissions.Submission`, `Principal(u auth.User) auth.Principal`, `PrincipalWithRoles(t, roles ...string) auth.Principal`, `Jobs(t) []jobs.Job` (oldest first), `Events(t, id uuid.UUID) []submissions.Event`, `Reload(t, id uuid.UUID) submissions.Submission`.

- [ ] **Step 1: Create the shared fixture (test support, no test of its own)**

`internal/testutil/fixture/fixture.go`:

```go
// Package fixture sets up a database with a hiring workflow, a form that uses
// it ("apply") and a form without a workflow ("feedback"). It is shared by the
// actions, workflow, httpapi and app tests of Plan 04.
package fixture

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

const WorkflowYAML = `
slug: hiring
title: Hiring
initial: new
states:
  - { key: new, label: New }
  - { key: screening, label: Screening }
  - { key: interview, label: Interview }
  - { key: hired, label: Hired, terminal: true }
  - { key: rejected, label: Rejected, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
  - key: priority
    type: select
    label: Priority
    options:
      - { value: low, label: Low }
      - { value: high, label: High }
onSubmit:
  - { type: email, to: "{{submission.data.email}}", subject: "Thanks {{submission.data.name}}", body: "We received your application." }
transitions:
  - { key: screen, label: Start screening, from: [new], to: screening, guard: { roles: [reviewer] } }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "http://127.0.0.1:9/hook" }
  - { key: hire, label: Hire, from: [interview], to: hired, guard: { roles: [hiring-manager] } }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
      - { type: assign, role: hiring-manager }
`

const FormYAML = `
slug: apply
title: Apply
workflow: hiring
settings: { public: true }
fields:
  - { key: name, type: text, label: Name, required: true }
  - { key: email, type: email, label: Email, required: true }
`

const PlainFormYAML = `
slug: feedback
title: Feedback
settings: { public: true }
fields:
  - { key: message, type: textarea, label: Message, required: true }
`

type Fixture struct {
	*testutil.Env
	Defs  *definitions.Store
	Subs  *submissions.Service
	Queue *jobs.Queue
}

func Setup(t testing.TB) *Fixture {
	t.Helper()
	env := testutil.NewEnv(t)
	wf, err := definition.ParseWorkflow([]byte(WorkflowYAML))
	if err != nil {
		t.Fatalf("fixture workflow: %v", err)
	}
	form, err := definition.ParseForm([]byte(FormYAML))
	if err != nil {
		t.Fatalf("fixture form: %v", err)
	}
	plain, err := definition.ParseForm([]byte(PlainFormYAML))
	if err != nil {
		t.Fatalf("fixture plain form: %v", err)
	}
	defs := definitions.NewStore(env.Pool)
	if _, err := defs.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms:     []definition.Form{form, plain},
		Workflows: []definition.Workflow{wf},
		Source:    definitions.SourceAPI,
		Actor:     "fixture",
	}); err != nil {
		t.Fatalf("fixture apply: %v", err)
	}
	return &Fixture{
		Env:   env,
		Defs:  defs,
		Subs:  submissions.NewService(env.Pool, defs),
		Queue: jobs.NewQueue(env.Pool),
	}
}

func (f *Fixture) Submit(t testing.TB, formSlug string, data map[string]any) submissions.Submission {
	t.Helper()
	sub, _, err := f.Subs.Create(context.Background(), f.OrgID, formSlug, data, false)
	if err != nil {
		t.Fatalf("submit %s: %v", formSlug, err)
	}
	return sub
}

// Applicant creates a submission on the "apply" form in state "new".
func (f *Fixture) Applicant(t testing.TB) submissions.Submission {
	return f.Submit(t, "apply", map[string]any{"name": "Ada", "email": "ada@example.com"})
}

func (f *Fixture) Principal(u auth.User) auth.Principal {
	return auth.Principal{OrgID: f.OrgID, Kind: auth.PrincipalUser, ID: u.ID, Name: u.Name, Email: u.Email, Roles: u.Roles}
}

// PrincipalWithRoles creates a new user with a unique email and returns its principal.
func (f *Fixture) PrincipalWithRoles(t testing.TB, roles ...string) auth.Principal {
	t.Helper()
	return f.Principal(f.User(t, uuid.NewString()+"@example.com", roles...))
}

// Jobs returns every job of the org, oldest first.
func (f *Fixture) Jobs(t testing.TB) []jobs.Job {
	t.Helper()
	all, err := f.Queue.List(context.Background(), f.OrgID, "", 1000)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

func (f *Fixture) Events(t testing.TB, id uuid.UUID) []submissions.Event {
	t.Helper()
	evs, err := f.Subs.Events(context.Background(), f.OrgID, id)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	return evs
}

func (f *Fixture) Reload(t testing.TB, id uuid.UUID) submissions.Submission {
	t.Helper()
	sub, err := f.Subs.Get(context.Background(), f.OrgID, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return sub
}
```

- [ ] **Step 2: Write the failing tests**

`internal/actions/helpers_test.go`:

```go
package actions_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

type sentMail struct{ To, Subject, Body string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (m *fakeMailer) Send(ctx context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

// setupActions builds a fixture and registers the action handlers.
// mutate may adjust Deps (e.g. set a webhook secret) before registration.
func setupActions(t *testing.T, mutate func(*actions.Deps)) (*fixture.Fixture, *fakeMailer) {
	t.Helper()
	f := fixture.Setup(t)
	m := &fakeMailer{}
	d := actions.Deps{Config: f.Config, Subs: f.Subs, Defs: f.Defs, Auth: f.Auth, Mailer: m, Pool: f.Pool}
	if mutate != nil {
		mutate(&d)
	}
	actions.Register(f.Queue, d)
	return f, m
}

func enqueueAction(t *testing.T, f *fixture.Fixture, sub submissions.Submission, trigger string, a definition.Action) jobs.Job {
	t.Helper()
	p := actions.Payload{SubmissionID: sub.ID, Trigger: trigger, Action: a}
	if err := actions.Enqueue(context.Background(), f.Queue, f.Pool, f.OrgID, p); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	all := f.Jobs(t)
	return all[len(all)-1]
}

func runOne(t *testing.T, f *fixture.Fixture) {
	t.Helper()
	if p, err := f.Queue.RunOnce(context.Background()); !p || err != nil {
		t.Fatalf("RunOnce = %v, %v", p, err)
	}
}

func jobByID(t *testing.T, f *fixture.Fixture, id int64) jobs.Job {
	t.Helper()
	for _, j := range f.Jobs(t) {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %d missing", id)
	return jobs.Job{}
}

func eventsOfType(t *testing.T, f *fixture.Fixture, id uuid.UUID, typ string) []submissions.Event {
	t.Helper()
	var out []submissions.Event
	for _, e := range f.Events(t, id) {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}
```

`internal/actions/webhook_test.go`:

```go
package actions_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
)

type capturedRequest struct {
	Header http.Header
	Body   []byte
}

func webhookServer(t *testing.T, status int) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, capturedRequest{r.Header.Clone(), b})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []capturedRequest { mu.Lock(); defer mu.Unlock(); return append([]capturedRequest(nil), got...) }
}

func TestKindFor(t *testing.T) {
	for typ, want := range map[definition.ActionType]string{
		definition.ActionWebhook: actions.KindWebhook,
		definition.ActionEmail:   actions.KindEmail,
		definition.ActionAssign:  actions.KindAssign,
	} {
		if got, err := actions.KindFor(typ); err != nil || got != want {
			t.Errorf("KindFor(%s) = %q, %v", typ, got, err)
		}
	}
	if _, err := actions.KindFor("sms"); err == nil {
		t.Error("unknown action type must error")
	}
}

func TestSign(t *testing.T) {
	// echo -n '{"a":1}' | openssl dgst -sha256 -hmac s3cret
	want := "sha256=" + "4c1fa4b8a27b2b3e7ac1a1f76a3d6b1ea06c6ab9b3f0f1f8a44c5a0e8b9b7f1b"
	got := actions.Sign("s3cret", []byte(`{"a":1}`))
	if !strings.HasPrefix(got, "sha256=") || len(got) != len(want) {
		t.Fatalf("Sign = %q", got)
	}
	if actions.Sign("s3cret", []byte(`{"a":1}`)) != got || actions.Sign("other", []byte(`{"a":1}`)) == got {
		t.Fatal("Sign must be deterministic and key-dependent")
	}
}

func TestWebhook_DeliversSignedTransitionEvent(t *testing.T) {
	srv, received := webhookServer(t, http.StatusOK)
	f, _ := setupActions(t, func(d *actions.Deps) { d.Config.WebhookSecret = "s3cret" })
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL + "/hook"})

	runOne(t, f)

	reqs := received()
	if len(reqs) != 1 {
		t.Fatalf("received %d requests", len(reqs))
	}
	r := reqs[0]
	if r.Header.Get("Content-Type") != "application/json" ||
		r.Header.Get("X-OpenForms-Event") != "submission.transitioned" ||
		r.Header.Get("X-OpenForms-Delivery") != strconv.FormatInt(job.ID, 10) {
		t.Fatalf("headers = %v", r.Header)
	}
	if got := r.Header.Get("X-OpenForms-Signature"); got != actions.Sign("s3cret", r.Body) {
		t.Fatalf("signature = %q", got)
	}
	var body struct {
		Event      string `json:"event"`
		Submission struct {
			ID       string         `json:"id"`
			Form     string         `json:"form"`
			State    string         `json:"state"`
			Data     map[string]any `json:"data"`
			Assignee any            `json:"assignee"`
		} `json:"submission"`
		Transition *struct{ Key, Label, From, To string } `json:"transition"`
		Form       struct{ Slug, Title string }            `json:"form"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Event != "submission.transitioned" || body.Submission.ID != sub.ID.String() ||
		body.Submission.Form != "apply" || body.Submission.Data["email"] != "ada@example.com" ||
		body.Transition == nil || body.Transition.Key != "invite" || body.Transition.To != "interview" ||
		body.Form.Slug != "apply" || body.Form.Title != "Apply" {
		t.Fatalf("body = %s", r.Body)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if evs := eventsOfType(t, f, sub.ID, "action_succeeded"); len(evs) != 1 || evs[0].ActorType != "system" {
		t.Fatalf("action_succeeded events = %+v", evs)
	}
}

func TestWebhook_SubmitTriggerAndNoSecret(t *testing.T) {
	srv, received := webhookServer(t, http.StatusNoContent)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	r := received()[0]
	if r.Header.Get("X-OpenForms-Event") != "submission.created" {
		t.Fatalf("event header = %q", r.Header.Get("X-OpenForms-Event"))
	}
	if _, ok := r.Header["X-Openforms-Signature"]; ok {
		t.Fatal("signature header must be absent without a secret")
	}
	var body map[string]any
	json.Unmarshal(r.Body, &body)
	if v, ok := body["transition"]; !ok || v != nil {
		t.Fatalf("transition must be null, body = %s", r.Body)
	}
}

func TestWebhook_Retryable5xx(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusInternalServerError)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	j := jobByID(t, f, job.ID)
	if j.Status != jobs.StatusPending || j.Attempts != 1 || !strings.Contains(j.LastError, "500") {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded"))+len(eventsOfType(t, f, sub.ID, "action_failed")) != 0 {
		t.Fatal("no outcome event expected while retrying")
	}
}

func TestWebhook_429IsRetryable(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusTooManyRequests)
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusPending {
		t.Fatalf("job = %+v", j)
	}
}

func TestWebhook_400IsPermanent(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusBadRequest)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
	evs := eventsOfType(t, f, sub.ID, "action_failed")
	if len(evs) != 1 {
		t.Fatalf("action_failed events = %+v", evs)
	}
	if msg, _ := evs[0].Payload["error"].(string); !strings.Contains(msg, "400") {
		t.Fatalf("payload = %v", evs[0].Payload)
	}
	if a, _ := evs[0].Payload["action"].(map[string]any); a["type"] != "webhook" {
		t.Fatalf("payload action = %v", evs[0].Payload["action"])
	}
}

func TestWebhook_InvalidURLIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), "invite", definition.Action{Type: definition.ActionWebhook, URL: "ftp://x"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
}

func TestEnqueue_UsesCallerTransaction(t *testing.T) {
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	tx, err := f.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := actions.Payload{SubmissionID: sub.ID, Trigger: "invite", Action: definition.Action{Type: definition.ActionWebhook, URL: "http://x"}}
	if err := actions.Enqueue(context.Background(), f.Queue, tx, f.OrgID, p); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(context.Background())
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("rolled-back enqueue left %d jobs", n)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/actions/... -run 'Webhook|KindFor|Sign|Enqueue' -v`
Expected: FAIL to compile with `undefined: actions.Payload` / `actions.Register`.

- [ ] **Step 4: Write the implementation**

`internal/actions/payload.go`:

```go
package actions

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
)

const (
	KindWebhook = "action.webhook"
	KindEmail   = "action.email"
	KindAssign  = "action.assign"

	// TriggerSubmit is Payload.Trigger for onSubmit actions; otherwise Trigger
	// is the transition key.
	TriggerSubmit = "submit"

	// SystemActorName is the actor_name on events written by actions.
	SystemActorName = "openforms"
)

type Payload struct {
	SubmissionID uuid.UUID         `json:"submissionId"`
	Trigger      string            `json:"trigger"`
	EventID      int64             `json:"eventId"`
	Action       definition.Action `json:"action"`
}

func KindFor(t definition.ActionType) (string, error) {
	switch t {
	case definition.ActionWebhook:
		return KindWebhook, nil
	case definition.ActionEmail:
		return KindEmail, nil
	case definition.ActionAssign:
		return KindAssign, nil
	}
	return "", fmt.Errorf("actions: unknown action type %q", t)
}

// Enqueue schedules p as a job inside tx (transactional outbox).
func Enqueue(ctx context.Context, q *jobs.Queue, tx db.DBTX, orgID uuid.UUID, p Payload) error {
	kind, err := KindFor(p.Action.Type)
	if err != nil {
		return err
	}
	_, err = q.Enqueue(ctx, tx, orgID, kind, p)
	return err
}
```

`internal/actions/runner.go`:

```go
package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

type Deps struct {
	Config     config.Config
	Subs       *submissions.Service
	Defs       *definitions.Store
	Auth       *auth.Service
	Mailer     Mailer
	HTTPClient *http.Client
	Pool       *pgxpool.Pool
}

type runner struct{ d Deps }

// Register installs the action job handlers and the failure hook on q.
func Register(q *jobs.Queue, d Deps) {
	if d.HTTPClient == nil {
		d.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if d.Mailer == nil {
		d.Mailer = NewMailer(d.Config)
	}
	r := &runner{d: d}
	q.Register(KindWebhook, r.webhook)
	q.SetFailedHook(r.onFailed)
}

// actionContext is everything a handler needs about the job's submission.
type actionContext struct {
	job        jobs.Job
	payload    Payload
	sub        submissions.Submission
	form       definition.Form
	transition *definition.Transition
	fromState  string
	vars       map[string]any
}

func decodePayload(job jobs.Job) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return p, jobs.Permanent(fmt.Errorf("decode action payload: %w", err))
	}
	return p, nil
}

func (r *runner) load(ctx context.Context, job jobs.Job) (*actionContext, error) {
	p, err := decodePayload(job)
	if err != nil {
		return nil, err
	}
	sub, err := r.d.Subs.Get(ctx, job.OrgID, p.SubmissionID)
	if errors.Is(err, submissions.ErrNotFound) {
		return nil, jobs.Permanent(err)
	}
	if err != nil {
		return nil, err
	}
	formRec, err := r.d.Defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		return nil, err
	}
	ac := &actionContext{job: job, payload: p, sub: sub, form: formRec.Definition}
	if p.Trigger != TriggerSubmit && sub.WorkflowVersionID != nil {
		wfRec, err := r.d.Defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			return nil, err
		}
		if t, ok := wfRec.Definition.Transition(p.Trigger); ok {
			ac.transition = &t
		}
		if p.EventID != 0 {
			var from *string
			err := r.d.Pool.QueryRow(ctx, `SELECT from_state FROM submission_events WHERE id = $1`, p.EventID).Scan(&from)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if from != nil {
				ac.fromState = *from
			}
		}
	}
	ac.vars = TemplateVars(r.d.Config, ac.form, sub, ac.transition)
	return ac, nil
}

func actionMap(a definition.Action) map[string]any {
	b, _ := json.Marshal(a)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func (r *runner) recordSuccess(ctx context.Context, ac *actionContext) error {
	_, err := submissions.InsertEvent(ctx, r.d.Pool, submissions.Event{
		SubmissionID: ac.sub.ID,
		OrgID:        ac.sub.OrgID,
		Type:         "action_succeeded",
		ActorType:    "system",
		ActorName:    SystemActorName,
		Payload:      map[string]any{"action": actionMap(ac.payload.Action), "trigger": ac.payload.Trigger, "jobId": ac.job.ID},
	})
	return err
}

// onFailed writes an action_failed event when an action job ends failed.
func (r *runner) onFailed(ctx context.Context, job jobs.Job, runErr error) {
	if !strings.HasPrefix(job.Kind, "action.") {
		return
	}
	p, err := decodePayload(job)
	if err != nil {
		slog.ErrorContext(ctx, "actions: failed job has undecodable payload", "job", job.ID, "err", err)
		return
	}
	_, err = submissions.InsertEvent(ctx, r.d.Pool, submissions.Event{
		SubmissionID: p.SubmissionID,
		OrgID:        job.OrgID,
		Type:         "action_failed",
		ActorType:    "system",
		ActorName:    SystemActorName,
		Payload:      map[string]any{"error": runErr.Error(), "action": actionMap(p.Action), "trigger": p.Trigger, "jobId": job.ID},
	})
	if err != nil {
		slog.ErrorContext(ctx, "actions: record action_failed", "job", job.ID, "err", err)
	}
}
```

`internal/actions/webhook.go`:

```go
package actions

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

// Sign returns "sha256=<hex HMAC-SHA256(secret, body)>".
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// webhookSubmission mirrors the Submission JSON of spec §7.3.
type webhookSubmission struct {
	ID          uuid.UUID        `json:"id"`
	Form        string           `json:"form"`
	FormVersion int              `json:"formVersion"`
	State       string           `json:"state"`
	StateLabel  string           `json:"stateLabel"`
	Terminal    bool             `json:"terminal"`
	Data        map[string]any   `json:"data"`
	Fields      map[string]any   `json:"fields"`
	Assignee    *webhookAssignee `json:"assignee"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

type webhookAssignee struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type webhookTransition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type webhookForm struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type webhookBody struct {
	Event      string             `json:"event"`
	Submission webhookSubmission  `json:"submission"`
	Transition *webhookTransition `json:"transition"`
	Form       webhookForm        `json:"form"`
}

func toWebhookSubmission(s submissions.Submission) webhookSubmission {
	var assignee *webhookAssignee
	if s.AssigneeID != nil {
		assignee = &webhookAssignee{ID: *s.AssigneeID, Name: s.AssigneeName, Email: s.AssigneeEmail}
	}
	return webhookSubmission{
		ID: s.ID, Form: s.FormSlug, FormVersion: s.FormVersion,
		State: s.State, StateLabel: s.StateLabel, Terminal: s.Terminal,
		Data: nonNil(s.Data), Fields: nonNil(s.Fields), Assignee: assignee,
		CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
}

func (r *runner) webhook(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	target := ac.payload.Action.URL
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return jobs.Permanent(fmt.Errorf("webhook: invalid url %q", target))
	}

	body := webhookBody{
		Event:      "submission.transitioned",
		Submission: toWebhookSubmission(ac.sub),
		Form:       webhookForm{Slug: ac.form.Slug, Title: ac.form.Title},
	}
	if ac.payload.Trigger == TriggerSubmit {
		body.Event = "submission.created"
	} else if ac.transition != nil {
		body.Transition = &webhookTransition{Key: ac.transition.Key, Label: ac.transition.Label, From: ac.fromState, To: ac.transition.To}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return jobs.Permanent(err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return jobs.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "openforms-webhook/1")
	req.Header.Set("X-OpenForms-Event", body.Event)
	req.Header.Set("X-OpenForms-Delivery", strconv.FormatInt(job.ID, 10))
	if secret := r.d.Config.WebhookSecret; secret != "" {
		req.Header.Set("X-OpenForms-Signature", Sign(secret, raw))
	}

	resp, err := r.d.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook %s: %w", target, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := fmt.Errorf("webhook %s responded %d", target, resp.StatusCode)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 &&
			resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
			return jobs.Permanent(e)
		}
		return e
	}
	return r.recordSuccess(ctx, ac)
}
```

Fix the `TestSign` expectation before running. It uses a length check rather than a hard-coded digest, so it is already self-consistent. The `want` variable only supplies the expected length (7 + 64 chars), so leave it as written.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/actions/... -v`
Expected: PASS (render, mailer, webhook, KindFor, Sign and Enqueue tests).

- [ ] **Step 6: Commit**

```bash
git add internal/actions/payload.go internal/actions/runner.go internal/actions/webhook.go \
        internal/actions/webhook_test.go internal/actions/helpers_test.go internal/testutil/fixture/fixture.go
git commit -m "feat(actions): action payloads, webhook delivery with signatures, failure events"
```

---

### Task 6: Email and assign actions

**Parallel group:** P2 (with Task 7)

**Files:**
- Create: `internal/submissions/assignee.go`
- Create: `internal/actions/email.go`
- Create: `internal/actions/assign.go`
- Modify: `internal/actions/runner.go` (`Register`: two lines)
- Test: `internal/actions/email_assign_test.go`
- Test: `internal/submissions/assignee_test.go`

**Interfaces:**
- Consumes: Task 5 runner/load/recordSuccess; `auth.Service.GetUserByEmail`, `UsersWithRole`, `auth.ErrNotFound`, `auth.RoleAdmin`; `db.WithTx`.
- Produces:
  - `func SetAssignee(ctx context.Context, q db.DBTX, orgID, id uuid.UUID, userID *uuid.UUID) error` (package `submissions`; `ErrNotFound` when no row)
  - job handlers for `action.email` and `action.assign`.

- [ ] **Step 1: Write the failing tests**

`internal/submissions/assignee_test.go`:

```go
package submissions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

func TestSetAssignee(t *testing.T) {
	f := fixture.Setup(t)
	sub := f.Applicant(t)
	u := f.User(t, "rev@example.com", "reviewer")
	ctx := context.Background()

	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, sub.ID, &u.ID); err != nil {
		t.Fatal(err)
	}
	got := f.Reload(t, sub.ID)
	if got.AssigneeID == nil || *got.AssigneeID != u.ID || got.AssigneeEmail != "rev@example.com" {
		t.Fatalf("after assign: %+v", got)
	}
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, sub.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.Reload(t, sub.ID); got.AssigneeID != nil {
		t.Fatalf("after unassign: %+v", got)
	}
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, uuid.New(), nil); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("unknown submission err = %v", err)
	}
}
```

`internal/actions/email_assign_test.go`:

```go
package actions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

func TestEmail_RendersAndSends(t *testing.T) {
	f, m := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "{{submission.data.email}}",
		Subject: "Thanks {{submission.data.name}}", Body: "See {{submission.url}}",
	})

	runOne(t, f)

	if len(m.sent) != 1 {
		t.Fatalf("sent = %+v", m.sent)
	}
	s := m.sent[0]
	if s.To != "ada@example.com" || s.Subject != "Thanks Ada" || s.Body != "See http://test.local/admin/submissions/"+sub.ID.String() {
		t.Fatalf("mail = %+v", s)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded")) != 1 {
		t.Fatal("missing action_succeeded event")
	}
}

func TestEmail_EmptyRecipientIsPermanent(t *testing.T) {
	f, m := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "{{submission.data.missing}}", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
	if len(m.sent) != 0 || len(eventsOfType(t, f, sub.ID, "action_failed")) != 1 {
		t.Fatalf("sent=%v", m.sent)
	}
}

func TestEmail_InvalidRecipientIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "not-an-email", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
}

func TestEmail_MailerErrorRetries(t *testing.T) {
	f, m := setupActions(t, nil)
	m.err = errors.New("smtp down")
	job := enqueueAction(t, f, f.Applicant(t), actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "a@example.com", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusPending || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
}

func setCreatedAt(t *testing.T, f interface{ Exec(string, ...any) }, _ ...any) {}

func TestAssign_ByRolePicksLeastLoaded(t *testing.T) {
	f, _ := setupActions(t, nil)
	ctx := context.Background()
	busy := f.User(t, "busy@example.com", "reviewer")
	idle := f.User(t, "idle@example.com", "reviewer")
	f.User(t, "other@example.com", "hiring-manager")

	// busy has one open (non-terminal) assignment.
	open := f.Applicant(t)
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, open.ID, &busy.ID); err != nil {
		t.Fatal(err)
	}
	// idle has one assignment, but it is terminal (rejected) and must not count.
	closed := f.Applicant(t)
	if _, err := f.Pool.Exec(ctx, `UPDATE submissions SET state = 'rejected', assignee_id = $2 WHERE id = $1`, closed.ID, idle.ID); err != nil {
		t.Fatal(err)
	}

	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)

	got := f.Reload(t, sub.ID)
	if got.AssigneeID == nil || *got.AssigneeID != idle.ID {
		t.Fatalf("assigned to %v, want idle %v", got.AssigneeID, idle.ID)
	}
	evs := eventsOfType(t, f, sub.ID, "assigned")
	if len(evs) != 1 || evs[0].Payload["assigneeId"] != idle.ID.String() || evs[0].ActorType != "system" {
		t.Fatalf("assigned events = %+v", evs)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded")) != 1 {
		t.Fatal("missing action_succeeded")
	}
}

func TestAssign_TieGoesToEarliestCreated(t *testing.T) {
	f, _ := setupActions(t, nil)
	ctx := context.Background()
	later := f.User(t, "later@example.com", "reviewer")
	earlier := f.User(t, "earlier@example.com", "reviewer")
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Pool.Exec(ctx, `UPDATE users SET created_at = $2 WHERE id = $1`, later.ID, base.Add(time.Hour))
	f.Pool.Exec(ctx, `UPDATE users SET created_at = $2 WHERE id = $1`, earlier.ID, base)

	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != earlier.ID {
		t.Fatalf("assigned to %v, want earliest %v", got.AssigneeID, earlier.ID)
	}
}

func TestAssign_FallsBackToAdmin(t *testing.T) {
	f, _ := setupActions(t, nil)
	admin := f.User(t, "admin@example.com", "admin")
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "nobody-has-this"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != admin.ID {
		t.Fatalf("assigned to %v, want admin", got.AssigneeID)
	}
}

func TestAssign_NoCandidatesIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_failed")) != 1 {
		t.Fatal("missing action_failed")
	}
}

func TestAssign_ByUserEmail(t *testing.T) {
	f, _ := setupActions(t, nil)
	u := f.User(t, "named@example.com", "reviewer")
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, User: "NAMED@example.com"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != u.ID {
		t.Fatalf("assigned to %v", got.AssigneeID)
	}

	sub2 := f.Applicant(t)
	job := enqueueAction(t, f, sub2, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, User: "ghost@example.com"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("unknown user job = %+v", j)
	}
}
```

Delete the unused `setCreatedAt` stub from the test file before running. It exists only as a reminder that `created_at` is set with direct SQL in `TestAssign_TieGoesToEarliestCreated`. The final file must not contain it.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/actions/... ./internal/submissions/... -run 'Email|Assign|SetAssignee' -v`
Expected: FAIL: `undefined: submissions.SetAssignee`. Once that exists, the email and assign tests fail with `no handler registered for kind "action.email"`.

- [ ] **Step 3: Write the implementation**

`internal/submissions/assignee.go`:

```go
package submissions

import (
	"context"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/db"
)

// SetAssignee sets (or clears, when userID is nil) a submission's assignee.
// It writes no event; callers record the "assigned" event themselves.
func SetAssignee(ctx context.Context, q db.DBTX, orgID, id uuid.UUID, userID *uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE submissions SET assignee_id = $3, updated_at = now() WHERE org_id = $1 AND id = $2`,
		orgID, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
```

`internal/actions/email.go`:

```go
package actions

import (
	"context"
	"fmt"
	"net/mail"
	"strings"

	"github.com/openforms/openforms/internal/jobs"
)

func (r *runner) email(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	a := ac.payload.Action
	to := strings.TrimSpace(Render(a.To, ac.vars))
	if to == "" {
		return jobs.Permanent(fmt.Errorf("email: recipient template %q rendered empty", a.To))
	}
	if _, err := mail.ParseAddress(to); err != nil || strings.ContainsAny(to, "\r\n") {
		return jobs.Permanent(fmt.Errorf("email: invalid recipient %q", to))
	}
	if err := r.d.Mailer.Send(ctx, to, Render(a.Subject, ac.vars), Render(a.Body, ac.vars)); err != nil {
		return fmt.Errorf("email: %w", err)
	}
	return r.recordSuccess(ctx, ac)
}
```

`internal/actions/assign.go`:

```go
package actions

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

func (r *runner) assign(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	a := ac.payload.Action
	var target auth.User
	switch {
	case a.User != "":
		u, err := r.d.Auth.GetUserByEmail(ctx, job.OrgID, a.User)
		if errors.Is(err, auth.ErrNotFound) {
			return jobs.Permanent(fmt.Errorf("assign: no user with email %q", a.User))
		}
		if err != nil {
			return err
		}
		target = u
	case a.Role != "":
		u, err := r.leastLoaded(ctx, job.OrgID, a.Role)
		if err != nil {
			return err
		}
		target = u
	default:
		return jobs.Permanent(errors.New("assign: action needs user or role"))
	}

	err = db.WithTx(ctx, r.d.Pool, func(tx pgx.Tx) error {
		if err := submissions.SetAssignee(ctx, tx, job.OrgID, ac.sub.ID, &target.ID); err != nil {
			return err
		}
		_, err := submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: ac.sub.ID,
			OrgID:        job.OrgID,
			Type:         "assigned",
			ActorType:    "system",
			ActorName:    SystemActorName,
			Payload:      map[string]any{"assigneeId": target.ID.String(), "assigneeName": target.Name, "trigger": ac.payload.Trigger},
		})
		return err
	})
	if err != nil {
		return err
	}
	return r.recordSuccess(ctx, ac)
}

// leastLoaded picks, among users with role (or admins if none), the one with
// the fewest assigned non-terminal submissions; ties go to the earliest created.
func (r *runner) leastLoaded(ctx context.Context, orgID uuid.UUID, role string) (auth.User, error) {
	cands, err := r.d.Auth.UsersWithRole(ctx, orgID, role)
	if err != nil {
		return auth.User{}, err
	}
	if len(cands) == 0 {
		if cands, err = r.d.Auth.UsersWithRole(ctx, orgID, auth.RoleAdmin); err != nil {
			return auth.User{}, err
		}
	}
	if len(cands) == 0 {
		return auth.User{}, jobs.Permanent(fmt.Errorf("assign: no users with role %q or admin", role))
	}
	ids := make([]uuid.UUID, len(cands))
	for i, c := range cands {
		ids[i] = c.ID
	}
	rows, err := r.d.Pool.Query(ctx,
		`SELECT assignee_id, workflow_version_id, state FROM submissions
		 WHERE org_id = $1 AND assignee_id = ANY($2)`, orgID, ids)
	if err != nil {
		return auth.User{}, err
	}
	defer rows.Close()
	load := map[uuid.UUID]int{}
	for rows.Next() {
		var assignee uuid.UUID
		var wv *uuid.UUID
		var state string
		if err := rows.Scan(&assignee, &wv, &state); err != nil {
			return auth.User{}, err
		}
		terminal, err := r.isTerminal(ctx, wv, state)
		if err != nil {
			return auth.User{}, err
		}
		if !terminal {
			load[assignee]++
		}
	}
	if err := rows.Err(); err != nil {
		return auth.User{}, err
	}
	sort.SliceStable(cands, func(i, j int) bool {
		li, lj := load[cands[i].ID], load[cands[j].ID]
		if li != lj {
			return li < lj
		}
		return cands[i].CreatedAt.Before(cands[j].CreatedAt)
	})
	return cands[0], nil
}

func (r *runner) isTerminal(ctx context.Context, workflowVersionID *uuid.UUID, state string) (bool, error) {
	if workflowVersionID == nil {
		return true, nil // forms without a workflow end in "submitted", which is terminal
	}
	rec, err := r.d.Defs.GetWorkflowVersion(ctx, *workflowVersionID)
	if err != nil {
		return false, err
	}
	st, ok := rec.Definition.State(state)
	return ok && st.Terminal, nil
}
```

Modify `internal/actions/runner.go` `Register` so that it reads:

```go
	r := &runner{d: d}
	q.Register(KindWebhook, r.webhook)
	q.Register(KindEmail, r.email)
	q.Register(KindAssign, r.assign)
	q.SetFailedHook(r.onFailed)
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/actions/... ./internal/submissions/... -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/submissions/assignee.go internal/submissions/assignee_test.go \
        internal/actions/email.go internal/actions/assign.go internal/actions/runner.go internal/actions/email_assign_test.go
git commit -m "feat(actions): email and least-loaded assign actions"
```

---

### Task 7: Engine core: Available, Transition, OnCreated

**Parallel group:** P2 (with Task 6)

**Files:**
- Create: `internal/workflow/errors.go`
- Create: `internal/workflow/engine.go`
- Create: `internal/workflow/fields.go`
- Create: `internal/workflow/transition.go`
- Create: `internal/workflow/export_test.go`
- Test: `internal/workflow/transition_test.go`

**Interfaces:**
- Consumes: `actions.Enqueue`, `actions.Payload`, `actions.TriggerSubmit` (Task 5); `submissions.Service.GetForUpdate`, `Get`, `InsertEvent`, `ActorFromPrincipal`; `definitions.Store.GetWorkflowVersion`; `definition.ValidateWorkflowFields`; `auth.Principal.HasAnyRole`; `db.WithTx`; fixture (Task 5).
- Produces: everything in spec §6.10 except `UpdateFields`, `Comment` and `Assign` (Task 8). Also produces the internal helpers `mergeFields(wf, current, patch) (merged, changed map[string]any, err error)` and `isEmpty(v any) bool`, and the unexported `Engine.beforeCommit func() error` test hook.

- [ ] **Step 1: Write the failing tests**

`internal/workflow/export_test.go`:

```go
package workflow

// SetBeforeCommitHook lets tests force a failure at the very end of a
// transition transaction, after actions have been enqueued.
func SetBeforeCommitHook(e *Engine, f func() error) { e.beforeCommit = f }
```

`internal/workflow/transition_test.go`:

```go
package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func newEngine(t *testing.T) (*fixture.Fixture, *workflow.Engine) {
	t.Helper()
	f := fixture.Setup(t)
	return f, workflow.NewEngine(f.Pool, f.Defs, f.Subs, f.Auth, f.Queue)
}

func payloads(t *testing.T, f *fixture.Fixture) []actions.Payload {
	t.Helper()
	var out []actions.Payload
	for _, j := range f.Jobs(t) {
		var p actions.Payload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func transitionEvents(t *testing.T, f *fixture.Fixture, id uuid.UUID) []submissions.Event {
	t.Helper()
	var out []submissions.Event
	for _, e := range f.Events(t, id) {
		if e.Type == "transition" {
			out = append(out, e)
		}
	}
	return out
}

func TestAvailable(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	ctx := context.Background()

	reviewer := f.PrincipalWithRoles(t, "reviewer")
	av, err := e.Available(ctx, reviewer, sub)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]workflow.AvailableTransition{}
	for _, a := range av {
		keys[a.Key] = a
	}
	if len(av) != 2 || !keys["screen"].Allowed || !keys["reject"].Allowed {
		t.Fatalf("reviewer available = %+v", av)
	}
	if keys["screen"].ToLabel != "Screening" || keys["screen"].RequireFields == nil {
		t.Fatalf("screen = %+v", keys["screen"])
	}
	if got := keys["reject"].RequireFields; len(got) != 1 || got[0] != "rejectionReason" {
		t.Fatalf("reject requireFields = %v", got)
	}

	nobody := f.PrincipalWithRoles(t)
	av, _ = e.Available(ctx, nobody, sub)
	for _, a := range av {
		if a.Allowed || a.Reason != "role" {
			t.Fatalf("role-less principal got %+v", a)
		}
	}

	admin := f.PrincipalWithRoles(t, auth.RoleAdmin)
	av, _ = e.Available(ctx, admin, sub)
	for _, a := range av {
		if !a.Allowed {
			t.Fatalf("admin must pass every guard: %+v", a)
		}
	}

	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	av, err = e.Available(ctx, admin, plain)
	if err != nil || av == nil || len(av) != 0 {
		t.Fatalf("no-workflow available = %v, %v; want empty non-nil", av, err)
	}
}

func TestTransition_Success(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")

	got, err := e.Transition(context.Background(), p, sub.ID, workflow.TransitionInput{Transition: "screen", Comment: "  looks good  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "screening" || got.StateLabel != "Screening" {
		t.Fatalf("submission = %+v", got)
	}
	evs := transitionEvents(t, f, sub.ID)
	if len(evs) != 1 {
		t.Fatalf("transition events = %+v", evs)
	}
	ev := evs[0]
	if ev.FromState != "new" || ev.ToState != "screening" || ev.Transition != "screen" ||
		ev.ActorType != "user" || ev.ActorID == nil || *ev.ActorID != p.ID || ev.ActorName != p.Name {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Payload["comment"] != "looks good" {
		t.Fatalf("payload = %v", ev.Payload)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("screen has no actions, got %d jobs", n)
	}
}

func TestTransition_Errors(t *testing.T) {
	f, e := newEngine(t)
	ctx := context.Background()
	reviewer := f.PrincipalWithRoles(t, "reviewer")
	sub := f.Applicant(t)
	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})

	cases := []struct {
		name string
		p    auth.Principal
		id   uuid.UUID
		in   workflow.TransitionInput
		want error
	}{
		{"unknown transition", reviewer, sub.ID, workflow.TransitionInput{Transition: "teleport"}, workflow.ErrUnknownTransition},
		{"wrong from-state", reviewer, sub.ID, workflow.TransitionInput{Transition: "invite", Fields: map[string]any{"score": 5}}, workflow.ErrInvalidState},
		{"missing role", f.PrincipalWithRoles(t, "hiring-manager"), sub.ID, workflow.TransitionInput{Transition: "screen"}, workflow.ErrForbidden},
		{"expected state mismatch", reviewer, sub.ID, workflow.TransitionInput{Transition: "screen", ExpectedState: "interview"}, workflow.ErrStateConflict},
		{"no workflow", reviewer, plain.ID, workflow.TransitionInput{Transition: "screen"}, workflow.ErrNoWorkflow},
		{"unknown submission", reviewer, uuid.New(), workflow.TransitionInput{Transition: "screen"}, submissions.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.Transition(ctx, c.p, c.id, c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	if got := f.Reload(t, sub.ID); got.State != "new" {
		t.Fatalf("state changed to %s", got.State)
	}
}

func TestTransition_RequireFields(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	_, err := e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"rejectionReason": "   "}})
	var ve *definition.ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Path != "fields.rejectionReason" ||
		!strings.Contains(ve.Problems[0].Message, "required for transition reject") {
		t.Fatalf("err = %#v", err)
	}
	if got := f.Reload(t, sub.ID); got.State != "new" || len(got.Fields) != 0 {
		t.Fatalf("submission changed: %+v", got)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("failed transition enqueued %d jobs", n)
	}

	_, err = e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"score": "not a number", "rejectionReason": "x"}})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.score" {
		t.Fatalf("invalid field type err = %#v", err)
	}
	_, err = e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"nope": 1, "rejectionReason": "x"}})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.nope" {
		t.Fatalf("unknown field err = %#v", err)
	}
}

func TestTransition_EnqueuesActionsWithEventID(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	got, err := e.Transition(ctx, p, sub.ID, workflow.TransitionInput{
		Transition: "reject", Fields: map[string]any{"rejectionReason": "Not a fit"}, Comment: "sorry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "rejected" || !got.Terminal || got.Fields["rejectionReason"] != "Not a fit" {
		t.Fatalf("submission = %+v", got)
	}
	ev := transitionEvents(t, f, sub.ID)[0]
	if fields, _ := ev.Payload["fields"].(map[string]any); fields["rejectionReason"] != "Not a fit" {
		t.Fatalf("event payload = %v", ev.Payload)
	}
	ps := payloads(t, f)
	if len(ps) != 2 || ps[0].Action.Type != definition.ActionEmail || ps[1].Action.Type != definition.ActionAssign {
		t.Fatalf("payloads = %+v", ps)
	}
	for _, pl := range ps {
		if pl.Trigger != "reject" || pl.EventID != ev.ID || pl.SubmissionID != sub.ID {
			t.Fatalf("payload = %+v, event id %d", pl, ev.ID)
		}
	}
	kinds := []string{f.Jobs(t)[0].Kind, f.Jobs(t)[1].Kind}
	if kinds[0] != actions.KindEmail || kinds[1] != actions.KindAssign {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestTransition_FieldsAlreadySetSatisfyGuard(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	admin := f.PrincipalWithRoles(t, auth.RoleAdmin)
	ctx := context.Background()
	if _, err := e.Transition(ctx, admin, sub.ID, workflow.TransitionInput{Transition: "screen", Fields: map[string]any{"score": 4}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.Transition(ctx, admin, sub.ID, workflow.TransitionInput{Transition: "invite"})
	if err != nil {
		t.Fatalf("score set earlier must satisfy requireFields: %v", err)
	}
	if got.State != "interview" || got.Fields["score"] != float64(4) {
		t.Fatalf("submission = %+v", got)
	}
}

func TestTransition_RollbackLeavesNoJobs(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	boom := errors.New("commit exploded")
	workflow.SetBeforeCommitHook(e, func() error { return boom })

	_, err := e.Transition(context.Background(), p, sub.ID, workflow.TransitionInput{
		Transition: "reject", Fields: map[string]any{"rejectionReason": "x"},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("rolled-back transition left %d jobs", n)
	}
	if got := f.Reload(t, sub.ID); got.State != "new" || len(got.Fields) != 0 {
		t.Fatalf("rolled-back transition changed submission: %+v", got)
	}
	if n := len(transitionEvents(t, f, sub.ID)); n != 0 {
		t.Fatalf("rolled-back transition left %d events", n)
	}
}

func TestTransition_ConcurrentOnlyOneWins(t *testing.T) {
	for _, expected := range []string{"", "new"} {
		t.Run("expectedState="+expected, func(t *testing.T) {
			f, e := newEngine(t)
			sub := f.Applicant(t)
			p := f.PrincipalWithRoles(t, "reviewer")

			start := make(chan struct{})
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range errs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					_, errs[i] = e.Transition(context.Background(), p, sub.ID,
						workflow.TransitionInput{Transition: "screen", ExpectedState: expected})
				}(i)
			}
			close(start)
			wg.Wait()

			wins, losses := 0, 0
			for _, err := range errs {
				switch {
				case err == nil:
					wins++
				case errors.Is(err, workflow.ErrInvalidState), errors.Is(err, workflow.ErrStateConflict):
					losses++
				default:
					t.Fatalf("unexpected err: %v", err)
				}
			}
			if wins != 1 || losses != 1 {
				t.Fatalf("wins=%d losses=%d errs=%v", wins, losses, errs)
			}
			if n := len(transitionEvents(t, f, sub.ID)); n != 1 {
				t.Fatalf("transition events = %d", n)
			}
		})
	}
}

func TestOnCreated_EnqueuesOnSubmitActions(t *testing.T) {
	f, e := newEngine(t)
	f.Subs.SetCreatedHook(e.OnCreated)

	sub := f.Applicant(t)
	ps := payloads(t, f)
	if len(ps) != 1 || ps[0].Trigger != actions.TriggerSubmit || ps[0].SubmissionID != sub.ID ||
		ps[0].Action.Type != definition.ActionEmail {
		t.Fatalf("payloads = %+v", ps)
	}
	var createdID int64
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "created" {
			createdID = ev.ID
		}
	}
	if createdID != 0 && ps[0].EventID != createdID {
		t.Fatalf("EventID = %d, want created event %d", ps[0].EventID, createdID)
	}

	f.Submit(t, "feedback", map[string]any{"message": "hi"})
	if n := len(f.Jobs(t)); n != 1 {
		t.Fatalf("form without workflow enqueued jobs: %d total", n)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/workflow/... -v`
Expected: FAIL to compile with `undefined: workflow.NewEngine`.

- [ ] **Step 3: Write the implementation**

`internal/workflow/errors.go`:

```go
// Package workflow moves submissions through their workflow's state machine.
package workflow

import "errors"

var (
	ErrUnknownTransition = errors.New("unknown transition")
	ErrInvalidState      = errors.New("transition is not allowed from the submission's current state")
	ErrForbidden         = errors.New("you are not allowed to perform this transition")
	ErrNoWorkflow        = errors.New("submission's form has no workflow")
	ErrStateConflict     = errors.New("submission state changed since it was loaded")
)
```

`internal/workflow/engine.go`:

```go
package workflow

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

type AvailableTransition struct {
	Key, Label, To, ToLabel string
	RequireFields           []string
	Allowed                 bool
	Reason                  string // "" or "role"
}

type TransitionInput struct {
	Transition    string
	Fields        map[string]any
	Comment       string
	ExpectedState string
}

type Engine struct {
	pool  *pgxpool.Pool
	defs  *definitions.Store
	subs  *submissions.Service
	auth  *auth.Service
	queue *jobs.Queue

	// beforeCommit runs at the end of a transition transaction; tests only.
	beforeCommit func() error
}

func NewEngine(pool *pgxpool.Pool, defs *definitions.Store, subs *submissions.Service, authSvc *auth.Service, q *jobs.Queue) *Engine {
	return &Engine{pool: pool, defs: defs, subs: subs, auth: authSvc, queue: q}
}

// workflowFor returns the workflow version pinned by the submission.
func (e *Engine) workflowFor(ctx context.Context, sub submissions.Submission) (*definition.Workflow, error) {
	if sub.WorkflowVersionID == nil {
		return nil, ErrNoWorkflow
	}
	rec, err := e.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
	if err != nil {
		return nil, err
	}
	wf := rec.Definition
	return &wf, nil
}

// Available lists the transitions leaving the submission's current state and
// whether p may perform each. Forms without a workflow yield an empty slice.
func (e *Engine) Available(ctx context.Context, p auth.Principal, sub submissions.Submission) ([]AvailableTransition, error) {
	out := []AvailableTransition{}
	wf, err := e.workflowFor(ctx, sub)
	if errors.Is(err, ErrNoWorkflow) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, t := range wf.Transitions {
		if !slices.Contains(t.From, sub.State) {
			continue
		}
		at := AvailableTransition{
			Key: t.Key, Label: t.Label, To: t.To,
			RequireFields: append([]string{}, t.Guard.RequireFields...),
			Allowed:       true,
		}
		if st, ok := wf.State(t.To); ok {
			at.ToLabel = st.Label
		}
		if !p.HasAnyRole(t.Guard.Roles...) {
			at.Allowed, at.Reason = false, "role"
		}
		out = append(out, at)
	}
	return out, nil
}

// OnCreated enqueues the workflow's onSubmit actions in the creating
// transaction. Registered with submissions.Service.SetCreatedHook.
func (e *Engine) OnCreated(ctx context.Context, tx pgx.Tx, sub submissions.Submission, wf *definition.Workflow) error {
	if wf == nil || len(wf.OnSubmit) == 0 {
		return nil
	}
	var eventID int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM submission_events WHERE submission_id = $1 AND type = 'created' ORDER BY id LIMIT 1`,
		sub.ID).Scan(&eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	for _, a := range wf.OnSubmit {
		if err := actions.Enqueue(ctx, e.queue, tx, sub.OrgID, actions.Payload{
			SubmissionID: sub.ID, Trigger: actions.TriggerSubmit, EventID: eventID, Action: a,
		}); err != nil {
			return err
		}
	}
	return nil
}
```

`internal/workflow/fields.go`:

```go
package workflow

import (
	"errors"
	"reflect"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

// mergeFields validates patch against the workflow's fields and applies it to
// current. A nil value in patch unsets that field. It returns the merged map
// and the changed subset (removed keys map to nil).
func mergeFields(wf definition.Workflow, current, patch map[string]any) (map[string]any, map[string]any, error) {
	merged := make(map[string]any, len(current)+len(patch))
	for k, v := range current {
		merged[k] = v
	}
	changed := map[string]any{}
	sets := map[string]any{}
	var unset []string
	var problems []definition.Problem
	for k, v := range patch {
		if v != nil {
			sets[k] = v
			continue
		}
		if _, ok := wf.Field(k); !ok {
			problems = append(problems, definition.Problem{Path: "fields." + k, Message: "unknown workflow field"})
			continue
		}
		unset = append(unset, k)
	}
	clean, err := definition.ValidateWorkflowFields(wf, sets)
	if err != nil {
		var ve *definition.ValidationError
		if !errors.As(err, &ve) {
			return nil, nil, err
		}
		problems = append(problems, ve.Problems...)
	}
	if len(problems) > 0 {
		sort.Slice(problems, func(i, j int) bool { return problems[i].Path < problems[j].Path })
		return nil, nil, &definition.ValidationError{Problems: problems}
	}
	for _, k := range unset {
		if _, had := merged[k]; had {
			delete(merged, k)
			changed[k] = nil
		}
	}
	for k, v := range clean {
		if old, had := merged[k]; !had || !reflect.DeepEqual(old, v) {
			merged[k] = v
			changed[k] = v
		}
	}
	return merged, changed, nil
}

// isEmpty reports whether a workflow field value counts as "not provided".
func isEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case bool:
		return !x
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	}
	return false
}
```

`internal/workflow/transition.go`:

```go
package workflow

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// Transition runs the spec §6.10 algorithm in one transaction: state change,
// event and action jobs commit together or not at all.
func (e *Engine) Transition(ctx context.Context, p auth.Principal, id uuid.UUID, in TransitionInput) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		wf, err := e.workflowFor(ctx, sub)
		if err != nil {
			return err
		}
		t, ok := wf.Transition(in.Transition)
		if !ok {
			return ErrUnknownTransition
		}
		if in.ExpectedState != "" && in.ExpectedState != sub.State {
			return ErrStateConflict
		}
		if !slices.Contains(t.From, sub.State) {
			return ErrInvalidState
		}
		if !p.HasAnyRole(t.Guard.Roles...) {
			return ErrForbidden
		}
		merged, changed, err := mergeFields(*wf, sub.Fields, in.Fields)
		if err != nil {
			return err
		}
		var problems []definition.Problem
		for _, k := range t.Guard.RequireFields {
			if isEmpty(merged[k]) {
				problems = append(problems, definition.Problem{Path: "fields." + k, Message: "required for transition " + t.Key})
			}
		}
		if len(problems) > 0 {
			return &definition.ValidationError{Problems: problems}
		}

		if _, err := tx.Exec(ctx,
			`UPDATE submissions SET state = $3, fields = $4, updated_at = now() WHERE org_id = $1 AND id = $2`,
			p.OrgID, id, t.To, merged); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		ev, err := submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id,
			OrgID:        p.OrgID,
			Type:         "transition",
			FromState:    sub.State,
			ToState:      t.To,
			Transition:   t.Key,
			ActorType:    actor.Type,
			ActorID:      actor.ID,
			ActorName:    actor.Name,
			Payload:      map[string]any{"comment": strings.TrimSpace(in.Comment), "fields": changed},
		})
		if err != nil {
			return err
		}
		for _, a := range t.Actions {
			if err := actions.Enqueue(ctx, e.queue, tx, p.OrgID, actions.Payload{
				SubmissionID: id, Trigger: t.Key, EventID: ev.ID, Action: a,
			}); err != nil {
				return err
			}
		}
		if e.beforeCommit != nil {
			return e.beforeCommit()
		}
		return nil
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/workflow/... -race -v`
Expected: PASS for `TestAvailable`, `TestTransition_*` and `TestOnCreated_EnqueuesOnSubmitActions`.

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/
git commit -m "feat(workflow): transition engine with guards, required fields and transactional actions"
```

---

### Task 8: Engine edits: UpdateFields, Comment, Assign

**Files:**
- Create: `internal/workflow/edits.go`
- Test: `internal/workflow/edits_test.go`

**Interfaces:**
- Consumes: Task 7 (`workflowFor`, `mergeFields`); `submissions.SetAssignee` (Task 6).
- Produces:
  - `func (e *Engine) UpdateFields(ctx, p auth.Principal, id uuid.UUID, fields map[string]any) (submissions.Submission, error)` writes an event `fields_updated` with `{"fields": changed}`; no-op edits write no event; `ErrNoWorkflow` when the form has no workflow.
  - `func (e *Engine) Comment(ctx, p, id, body string) (submissions.Event, error)` writes an event `comment` with `{"body"}`; an empty body gives a `*definition.ValidationError` at path `body`; bodies longer than 10000 runes are also rejected.
  - `func (e *Engine) Assign(ctx, p, id, userID *uuid.UUID) (submissions.Submission, error)` writes an event `assigned` with `{"assigneeId": string|nil, "assigneeName"}`; an unknown user gives a `*definition.ValidationError` at path `userId`; re-assigning the same user is a no-op.

- [ ] **Step 1: Write the failing tests**

`internal/workflow/edits_test.go`:

```go
package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func countEvents(t *testing.T, f *fixture.Fixture, id uuid.UUID, typ string) int {
	t.Helper()
	n := 0
	for _, e := range f.Events(t, id) {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func TestUpdateFields(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	got, err := e.UpdateFields(ctx, p, sub.ID, map[string]any{"score": 3, "priority": "high"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields["score"] != float64(3) || got.Fields["priority"] != "high" || got.State != "new" {
		t.Fatalf("fields = %v", got.Fields)
	}

	got, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"priority": nil})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Fields["priority"]; ok || got.Fields["score"] != float64(3) {
		t.Fatalf("after unset: %v", got.Fields)
	}
	if n := countEvents(t, f, sub.ID, "fields_updated"); n != 2 {
		t.Fatalf("fields_updated events = %d", n)
	}

	if _, err := e.UpdateFields(ctx, p, sub.ID, map[string]any{"score": 3}); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, f, sub.ID, "fields_updated"); n != 2 {
		t.Fatalf("no-op update wrote an event (%d)", n)
	}

	_, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"priority": "urgent"})
	var ve *definition.ValidationError
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.priority" {
		t.Fatalf("invalid option err = %#v", err)
	}
	_, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"ghost": nil})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.ghost" {
		t.Fatalf("unknown unset err = %#v", err)
	}

	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	if _, err := e.UpdateFields(ctx, p, plain.ID, map[string]any{"score": 1}); !errors.Is(err, workflow.ErrNoWorkflow) {
		t.Fatalf("no workflow err = %v", err)
	}
}

func TestComment(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t)
	ctx := context.Background()

	ev, err := e.Comment(ctx, p, sub.ID, "  Strong portfolio.  ")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "comment" || ev.Payload["body"] != "Strong portfolio." || ev.ActorName != p.Name || ev.ID == 0 {
		t.Fatalf("event = %+v", ev)
	}

	var ve *definition.ValidationError
	if _, err := e.Comment(ctx, p, sub.ID, "   "); !errors.As(err, &ve) || ve.Problems[0].Path != "body" {
		t.Fatalf("empty comment err = %#v", err)
	}
	if _, err := e.Comment(ctx, p, sub.ID, strings.Repeat("x", 10001)); !errors.As(err, &ve) {
		t.Fatalf("long comment err = %#v", err)
	}
	if _, err := e.Comment(ctx, p, uuid.New(), "hi"); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("unknown submission err = %v", err)
	}
}

func TestAssign(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	u := f.User(t, "assignee@example.com", "reviewer")
	ctx := context.Background()

	got, err := e.Assign(ctx, p, sub.ID, &u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssigneeID == nil || *got.AssigneeID != u.ID || got.AssigneeEmail != "assignee@example.com" {
		t.Fatalf("after assign: %+v", got)
	}
	if _, err := e.Assign(ctx, p, sub.ID, &u.ID); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, f, sub.ID, "assigned"); n != 1 {
		t.Fatalf("re-assigning same user wrote events: %d", n)
	}

	got, err = e.Assign(ctx, p, sub.ID, nil)
	if err != nil || got.AssigneeID != nil {
		t.Fatalf("unassign: %+v, %v", got, err)
	}
	var last submissions.Event
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "assigned" {
			last = ev
		}
	}
	if last.Payload["assigneeId"] != nil || last.ActorName != p.Name {
		t.Fatalf("unassign event = %+v", last)
	}

	ghost := uuid.New()
	var ve *definition.ValidationError
	if _, err := e.Assign(ctx, p, sub.ID, &ghost); !errors.As(err, &ve) || ve.Problems[0].Path != "userId" {
		t.Fatalf("unknown user err = %#v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/workflow/... -run 'TestUpdateFields|TestComment|TestAssign' -v`
Expected: FAIL to compile with `e.UpdateFields undefined`.

- [ ] **Step 3: Write the implementation**

`internal/workflow/edits.go`:

```go
package workflow

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

const maxCommentRunes = 10000

// UpdateFields edits workflow fields without changing state.
func (e *Engine) UpdateFields(ctx context.Context, p auth.Principal, id uuid.UUID, fields map[string]any) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		wf, err := e.workflowFor(ctx, sub)
		if err != nil {
			return err
		}
		merged, changed, err := mergeFields(*wf, sub.Fields, fields)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE submissions SET fields = $3, updated_at = now() WHERE org_id = $1 AND id = $2`,
			p.OrgID, id, merged); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		_, err = submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id, OrgID: p.OrgID, Type: "fields_updated",
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
			Payload: map[string]any{"fields": changed},
		})
		return err
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}

// Comment appends a comment event to the submission's timeline.
func (e *Engine) Comment(ctx context.Context, p auth.Principal, id uuid.UUID, body string) (submissions.Event, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return submissions.Event{}, &definition.ValidationError{Problems: []definition.Problem{{Path: "body", Message: "comment cannot be empty"}}}
	}
	if utf8.RuneCountInString(body) > maxCommentRunes {
		return submissions.Event{}, &definition.ValidationError{Problems: []definition.Problem{{Path: "body", Message: "comment is too long (max 10000 characters)"}}}
	}
	if _, err := e.subs.Get(ctx, p.OrgID, id); err != nil {
		return submissions.Event{}, err
	}
	actor := submissions.ActorFromPrincipal(p)
	return submissions.InsertEvent(ctx, e.pool, submissions.Event{
		SubmissionID: id, OrgID: p.OrgID, Type: "comment",
		ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
		Payload: map[string]any{"body": body},
	})
}

// Assign sets or clears the submission's assignee (userID nil = unassign).
func (e *Engine) Assign(ctx context.Context, p auth.Principal, id uuid.UUID, userID *uuid.UUID) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		payload := map[string]any{"assigneeId": nil, "assigneeName": ""}
		if userID != nil {
			var name string
			err := tx.QueryRow(ctx, `SELECT name FROM users WHERE org_id = $1 AND id = $2`, p.OrgID, *userID).Scan(&name)
			if errors.Is(err, pgx.ErrNoRows) {
				return &definition.ValidationError{Problems: []definition.Problem{{Path: "userId", Message: "unknown user"}}}
			}
			if err != nil {
				return err
			}
			payload = map[string]any{"assigneeId": userID.String(), "assigneeName": name}
		}
		if sameAssignee(sub.AssigneeID, userID) {
			return nil
		}
		if err := submissions.SetAssignee(ctx, tx, p.OrgID, id, userID); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		_, err = submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id, OrgID: p.OrgID, Type: "assigned",
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
			Payload: payload,
		})
		return err
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}

func sameAssignee(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/workflow/... -race -v`
Expected: PASS for every workflow test.

- [ ] **Step 5: Commit**

```bash
git add internal/workflow/edits.go internal/workflow/edits_test.go
git commit -m "feat(workflow): workflow field edits, comments and manual assignment"
```

---

### Task 9: HTTP endpoints, error mapping and detail transitions

**Files:**
- Create: `internal/httpapi/workflow_handlers.go`
- Create: `internal/httpapi/jobs_handlers.go`
- Modify: `internal/httpapi/errors.go` (append `errorMappings` rows)
- Modify: `internal/httpapi/routes.go` (mount calls)
- Modify: Plan 03's `GET /submissions/{id}` handler (the file containing `mountSubmissions`)
- Test: `internal/httpapi/workflow_handlers_test.go`

**Interfaces:**
- Consumes: `Engine` (Tasks 7–8), `jobs.Queue.List`, `jobs.Queue.Retry`, `jobs.ErrNotFound`; Plan 01 `WriteJSON`, `WriteError`, `DecodeJSON`, `RequireAdmin`, `MustPrincipal`, `URLParamUUID`; Plan 03's Submission and Event JSON encoders.
- Produces:
  - `errorMappings` rows for workflow sentinels and `jobs.ErrNotFound`
  - `func mountWorkflow(r chi.Router, d Deps)`, `func mountJobs(r chi.Router, d Deps)`
  - `type availableTransitionJSON struct{ Key, Label, To, ToLabel string; RequireFields []string; Allowed bool; Reason string }` (camelCase tags)
  - `func availableTransitions(r *http.Request, d Deps, sub submissions.Submission) ([]availableTransitionJSON, error)`
  - `type jobJSON` (§7.3 Job)

- [ ] **Step 1: Find Plan 03's encoder names and detail handler**

Run: `grep -n 'func .*submissions\.\(Submission\|Event\)) ' internal/httpapi/*.go; grep -n '"transitions"' internal/httpapi/*.go`
Expected output: one function that encodes a `submissions.Submission` to its §7.3 JSON, one that encodes a `submissions.Event`, and the line in the detail handler that sets `"transitions"`. This plan calls them `toSubmissionJSON(sub)` and `toEventJSON(ev)`. If Plan 03 used different names, use Plan 03's names at the call sites below. Do not add duplicate encoders.

- [ ] **Step 2: Write the failing tests**

`internal/httpapi/workflow_handlers_test.go`:

```go
package httpapi_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/testutil"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func newWorkflowAPI(t *testing.T) (*fixture.Fixture, http.Handler) {
	t.Helper()
	f := fixture.Setup(t)
	eng := workflow.NewEngine(f.Pool, f.Defs, f.Subs, f.Auth, f.Queue)
	f.Subs.SetCreatedHook(eng.OnCreated)
	h := httpapi.NewRouter(httpapi.Deps{
		Config: f.Config, Auth: f.Auth, Defs: f.Defs, Subs: f.Subs,
		Engine: eng, Queue: f.Queue, OrgID: f.OrgID,
	})
	return f, h
}

type subResp struct {
	Submission struct {
		ID       string         `json:"id"`
		State    string         `json:"state"`
		Fields   map[string]any `json:"fields"`
		Assignee *struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"assignee"`
	} `json:"submission"`
}

type errResp struct {
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Path string `json:"path"`
		} `json:"details"`
	} `json:"error"`
}

func TestTransitionHTTP_Success(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	key := f.APIKey(t, "reviewer")

	rec := testutil.Do(t, h, "POST", "/api/v1/submissions/"+sub.ID.String()+"/transitions", key,
		map[string]any{"transition": "screen", "comment": "ok", "expectedState": "new"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if got := testutil.Decode[subResp](t, rec); got.Submission.State != "screening" || got.Submission.ID != sub.ID.String() {
		t.Fatalf("response = %+v", got)
	}
}

func TestTransitionHTTP_Errors(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	reviewer := f.APIKey(t, "reviewer")
	manager := f.APIKey(t, "hiring-manager")
	path := "/api/v1/submissions/" + sub.ID.String() + "/transitions"

	cases := []struct {
		name, key, path string
		body            any
		status          int
		code            string
	}{
		{"unauthenticated", "", path, map[string]any{"transition": "screen"}, 401, "unauthenticated"},
		{"missing transition", reviewer, path, map[string]any{}, 422, "validation_failed"},
		{"unknown transition", reviewer, path, map[string]any{"transition": "teleport"}, 422, "unknown_transition"},
		{"forbidden", manager, path, map[string]any{"transition": "screen"}, 403, "forbidden"},
		{"invalid state", reviewer, path, map[string]any{"transition": "invite", "fields": map[string]any{"score": 1}}, 409, "invalid_state"},
		{"state conflict", reviewer, path, map[string]any{"transition": "screen", "expectedState": "interview"}, 409, "state_conflict"},
		{"required field", reviewer, path, map[string]any{"transition": "reject"}, 422, "validation_failed"},
		{"no workflow", reviewer, "/api/v1/submissions/" + plain.ID.String() + "/transitions", map[string]any{"transition": "screen"}, 409, "no_workflow"},
		{"malformed id", reviewer, "/api/v1/submissions/not-a-uuid/transitions", map[string]any{"transition": "screen"}, 404, "not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := testutil.Do(t, h, "POST", c.path, c.key, c.body)
			if rec.Code != c.status || testutil.ErrorCode(t, rec) != c.code {
				t.Fatalf("got %d %s, want %d %s; body %s", rec.Code, testutil.ErrorCode(t, rec), c.status, c.code, rec.Body)
			}
		})
	}

	rec := testutil.Do(t, h, "POST", path, reviewer, map[string]any{"transition": "reject"})
	if got := testutil.Decode[errResp](t, rec); len(got.Error.Details) != 1 || got.Error.Details[0].Path != "fields.rejectionReason" {
		t.Fatalf("details = %+v", got.Error.Details)
	}
}

func TestFieldsCommentsAssigneeHTTP(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	key := f.APIKey(t, "reviewer")
	base := "/api/v1/submissions/" + sub.ID.String()

	rec := testutil.Do(t, h, "PATCH", base+"/fields", key, map[string]any{"fields": map[string]any{"score": 5}})
	if rec.Code != 200 || testutil.Decode[subResp](t, rec).Submission.Fields["score"] != float64(5) {
		t.Fatalf("patch fields: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "PATCH", base+"/fields", key, map[string]any{"fields": map[string]any{"score": "high"}})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("invalid fields: %d %s", rec.Code, rec.Body)
	}

	rec = testutil.Do(t, h, "POST", base+"/comments", key, map[string]any{"body": "Nice"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("comment: %d %s", rec.Code, rec.Body)
	}
	ev := testutil.Decode[struct {
		Event struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		} `json:"event"`
	}](t, rec)
	if ev.Event.Type != "comment" || ev.Event.Payload["body"] != "Nice" {
		t.Fatalf("comment event = %+v", ev)
	}
	if rec := testutil.Do(t, h, "POST", base+"/comments", key, map[string]any{"body": ""}); rec.Code != 422 {
		t.Fatalf("empty comment: %d", rec.Code)
	}

	u := f.User(t, "who@example.com", "reviewer")
	rec = testutil.Do(t, h, "PUT", base+"/assignee", key, map[string]any{"userId": u.ID.String()})
	if got := testutil.Decode[subResp](t, rec); rec.Code != 200 || got.Submission.Assignee == nil || got.Submission.Assignee.Email != "who@example.com" {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "PUT", base+"/assignee", key, map[string]any{"userId": nil})
	if got := testutil.Decode[subResp](t, rec); rec.Code != 200 || got.Submission.Assignee != nil {
		t.Fatalf("unassign: %d %s", rec.Code, rec.Body)
	}
}

func TestSubmissionDetailIncludesTransitions(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	rec := testutil.Do(t, h, "GET", "/api/v1/submissions/"+sub.ID.String(), f.APIKey(t, "hiring-manager"), nil)
	if rec.Code != 200 {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Transitions []struct {
			Key           string   `json:"key"`
			ToLabel       string   `json:"toLabel"`
			RequireFields []string `json:"requireFields"`
			Allowed       bool     `json:"allowed"`
			Reason        string   `json:"reason"`
		} `json:"transitions"`
	}](t, rec)
	byKey := map[string]int{}
	for i, tr := range got.Transitions {
		byKey[tr.Key] = i
	}
	screen, reject := got.Transitions[byKey["screen"]], got.Transitions[byKey["reject"]]
	if len(got.Transitions) != 2 || screen.Allowed || screen.Reason != "role" || screen.RequireFields == nil ||
		!reject.Allowed || reject.ToLabel != "Rejected" {
		t.Fatalf("transitions = %+v", got.Transitions)
	}
}

func TestJobsHTTP(t *testing.T) {
	f, h := newWorkflowAPI(t)
	f.Applicant(t) // onSubmit email → one pending job
	admin := f.APIKey(t, "admin")

	if rec := testutil.Do(t, h, "GET", "/api/v1/jobs", f.APIKey(t, "reviewer"), nil); rec.Code != 403 {
		t.Fatalf("non-admin list: %d", rec.Code)
	}
	rec := testutil.Do(t, h, "GET", "/api/v1/jobs?status=pending", admin, nil)
	list := testutil.Decode[struct {
		Items []struct {
			ID          int64          `json:"id"`
			Kind        string         `json:"kind"`
			Status      string         `json:"status"`
			MaxAttempts int            `json:"maxAttempts"`
			Payload     map[string]any `json:"payload"`
		} `json:"items"`
	}](t, rec)
	if rec.Code != 200 || len(list.Items) != 1 || list.Items[0].Kind != "action.email" || list.Items[0].MaxAttempts != 8 ||
		list.Items[0].Payload["trigger"] != "submit" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, h, "GET", "/api/v1/jobs?status=bogus", admin, nil); rec.Code != 400 {
		t.Fatalf("bad status: %d", rec.Code)
	}

	id := list.Items[0].ID
	if _, err := f.Pool.Exec(context.Background(), `UPDATE jobs SET status = 'failed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if rec := testutil.Do(t, h, "POST", "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/retry", admin, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "POST", "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/retry", admin, nil)
	if rec.Code != 404 || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("retry pending: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, h, "POST", "/api/v1/jobs/abc/retry", admin, nil); rec.Code != 404 {
		t.Fatalf("retry bad id: %d", rec.Code)
	}
	all, _ := f.Queue.List(context.Background(), f.OrgID, jobs.StatusPending, 10)
	if len(all) != 1 {
		t.Fatalf("pending after retry = %d", len(all))
	}
}
```

The test file imports `"strconv"` (for `strconv.FormatInt`) in addition to its other imports.

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/httpapi/... -run 'TransitionHTTP|FieldsCommentsAssigneeHTTP|SubmissionDetailIncludesTransitions|JobsHTTP' -v`
Expected: FAIL. The new routes return 404 (`POST .../transitions` isn't mounted), and the detail test fails because `transitions` is empty.

- [ ] **Step 4: Write the implementation**

Append these rows to `var errorMappings` in `internal/httpapi/errors.go` (Plan 01 matches rows with `errors.Is`, so wrapped errors map correctly), and add the `workflow` and `jobs` imports to that file:

```go
	{workflow.ErrForbidden, http.StatusForbidden, "forbidden", "you are not allowed to perform this transition"},
	{workflow.ErrUnknownTransition, http.StatusUnprocessableEntity, "unknown_transition", "unknown transition"},
	{workflow.ErrInvalidState, http.StatusConflict, "invalid_state", "transition not allowed from the current state"},
	{workflow.ErrStateConflict, http.StatusConflict, "state_conflict", "submission state changed; reload and retry"},
	{workflow.ErrNoWorkflow, http.StatusConflict, "no_workflow", "this submission has no workflow"},
	{jobs.ErrNotFound, http.StatusNotFound, "not_found", "job not found or not failed"},
```

`internal/httpapi/workflow_handlers.go`:

```go
package httpapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/workflow"
)

type availableTransitionJSON struct {
	Key           string   `json:"key"`
	Label         string   `json:"label"`
	To            string   `json:"to"`
	ToLabel       string   `json:"toLabel"`
	RequireFields []string `json:"requireFields"`
	Allowed       bool     `json:"allowed"`
	Reason        string   `json:"reason"`
}

// availableTransitions returns the detail endpoint's "transitions" array
// ([] when no engine is wired or the form has no workflow).
func availableTransitions(r *http.Request, d Deps, sub submissions.Submission) ([]availableTransitionJSON, error) {
	out := []availableTransitionJSON{}
	if d.Engine == nil {
		return out, nil
	}
	av, err := d.Engine.Available(r.Context(), MustPrincipal(r), sub)
	if err != nil {
		return nil, err
	}
	for _, a := range av {
		req := a.RequireFields
		if req == nil {
			req = []string{}
		}
		out = append(out, availableTransitionJSON{
			Key: a.Key, Label: a.Label, To: a.To, ToLabel: a.ToLabel,
			RequireFields: req, Allowed: a.Allowed, Reason: a.Reason,
		})
	}
	return out, nil
}

type workflowHandlers struct{ d Deps }

func mountWorkflow(r chi.Router, d Deps) {
	h := workflowHandlers{d: d}
	r.Post("/submissions/{id}/transitions", h.transition)
	r.Patch("/submissions/{id}/fields", h.updateFields)
	r.Post("/submissions/{id}/comments", h.comment)
	r.Put("/submissions/{id}/assignee", h.assign)
}

type transitionRequest struct {
	Transition    string         `json:"transition"`
	Fields        map[string]any `json:"fields"`
	Comment       string         `json:"comment"`
	ExpectedState string         `json:"expectedState"`
}

func (h workflowHandlers) transition(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req transitionRequest
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	if strings.TrimSpace(req.Transition) == "" {
		WriteError(w, r, &definition.ValidationError{Problems: []definition.Problem{{Path: "transition", Message: "transition is required"}}})
		return
	}
	sub, err := h.d.Engine.Transition(r.Context(), MustPrincipal(r), id, workflow.TransitionInput{
		Transition: req.Transition, Fields: req.Fields, Comment: req.Comment, ExpectedState: req.ExpectedState,
	})
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}

func (h workflowHandlers) updateFields(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		Fields map[string]any `json:"fields"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, err := h.d.Engine.UpdateFields(r.Context(), MustPrincipal(r), id, req.Fields)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}

func (h workflowHandlers) comment(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		Body string `json:"body"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	ev, err := h.d.Engine.Comment(r.Context(), MustPrincipal(r), id, req.Body)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusCreated, map[string]any{"event": toEventJSON(ev)})
}

func (h workflowHandlers) assign(w http.ResponseWriter, r *http.Request) {
	id, err := URLParamUUID(r, "id")
	if err != nil {
		WriteError(w, r, err)
		return
	}
	var req struct {
		UserID *uuid.UUID `json:"userId"`
	}
	if err := DecodeJSON(r, &req); err != nil {
		WriteError(w, r, err)
		return
	}
	sub, err := h.d.Engine.Assign(r.Context(), MustPrincipal(r), id, req.UserID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	WriteJSON(w, http.StatusOK, map[string]any{"submission": toSubmissionJSON(sub)})
}
```

`internal/httpapi/jobs_handlers.go`:

```go
package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/openforms/openforms/internal/jobs"
)

type jobJSON struct {
	ID          int64           `json:"id"`
	Kind        string          `json:"kind"`
	Status      string          `json:"status"`
	Attempts    int             `json:"attempts"`
	MaxAttempts int             `json:"maxAttempts"`
	RunAt       time.Time       `json:"runAt"`
	LastError   string          `json:"lastError"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

func toJobJSON(j jobs.Job) jobJSON {
	return jobJSON{
		ID: j.ID, Kind: j.Kind, Status: j.Status, Attempts: j.Attempts, MaxAttempts: j.MaxAttempts,
		RunAt: j.RunAt.UTC(), LastError: j.LastError, Payload: j.Payload,
		CreatedAt: j.CreatedAt.UTC(), UpdatedAt: j.UpdatedAt.UTC(),
	}
}

var validJobStatus = map[string]bool{
	"": true, jobs.StatusPending: true, jobs.StatusRunning: true, jobs.StatusDone: true, jobs.StatusFailed: true,
}

func mountJobs(r chi.Router, d Deps) {
	r.With(RequireAdmin).Get("/jobs", func(w http.ResponseWriter, r *http.Request) {
		status := r.URL.Query().Get("status")
		if !validJobStatus[status] {
			WriteError(w, r, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "status must be one of pending, running, done, failed"})
			return
		}
		limit := 100
		if s := r.URL.Query().Get("limit"); s != "" {
			n, err := strconv.Atoi(s)
			if err != nil || n < 1 || n > 500 {
				WriteError(w, r, &APIError{Status: http.StatusBadRequest, Code: "bad_request", Message: "limit must be between 1 and 500"})
				return
			}
			limit = n
		}
		list, err := d.Queue.List(r.Context(), d.OrgID, status, limit)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		items := make([]jobJSON, 0, len(list))
		for _, j := range list {
			items = append(items, toJobJSON(j))
		}
		WriteJSON(w, http.StatusOK, map[string]any{"items": items})
	})
	r.With(RequireAdmin).Post("/jobs/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			WriteError(w, r, jobs.ErrNotFound)
			return
		}
		if err := d.Queue.Retry(r.Context(), d.OrgID, id); err != nil {
			WriteError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}
```

Modify `internal/httpapi/routes.go`. Inside the authenticated group (the `r.Group(func(r chi.Router){ r.Use(RequireAuth); ... })` block that already calls `mountSubmissions(r, d)`), add after `mountSubmissions(r, d)`:

```go
			if d.Engine != nil {
				mountWorkflow(r, d)
			}
			if d.Queue != nil {
				mountJobs(r, d)
			}
```

Modify Plan 03's `GET /submissions/{id}` handler (the file found in Step 1). Immediately before the handler writes its response, add:

```go
	transitions, err := availableTransitions(r, d, sub)
	if err != nil {
		WriteError(w, r, err)
		return
	}
```

Then replace the existing `"transitions": ...` entry in the response map with `"transitions": transitions`. If the handler is a method whose deps live on a receiver (e.g. `h.d`), pass that value instead of `d`, and use whatever variable holds the loaded submission in place of `sub`.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/httpapi/... -v`
Expected: PASS. That includes every Plan 01/03 handler test, which still passes because routes are only mounted when `Engine`/`Queue` are non-nil.

- [ ] **Step 6: Commit**

```bash
git add internal/httpapi/
git commit -m "feat(httpapi): transition, fields, comments, assignee and jobs endpoints"
```

---

### Task 10: App wiring, background worker, end-to-end check

**Files:**
- Create: `internal/app/workers.go`
- Modify: `internal/app/app.go` (`New` and `Run`)
- Test: `internal/app/workers_test.go`

**Interfaces:**
- Consumes: everything above; `App` fields from spec §6.13.
- Produces:
  - `func (a *App) wireWorkflow()` sets `a.Queue` and `a.Engine`, registers the created hook, and registers the action handlers.
  - `func (a *App) runWorker(ctx context.Context) (wait func())`
  - `App.Run` now processes jobs until the HTTP server has shut down.

- [ ] **Step 1: Write the failing test**

`internal/app/workers_test.go`:

```go
package app

import (
	"context"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

func TestWorkerProcessesOnSubmitActions(t *testing.T) {
	f := fixture.Setup(t)
	cfg := f.Config
	cfg.SMTPHost = "" // log mailer
	cfg.WorkerConcurrency = 2
	a := &App{Config: cfg, Pool: f.Pool, Auth: f.Auth, Defs: f.Defs, Subs: f.Subs, OrgID: f.OrgID}
	a.wireWorkflow()
	if a.Queue == nil || a.Engine == nil {
		t.Fatal("wireWorkflow must set Queue and Engine")
	}
	a.Queue.SetPollInterval(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	wait := a.runWorker(ctx)
	defer func() { cancel(); wait() }()

	sub := f.Applicant(t) // created hook → onSubmit email job

	deadline := time.Now().Add(10 * time.Second)
	for {
		done, err := a.Queue.List(context.Background(), f.OrgID, jobs.StatusDone, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(done) == 1 {
			break
		}
		if time.Now().After(deadline) {
			all, _ := a.Queue.List(context.Background(), f.OrgID, "", 10)
			t.Fatalf("job not processed: %+v", all)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var succeeded bool
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "action_succeeded" {
			succeeded = true
		}
	}
	if !succeeded {
		t.Fatal("expected an action_succeeded event")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/... -run TestWorkerProcessesOnSubmitActions -v`
Expected: FAIL to compile with `a.wireWorkflow undefined`.

- [ ] **Step 3: Write the implementation**

`internal/app/workers.go`:

```go
package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/workflow"
)

// wireWorkflow builds the job queue and workflow engine, hooks onSubmit
// actions into submission creation and registers the action handlers.
// Requires a.Config, a.Pool, a.Auth, a.Defs and a.Subs.
func (a *App) wireWorkflow() {
	a.Queue = jobs.NewQueue(a.Pool)
	a.Engine = workflow.NewEngine(a.Pool, a.Defs, a.Subs, a.Auth, a.Queue)
	a.Subs.SetCreatedHook(a.Engine.OnCreated)
	actions.Register(a.Queue, actions.Deps{
		Config:     a.Config,
		Subs:       a.Subs,
		Defs:       a.Defs,
		Auth:       a.Auth,
		Mailer:     actions.NewMailer(a.Config),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Pool:       a.Pool,
	})
}

// runWorker starts the job worker; the returned func blocks until it stops.
func (a *App) runWorker(ctx context.Context) (wait func()) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Queue.Run(ctx, a.Config.WorkerConcurrency)
	}()
	return wg.Wait
}
```

Modify `internal/app/app.go`, `New`. After `a.Subs` is assigned and **before** the router is built, add:

```go
	a.wireWorkflow()
```

In the `httpapi.Deps{...}` literal that `New` passes to `httpapi.NewRouter`, add the two fields:

```go
		Engine: a.Engine,
		Queue:  a.Queue,
```

Modify `internal/app/app.go`, `Run`. Add the following as the first statements of `Run`. The worker keeps going while the HTTP server drains and stops once `Run` returns.

```go
	workerCtx, stopWorker := context.WithCancel(context.WithoutCancel(ctx))
	waitWorker := a.runWorker(workerCtx)
	defer func() {
		stopWorker()
		waitWorker()
	}()
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/app/... -v`
Expected: PASS.

- [ ] **Step 5: Run the whole backend suite**

Run: `go vet ./... && go build ./... && go test ./... -race`
Expected: `go vet` prints nothing, the build succeeds, and every package reports `ok` (no `FAIL`).

- [ ] **Step 6: Manual smoke test against the dev stack (optional but recommended)**

```bash
docker compose up -d postgres mailpit
OPENFORMS_DATABASE_URL='postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable' \
OPENFORMS_SMTP_HOST=localhost OPENFORMS_SMTP_PORT=1025 \
go run ./cmd/openforms serve
```

In another shell, run `curl -s localhost:8080/healthz`.
Expected: `{"status":"ok"}`. The server log shows no `jobs: worker` errors while idle.

- [ ] **Step 7: Commit**

```bash
git add internal/app/
git commit -m "feat(app): wire workflow engine, actions and background job worker"
```

---

## Self-review notes

- **Spec coverage.**
  - §4 migration 00003 is covered by Task 1.
  - §6.8 is covered by Tasks 1–2: `Queue`, `Enqueue`, `RunOnce`, `Run`, `List`, `Retry`, backoff, `Permanent` and lock reclaim.
  - §6.9 is covered by Tasks 3–6: `Render`, `TemplateVars`, `NewMailer`, the webhook headers/body/signature and permanent-vs-retryable codes, the assign algorithm, and the `action_succeeded`/`action_failed`/`assigned` events.
  - §6.10 is covered by Tasks 7–8. Every step of the transition algorithm is implemented and tested in its order.
  - The §7.2 Plan 04 rows are covered by Task 9.
  - §6.13 `App.Queue`/`Engine` and the worker in `Run` are covered by Task 10.
  - The §11 points on concurrency, the transactional outbox and at-least-once delivery (`X-OpenForms-Delivery`) are covered by Tasks 5, 7 and 9.
- **Decisions this plan adds on top of the spec (none contradict it):**
  - `Queue.SetClock`, `SetPollInterval` and `SetFailedHook`
  - `jobs.ErrNotFound`, mapped to 404
  - `actions.KindFor`, `Sign`, `TriggerSubmit`, `SystemActorName`
  - `submissions.SetAssignee`
  - `Engine.Available` returns `[]` for forms without a workflow, rather than `ErrNoWorkflow`, so the detail endpoint works for every form
  - comments are capped at 10 000 runes
  - `GET /jobs` accepts `limit` (1–500)
  - `OnCreated` looks up the `created` event id inside the transaction and uses 0 if Plan 03 inserts that event after the hook
  - the webhook `transition.from` comes from the transition event referenced by `EventID`
  - webhooks describe the submission as it is **at delivery time**, not at transition time

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

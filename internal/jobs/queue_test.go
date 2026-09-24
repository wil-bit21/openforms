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

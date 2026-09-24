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

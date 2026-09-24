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

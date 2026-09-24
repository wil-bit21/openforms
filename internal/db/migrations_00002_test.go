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

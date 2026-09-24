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

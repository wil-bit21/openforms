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

// Package testutil provides DB-backed fixtures and HTTP helpers for handler tests.
package testutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db/dbtest"
)

type Env struct {
	Pool   *pgxpool.Pool
	Auth   *auth.Service
	OrgID  uuid.UUID
	Config config.Config
}

// NewEnv returns an isolated, migrated schema with the default org created.
func NewEnv(t testing.TB) *Env {
	t.Helper()
	pool := dbtest.New(t)
	a := auth.NewService(pool)
	org, err := a.EnsureDefaultOrg(context.Background())
	if err != nil {
		t.Fatalf("ensure default org: %v", err)
	}
	return &Env{
		Pool:  pool,
		Auth:  a,
		OrgID: org,
		Config: config.Config{
			HTTPAddr:          "127.0.0.1:0",
			BaseURL:           "http://test.local",
			SMTPPort:          1025,
			SMTPFrom:          "openforms@test.local",
			WorkerConcurrency: 1,
		},
	}
}

var keySeq atomic.Int64

// APIKey creates an API key with the given roles and returns its plaintext.
func (e *Env) APIKey(t testing.TB, roles ...string) string {
	t.Helper()
	plaintext, _, err := e.Auth.CreateAPIKey(context.Background(), e.OrgID, fmt.Sprintf("test-key-%d", keySeq.Add(1)), roles)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	return plaintext
}

// User creates a user with password "password123".
func (e *Env) User(t testing.TB, email string, roles ...string) auth.User {
	t.Helper()
	u, err := e.Auth.CreateUser(context.Background(), e.OrgID, email, email, "password123", roles)
	if err != nil {
		t.Fatalf("create user %s: %v", email, err)
	}
	return u
}

// Do sends a request. body may be nil, a string/[]byte (sent raw) or any value (JSON-encoded).
// apiKey "" sends the request anonymously.
func Do(t testing.TB, h http.Handler, method, path, apiKey string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rdr = strings.NewReader(b)
	case []byte:
		rdr = bytes.NewReader(b)
	default:
		data, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		rdr = bytes.NewReader(data)
	}
	req := httptest.NewRequest(method, path, rdr)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// Decode unmarshals the recorder body into T or fails the test.
func Decode[T any](t testing.TB, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

// ErrorCode returns error.code from the envelope ("" if absent).
func ErrorCode(t testing.TB, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return env.Error.Code
}

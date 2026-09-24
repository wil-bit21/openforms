package app_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/app"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db/dbtest"
)

func testConfig(t *testing.T) config.Config {
	return config.Config{
		HTTPAddr: "127.0.0.1:0", DatabaseURL: dbtest.NewURL(t),
		BaseURL: "http://test.local", SMTPPort: 1025, WorkerConcurrency: 1,
	}
}

func TestNewServesHealthzAndAPI(t *testing.T) {
	a, err := app.New(context.Background(), testConfig(t))
	if err != nil {
		t.Fatalf("app.New: %v", err)
	}
	defer a.Close()
	if a.OrgID == uuid.Nil {
		t.Fatal("OrgID not set")
	}

	rec := httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "ok") {
		t.Fatalf("healthz: %d %s", rec.Code, rec.Body.String())
	}
	// Uses an unknown API path (not /auth/me) so this test does not depend on Task 9, which runs in parallel.
	rec = httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))
	if rec.Code != 404 || !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("unknown api path: %d %s", rec.Code, rec.Body.String())
	}
	// The admin SPA build lands in internal/webui/dist in a later plan; until then the web
	// handler answers 503. Accept either so this test keeps passing once that build exists.
	rec = httptest.NewRecorder()
	a.Handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin", nil))
	switch rec.Code {
	case http.StatusOK:
		if !strings.Contains(rec.Body.String(), `<div id="root">`) {
			t.Fatalf("admin 200 body missing root div: %s", rec.Body.String())
		}
	case http.StatusServiceUnavailable:
		if !strings.Contains(rec.Body.String(), "web UI not built") {
			t.Fatalf("admin 503 body missing message: %s", rec.Body.String())
		}
	default:
		t.Fatalf("admin: %d, want 200 or 503", rec.Code)
	}
}

func TestNewIsRestartSafe(t *testing.T) {
	cfg := testConfig(t)
	a1, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	org := a1.OrgID
	a1.Close()
	a2, err := app.New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("second New: %v", err)
	}
	defer a2.Close()
	if a2.OrgID != org {
		t.Fatalf("org changed across restarts: %v -> %v", org, a2.OrgID)
	}
}

func TestNewRequiresDatabaseURL(t *testing.T) {
	if _, err := app.New(context.Background(), config.Config{}); err == nil || !strings.Contains(err.Error(), "OPENFORMS_DATABASE_URL") {
		t.Fatalf("err = %v", err)
	}
}

func TestRunStopsOnContextCancel(t *testing.T) {
	a, err := app.New(context.Background(), testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Run(ctx) }()
	time.Sleep(200 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

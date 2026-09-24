package httpapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/testutil"
)

func decodeEnvelope(rec *httptest.ResponseRecorder) envelope {
	var env envelope
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return env
}

func newRouter(t *testing.T) (*testutil.Env, http.Handler) {
	t.Helper()
	env := testutil.NewEnv(t)
	web := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(299) // sentinel: request reached the web UI handler
	})
	return env, httpapi.NewRouter(httpapi.Deps{
		Config: env.Config, Pool: env.Pool, Auth: env.Auth, Web: web, OrgID: env.OrgID,
	})
}

func TestHealthz(t *testing.T) {
	_, h := newRouter(t)
	rec := testutil.Do(t, h, http.MethodGet, "/healthz", "", nil)
	if rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
	if got := testutil.Decode[map[string]string](t, rec); got["status"] != "ok" {
		t.Fatalf("body = %v", got)
	}
}

func TestUnknownAPIPathIsJSON404(t *testing.T) {
	_, h := newRouter(t)
	rec := testutil.Do(t, h, http.MethodGet, "/api/v1/does-not-exist", "", nil)
	if rec.Code != 404 || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestNonAPIPathsReachWebHandler(t *testing.T) {
	_, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodGet, "/admin/submissions", "", nil); rec.Code != 299 {
		t.Fatalf("code = %d, want web handler sentinel 299", rec.Code)
	}
}

func TestInvalidBearerDoesNotBreakPublicRoutes(t *testing.T) {
	_, h := newRouter(t)
	if rec := testutil.Do(t, h, http.MethodGet, "/healthz", "ofk_bogus", nil); rec.Code != 200 {
		t.Fatalf("code = %d", rec.Code)
	}
}

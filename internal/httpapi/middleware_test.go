package httpapi_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/httpapi"
)

var okHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	httpapi.WriteJSON(w, http.StatusOK, map[string]string{"name": httpapi.MustPrincipal(r).Name})
})

func serveWith(mw func(http.Handler) http.Handler, p *auth.Principal) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if p != nil {
		req = req.WithContext(auth.WithPrincipal(req.Context(), *p))
	}
	rec := httptest.NewRecorder()
	mw(okHandler).ServeHTTP(rec, req)
	return rec
}

func TestRequireAuth(t *testing.T) {
	rec, env := func() (*httptest.ResponseRecorder, envelope) {
		r := serveWith(httpapi.RequireAuth, nil)
		return r, decodeEnvelope(r)
	}()
	if rec.Code != 401 || env.Error.Code != "unauthenticated" {
		t.Fatalf("anonymous: %d %q", rec.Code, env.Error.Code)
	}
	if rec := serveWith(httpapi.RequireAuth, &auth.Principal{Name: "Ada"}); rec.Code != 200 {
		t.Fatalf("authenticated: %d", rec.Code)
	}
}

func TestRequireAdmin(t *testing.T) {
	if rec := serveWith(httpapi.RequireAdmin, nil); rec.Code != 401 {
		t.Fatalf("anonymous: %d", rec.Code)
	}
	rec := serveWith(httpapi.RequireAdmin, &auth.Principal{Roles: []string{"reviewer"}})
	if rec.Code != 403 || decodeEnvelope(rec).Error.Code != "forbidden" {
		t.Fatalf("reviewer: %d %s", rec.Code, rec.Body.String())
	}
	if rec := serveWith(httpapi.RequireAdmin, &auth.Principal{Roles: []string{"admin"}}); rec.Code != 200 {
		t.Fatalf("admin: %d", rec.Code)
	}
}

func TestURLParamUUID(t *testing.T) {
	r := chi.NewRouter()
	r.Get("/things/{id}", func(w http.ResponseWriter, req *http.Request) {
		id, err := httpapi.URLParamUUID(req, "id")
		if err != nil {
			httpapi.WriteError(w, req, err)
			return
		}
		httpapi.WriteJSON(w, 200, map[string]string{"id": id.String()})
	})
	good := uuid.New()
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things/"+good.String(), nil))
	if rec.Code != 200 {
		t.Fatalf("valid uuid: %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/things/not-a-uuid", nil))
	if rec.Code != 404 || decodeEnvelope(rec).Error.Code != "not_found" {
		t.Fatalf("invalid uuid: %d %s", rec.Code, rec.Body.String())
	}
}

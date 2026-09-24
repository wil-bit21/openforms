package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
)

// Deps are the services handlers use. Later plans add Defs, Subs, Engine and Queue (spec §6.11).
type Deps struct {
	Config config.Config
	Pool   *pgxpool.Pool
	Auth   *auth.Service
	Web    http.Handler // webui handler; may be nil
	OrgID  uuid.UUID
}

// NewRouter builds the full HTTP handler: /healthz, /api/v1/*, and the web UI fallback.
func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(chimw.Recoverer)
	if d.Web != nil {
		r.NotFound(d.Web.ServeHTTP)
		r.MethodNotAllowed(d.Web.ServeHTTP)
	}
	r.Get("/healthz", healthz(d))

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(Authenticate(d.Auth))
		r.NotFound(func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, req, notFound("no such endpoint"))
		})
		r.MethodNotAllowed(func(w http.ResponseWriter, req *http.Request) {
			WriteError(w, req, &APIError{Status: http.StatusMethodNotAllowed, Code: "bad_request", Message: "method not allowed"})
		})
		// Plan 01 Tasks 9–10 add: mountAuth(r, d); r.Group(RequireAuth: mountAdminUsers, mountAPIKeys).
	})
	return r
}

func healthz(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if d.Pool != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
			defer cancel()
			if err := d.Pool.Ping(ctx); err != nil {
				WriteJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
				return
			}
		}
		WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

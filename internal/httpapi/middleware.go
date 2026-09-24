package httpapi

import (
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

const sessionCookie = "of_session"

// Authenticate resolves a Bearer API key or the of_session cookie into a principal.
// Invalid credentials leave the request anonymous (RequireAuth rejects later);
// infrastructure errors (e.g. DB down) are returned as 500.
func Authenticate(a *auth.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p, err := principalFromRequest(r, a)
			switch {
			case err == nil:
				r = r.WithContext(auth.WithPrincipal(r.Context(), p))
			case errors.Is(err, auth.ErrUnauthenticated):
			default:
				WriteError(w, r, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func principalFromRequest(r *http.Request, a *auth.Service) (auth.Principal, error) {
	if h := r.Header.Get("Authorization"); h != "" {
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return auth.Principal{}, auth.ErrUnauthenticated
		}
		return a.PrincipalFromAPIKey(r.Context(), strings.TrimSpace(tok))
	}
	if c, err := r.Cookie(sessionCookie); err == nil && c.Value != "" {
		return a.PrincipalFromSession(r.Context(), c.Value)
	}
	return auth.Principal{}, auth.ErrUnauthenticated
}

// RequireAuth rejects requests without a principal with 401 unauthenticated.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := auth.PrincipalFrom(r.Context()); !ok {
			WriteError(w, r, auth.ErrUnauthenticated)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin rejects anonymous requests with 401 and non-admins with 403.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p, ok := auth.PrincipalFrom(r.Context())
		if !ok {
			WriteError(w, r, auth.ErrUnauthenticated)
			return
		}
		if !p.IsAdmin() {
			WriteError(w, r, forbidden("admin role required"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// MustPrincipal returns the request principal; only call it behind RequireAuth.
func MustPrincipal(r *http.Request) auth.Principal {
	p, ok := auth.PrincipalFrom(r.Context())
	if !ok {
		panic("httpapi: MustPrincipal called without RequireAuth")
	}
	return p
}

// URLParamUUID parses a chi URL parameter; malformed values are 404 not_found.
func URLParamUUID(r *http.Request, name string) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, name))
	if err != nil {
		return uuid.Nil, notFound("not found")
	}
	return id, nil
}

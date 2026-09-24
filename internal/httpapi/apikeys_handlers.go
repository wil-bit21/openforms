package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

type apiKeyJSON struct {
	ID         uuid.UUID  `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	Roles      []string   `json:"roles"`
	CreatedAt  time.Time  `json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}

func toAPIKeyJSON(k auth.APIKey) apiKeyJSON {
	return apiKeyJSON{
		ID: k.ID, Name: k.Name, Prefix: k.Prefix, Roles: nonNil(k.Roles),
		CreatedAt: k.CreatedAt.UTC(), LastUsedAt: utcPtr(k.LastUsedAt), RevokedAt: utcPtr(k.RevokedAt),
	}
}

func mountAPIKeys(r chi.Router, d Deps) {
	r.Route("/api-keys", func(r chi.Router) {
		r.Use(RequireAdmin)

		r.Get("/", func(w http.ResponseWriter, req *http.Request) {
			keys, err := d.Auth.ListAPIKeys(req.Context(), MustPrincipal(req).OrgID)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			items := make([]apiKeyJSON, len(keys))
			for i, k := range keys {
				items[i] = toAPIKeyJSON(k)
			}
			WriteJSON(w, http.StatusOK, map[string]any{"items": items})
		})

		r.Post("/", func(w http.ResponseWriter, req *http.Request) {
			var body struct {
				Name  string   `json:"name"`
				Roles []string `json:"roles"`
			}
			if err := DecodeJSON(req, &body); err != nil {
				WriteError(w, req, err)
				return
			}
			plaintext, k, err := d.Auth.CreateAPIKey(req.Context(), MustPrincipal(req).OrgID, body.Name, body.Roles)
			if err != nil {
				WriteError(w, req, err)
				return
			}
			WriteJSON(w, http.StatusCreated, map[string]any{"apiKey": toAPIKeyJSON(k), "key": plaintext})
		})

		r.Delete("/{id}", func(w http.ResponseWriter, req *http.Request) {
			id, err := URLParamUUID(req, "id")
			if err != nil {
				WriteError(w, req, err)
				return
			}
			if err := d.Auth.RevokeAPIKey(req.Context(), MustPrincipal(req).OrgID, id); err != nil {
				WriteError(w, req, err)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	})
}

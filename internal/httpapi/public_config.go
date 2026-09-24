package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type publicConfigResponse struct {
	Demo bool `json:"demo"`
}

// mountPublicConfig exposes non-sensitive server settings to anonymous clients
// (the admin login page and the /demo page use it to detect demo mode).
func mountPublicConfig(r chi.Router, d Deps) {
	r.Get("/public/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		WriteJSON(w, http.StatusOK, publicConfigResponse{Demo: d.Config.DemoMode})
	})
}

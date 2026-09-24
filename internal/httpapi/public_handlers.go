package httpapi

import "github.com/go-chi/chi/v5"

// mountPublic registers the unauthenticated /public routes.
func mountPublic(r chi.Router, d Deps) {}

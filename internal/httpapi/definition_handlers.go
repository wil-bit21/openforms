package httpapi

import "github.com/go-chi/chi/v5"

// mountDefinitions registers /forms, /workflows and /definitions routes (authenticated group).
func mountDefinitions(r chi.Router, d Deps) {}

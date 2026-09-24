package httpapi

import "github.com/go-chi/chi/v5"

// mountSubmissions registers /submissions routes and the CSV export (authenticated group).
func mountSubmissions(r chi.Router, d Deps) {}

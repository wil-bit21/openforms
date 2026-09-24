// Package definition holds the form/workflow definition language (Plan 02).
// This file (Plan 01) provides the shared validation problem types.
package definition

import "strings"

// Problem is one validation failure; Path is JSON-pointer-ish, e.g. "fields[2].showIf.field".
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError aggregates problems; the HTTP layer maps it to 422 validation_failed.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		parts[i] = p.Path + ": " + p.Message
	}
	return "validation failed: " + strings.Join(parts, "; ")
}

// Package httpapi is the /api/v1 HTTP layer: router, middleware, handlers and the error envelope.
package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/workflow"
)

// APIError is an error with an explicit HTTP status and envelope code.
type APIError struct {
	Status  int
	Code    string
	Message string
	Details []definition.Problem
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

// errorMapping maps a sentinel error (matched with errors.Is) to a response.
// Later plans register their packages' sentinel errors by appending rows here
// (e.g. {definitions.ErrNotFound, 404, "not_found", "not found"}).
type errorMapping struct {
	target  error
	status  int
	code    string
	message string
}

var errorMappings = []errorMapping{
	{auth.ErrUnauthenticated, http.StatusUnauthorized, "unauthenticated", "authentication required"},
	{auth.ErrInvalidCredentials, http.StatusUnauthorized, "invalid_credentials", "invalid email or password"},
	{auth.ErrEmailTaken, http.StatusConflict, "email_taken", "a user with this email already exists"},
	{auth.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{definitions.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrNotFound, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrFormNotPublic, http.StatusNotFound, "not_found", "not found"},
	{submissions.ErrInvalidCursor, http.StatusBadRequest, "bad_request", "invalid cursor"},
	{workflow.ErrForbidden, http.StatusForbidden, "forbidden", "you are not allowed to perform this transition"},
	{workflow.ErrUnknownTransition, http.StatusUnprocessableEntity, "unknown_transition", "unknown transition"},
	{workflow.ErrInvalidState, http.StatusConflict, "invalid_state", "transition not allowed from the current state"},
	{workflow.ErrStateConflict, http.StatusConflict, "state_conflict", "submission state changed; reload and retry"},
	{workflow.ErrNoWorkflow, http.StatusConflict, "no_workflow", "this submission has no workflow"},
	{jobs.ErrNotFound, http.StatusNotFound, "not_found", "job not found or not failed"},
}

func notFound(msg string) *APIError {
	return &APIError{Status: http.StatusNotFound, Code: "not_found", Message: msg}
}

func forbidden(msg string) *APIError {
	return &APIError{Status: http.StatusForbidden, Code: "forbidden", Message: msg}
}

func badRequest(status int, msg string) *APIError {
	return &APIError{Status: status, Code: "bad_request", Message: msg}
}

type errorBody struct {
	Code    string               `json:"code"`
	Message string               `json:"message"`
	Details []definition.Problem `json:"details,omitempty"`
}

// WriteError renders err as the standard error envelope.
// Order: *APIError, *definition.ValidationError, errorMappings, then 500 internal.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *APIError
	var verr *definition.ValidationError
	switch {
	case errors.As(err, &apiErr):
		writeEnvelope(w, apiErr.Status, apiErr.Code, apiErr.Message, apiErr.Details)
		return
	case errors.As(err, &verr):
		writeEnvelope(w, http.StatusUnprocessableEntity, "validation_failed", "validation failed", verr.Problems)
		return
	}
	for _, m := range errorMappings {
		if errors.Is(err, m.target) {
			writeEnvelope(w, m.status, m.code, m.message, nil)
			return
		}
	}
	slog.Error("internal error", "err", err, "method", r.Method, "path", r.URL.Path)
	writeEnvelope(w, http.StatusInternalServerError, "internal", "internal server error", nil)
}

func writeEnvelope(w http.ResponseWriter, status int, code, msg string, details []definition.Problem) {
	WriteJSON(w, status, map[string]errorBody{"error": {Code: code, Message: msg, Details: details}})
}

// WriteJSON writes v as JSON with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("write json", "err", err)
	}
}

const maxBodyBytes = 1 << 20

// DecodeJSON reads at most 1 MiB and unmarshals it into v. Unknown fields are ignored.
func DecodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return badRequest(http.StatusBadRequest, "request body is required")
	}
	data, err := io.ReadAll(io.LimitReader(r.Body, maxBodyBytes+1))
	if err != nil {
		return badRequest(http.StatusBadRequest, "could not read request body")
	}
	if len(data) > maxBodyBytes {
		return badRequest(http.StatusRequestEntityTooLarge, "request body exceeds 1 MiB")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return badRequest(http.StatusBadRequest, "request body is required")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return badRequest(http.StatusBadRequest, "invalid JSON: "+err.Error())
	}
	return nil
}

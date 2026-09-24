package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

func TestWriteErrorMapsDefsSubsErrors(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		detail string
	}{
		{"validation", &definition.ValidationError{Problems: []definition.Problem{{Path: "slug", Message: "bad"}}}, 422, "validation_failed", "slug"},
		{"wrapped validation", fmt.Errorf("apply: %w", &definition.ValidationError{Problems: []definition.Problem{{Path: "forms[0]", Message: "bad"}}}), 422, "validation_failed", "forms[0]"},
		{"definition not found", definitions.ErrNotFound, 404, "not_found", ""},
		{"wrapped definition not found", fmt.Errorf("workflow %q: %w", "x", definitions.ErrNotFound), 404, "not_found", ""},
		{"submission not found", submissions.ErrNotFound, 404, "not_found", ""},
		{"form not public", submissions.ErrFormNotPublic, 404, "not_found", ""},
		{"invalid cursor", submissions.ErrInvalidCursor, 400, "bad_request", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			WriteError(rec, httptest.NewRequest("GET", "/", nil), c.err)
			if rec.Code != c.status {
				t.Fatalf("status = %d, want %d", rec.Code, c.status)
			}
			var env struct {
				Error struct {
					Code    string `json:"code"`
					Details []struct {
						Path string `json:"path"`
					} `json:"details"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatal(err)
			}
			if env.Error.Code != c.code {
				t.Fatalf("code = %q, want %q", env.Error.Code, c.code)
			}
			if c.detail != "" && (len(env.Error.Details) != 1 || env.Error.Details[0].Path != c.detail) {
				t.Fatalf("details = %+v", env.Error.Details)
			}
		})
	}
}

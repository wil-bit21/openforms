package httpapi_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/httpapi"
)

type envelope struct {
	Error struct {
		Code    string               `json:"code"`
		Message string               `json:"message"`
		Details []definition.Problem `json:"details"`
	} `json:"error"`
}

func writeErr(err error) (*httptest.ResponseRecorder, envelope) {
	rec := httptest.NewRecorder()
	httpapi.WriteError(rec, httptest.NewRequest(http.MethodGet, "/x", nil), err)
	var env envelope
	json.Unmarshal(rec.Body.Bytes(), &env) //nolint:errcheck
	return rec, env
}

func TestWriteErrorMapping(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"api error", &httpapi.APIError{Status: 418, Code: "teapot", Message: "short and stout"}, 418, "teapot"},
		{"validation", &definition.ValidationError{Problems: []definition.Problem{{Path: "email", Message: "bad"}}}, 422, "validation_failed"},
		{"wrapped unauthenticated", fmt.Errorf("ctx: %w", auth.ErrUnauthenticated), 401, "unauthenticated"},
		{"invalid credentials", auth.ErrInvalidCredentials, 401, "invalid_credentials"},
		{"email taken", auth.ErrEmailTaken, 409, "email_taken"},
		{"not found", auth.ErrNotFound, 404, "not_found"},
		{"unknown", errors.New("database exploded"), 500, "internal"},
	}
	for _, c := range cases {
		rec, env := writeErr(c.err)
		if rec.Code != c.status || env.Error.Code != c.code {
			t.Errorf("%s: got %d/%q want %d/%q", c.name, rec.Code, env.Error.Code, c.status, c.code)
		}
		if env.Error.Message == "" {
			t.Errorf("%s: empty message", c.name)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Errorf("%s: content-type %q", c.name, ct)
		}
	}
	_, env := writeErr(&definition.ValidationError{Problems: []definition.Problem{{Path: "email", Message: "bad"}}})
	if len(env.Error.Details) != 1 || env.Error.Details[0].Path != "email" {
		t.Errorf("details = %+v", env.Error.Details)
	}
	rec, env := writeErr(errors.New("database exploded"))
	if strings.Contains(rec.Body.String(), "exploded") || env.Error.Details != nil {
		t.Errorf("internal errors must not leak: %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"details"`) {
		t.Errorf("details must be omitted when empty: %s", rec.Body.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	var v struct {
		Name string `json:"name"`
	}
	ok := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada","extra":1}`))
	if err := httpapi.DecodeJSON(ok, &v); err != nil || v.Name != "ada" {
		t.Fatalf("valid body: %v %+v", err, v)
	}
	cases := map[string]struct {
		body   string
		status int
	}{
		"malformed": {`{"name":`, 400},
		"empty":     {``, 400},
		"too large": {`{"name":"` + strings.Repeat("a", 1<<20) + `"}`, 413},
	}
	for name, c := range cases {
		err := httpapi.DecodeJSON(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(c.body)), &v)
		var apiErr *httpapi.APIError
		if !errors.As(err, &apiErr) || apiErr.Status != c.status || apiErr.Code != "bad_request" {
			t.Errorf("%s: err = %#v, want %d bad_request", name, err, c.status)
		}
	}
}

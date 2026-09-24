package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

func TestApplyPostsBundleWithSourceAndDryRun(t *testing.T) {
	var gotMethod, gotPath, gotQuery, gotAuth, gotCT string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath, gotQuery = r.Method, r.URL.Path, r.URL.RawQuery
		gotAuth, gotCT = r.Header.Get("Authorization"), r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[{"kind":"form","slug":"contact","version":2,"changed":true,"created":false}]}`)
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "ofk_test")
	res, err := c.Apply(context.Background(), Bundle{Forms: []definition.Form{{Slug: "contact", Title: "Contact"}}}, true)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/api/v1/definitions/apply" {
		t.Fatalf("request = %s %s", gotMethod, gotPath)
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("source") != "cli" || q.Get("dryRun") != "true" {
		t.Fatalf("query = %q", gotQuery)
	}
	if gotAuth != "Bearer ofk_test" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if !strings.Contains(string(gotBody), `"workflows":[]`) {
		t.Fatalf("nil workflows must be sent as [], body = %s", gotBody)
	}
	var sent Bundle
	if err := json.Unmarshal(gotBody, &sent); err != nil || sent.Forms[0].Slug != "contact" {
		t.Fatalf("body = %s (%v)", gotBody, err)
	}
	want := ApplyItem{Kind: "form", Slug: "contact", Version: 2, Changed: true}
	if len(res.Items) != 1 || res.Items[0] != want {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestApplyWithoutDryRunOmitsParam(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		io.WriteString(w, `{"items":[]}`)
	}))
	defer srv.Close()
	if _, err := New(srv.URL, "k").Apply(context.Background(), Bundle{}, false); err != nil {
		t.Fatal(err)
	}
	q, _ := url.ParseQuery(gotQuery)
	if q.Get("source") != "cli" || q.Has("dryRun") {
		t.Fatalf("query = %q", gotQuery)
	}
}

func TestValidateReturnsAPIErrorWithDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/definitions/validate" {
			t.Errorf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		io.WriteString(w, `{"error":{"code":"validation_failed","message":"definitions are invalid","details":[{"path":"fields[0].type","message":"unknown field type"}]}}`)
	}))
	defer srv.Close()

	_, err := New(srv.URL, "k").Validate(context.Background(), Bundle{})
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v, want *Error", err)
	}
	if apiErr.Status != 422 || apiErr.Code != "validation_failed" || apiErr.Message != "definitions are invalid" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
	if len(apiErr.Details) != 1 || apiErr.Details[0].Path != "fields[0].type" {
		t.Fatalf("details = %+v", apiErr.Details)
	}
	if !strings.Contains(apiErr.Error(), "validation_failed") {
		t.Fatalf("Error() = %q", apiErr.Error())
	}
}

func TestValidateOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"valid":true}`)
	}))
	defer srv.Close()
	res, err := New(srv.URL, "k").Validate(context.Background(), Bundle{})
	if err != nil || !res.Valid {
		t.Fatalf("res=%+v err=%v", res, err)
	}
}

func TestExportDecodesBundle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/definitions" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		io.WriteString(w, `{"forms":[{"slug":"contact","title":"Contact","settings":{"public":true},"fields":[]}],
		  "workflows":[{"slug":"triage","title":"Triage","initial":"new","states":[{"key":"new","label":"New"}],"transitions":[]}]}`)
	}))
	defer srv.Close()
	b, err := New(srv.URL, "k").Export(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Forms) != 1 || b.Forms[0].Slug != "contact" || !b.Forms[0].Settings.Public {
		t.Fatalf("forms = %+v", b.Forms)
	}
	if len(b.Workflows) != 1 || b.Workflows[0].Initial != "new" {
		t.Fatalf("workflows = %+v", b.Workflows)
	}
}

func TestNonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		io.WriteString(w, "Bad gateway\n")
	}))
	defer srv.Close()
	_, err := New(srv.URL, "k").Export(context.Background())
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %v", err)
	}
	if apiErr.Status != 502 || apiErr.Code != "http_error" || apiErr.Message != "Bad gateway" {
		t.Fatalf("apiErr = %+v", apiErr)
	}
}

func TestUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	_, err := New(base, "k").Export(context.Background())
	if err == nil || !strings.HasPrefix(err.Error(), "cannot reach openforms server at "+base) {
		t.Fatalf("err = %v", err)
	}
}

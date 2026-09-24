# openforms Plan 05 — CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents).

**Goal:** Ship the developer-facing half of the `openforms` binary: `admin create-user`, `admin create-api-key`, `init`, `validate`, `push`, `pull` and `diff`, plus the Go API client they use.

**Architecture:** `internal/client` is a small HTTP client for the definitions endpoints (§7.2). `internal/cli` holds one cobra command constructor per file. Each constructor takes a `Deps` value (output writers, working directory, env lookup, a client factory and a DB opener), so tests can drive commands with `SetArgs` and in-memory buffers, without real stdout, env or network. Local definitions are loaded and validated offline with `internal/definition` (Plan 02). `pull` writes **canonical YAML**, with a fixed key order, so pulling is byte-stable and `pull --check` can serve as a CI drift gate.

**Tech Stack:** Go ≥ 1.24, `github.com/spf13/cobra`, `sigs.k8s.io/yaml` (config file), `gopkg.in/yaml.v3` (ordered YAML output via `yaml.Node`), `net/http/httptest`, `internal/testutil` + `internal/db/dbtest` (Plan 01) for DB-backed tests.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md`. Binding sections: §5 (definition formats and rules), §6.4 (`definition` API), §6.5 (`auth.Service`), §6.14 (`internal/client`), §6.15 (`testutil`), §7.1 (error envelope), §7.2 (`/definitions/*` endpoints), §8 (CLI table, config resolution, file naming).

**Assumes complete:** Plan 01 (config, db, auth, httpapi skeleton, testutil, `internal/cli/root.go` with `serve`/`migrate`), Plan 02 (`schemas`, `internal/definition`), Plan 03 (`/api/v1/definitions`, `/definitions/validate`, `/definitions/apply`, `definitions.Store`, `submissions.Service`).

## Global Constraints

- Module path `github.com/openforms/openforms`; Go ≥ 1.24.
- The same binary is server + CLI; commands are added under the root created in Plan 01 (`cmd/openforms/main.go` calls `cli.Execute()`).
- Config resolution for remote commands: flags `--server`, `--api-key` > env `OPENFORMS_URL`, `OPENFORMS_API_KEY` > `openforms.yaml` in the working directory (`server:`; API keys are **never** read from the file).
- Definitions directory resolution: `--dir` flag > `dir:` in `openforms.yaml` > `./openforms`.
- File naming: `<dir>/forms/<slug>.yaml`, `<dir>/workflows/<slug>.yaml`. `validate` also reads `.yml` and `.json`. A file's `slug` must equal its basename.
- `init` writes `openforms.yaml` (`server: http://localhost:8080`, `dir: openforms`), `openforms/forms/contact.yaml`, `openforms/workflows/contact-triage.yaml`, and refuses to overwrite existing files.
- `push` calls `POST /api/v1/definitions/apply?source=cli` (+ `&dryRun=true`). `pull` calls `GET /api/v1/definitions`. Validation uses `POST /api/v1/definitions/validate`.
- Problems print as `file:path: message` (or `file: message` when there's no path). Exit code 1 on problems, drift (`pull --check`) or any error.
- API errors are decoded from the envelope `{"error":{"code","message","details":[{"path","message"}]}}` into `*client.Error{Status, Code, Message, Details}`.
- Admin commands (`admin create-user`, `admin create-api-key`) talk to the DB directly through `auth.Service` and require `OPENFORMS_DATABASE_URL`.
- All tests that need Postgres use `testutil.NewEnv(t)` / `dbtest.New(t)` (default DSN `postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable`, override with `OPENFORMS_TEST_DATABASE_URL`). Run `docker compose up -d postgres` first.
- Commit after every task with conventional prefixes.

## Review Focus

1. **Files saved on Windows (CRLF line endings, UTF-8 BOM)** must validate and push exactly like LF files. Tested in Task 3 (`TestLoadLocalAcceptsCRLFAndBOM`).
2. **Strings that look like other YAML types** (`"yes"`, `"on"`, `"123"`, `"null"`, `"{{submission.data.email}}"`) must come back from `pull` as the same strings, not booleans or numbers, or the next `push` silently changes the definition. Tested in Task 2 (`TestCanonicalYAMLKeepsAmbiguousStrings`).
3. **Server unreachable or wrong URL:** `push`/`pull`/`diff` must fail with a readable "cannot reach openforms server at …" message, not a panic or raw net error dump. Tested in Task 1 (`TestUnreachableServer`) and Task 7 (`TestPushUnreachableServer`).
4. **Missing or invalid API key:** the user must be told to check `--api-key` / `OPENFORMS_API_KEY`, not shown a bare `401`. Tested in Task 4 (`TestExplainRemoteError`) and Task 9 (e2e, wrong key).
5. **`pull` never destroys local-only work:** files for slugs that don't exist on the server are left untouched and reported, and `<slug>.yml`/`.json` files block the pull with an explanation rather than creating a duplicate `<slug>.yaml`. Tested in Task 8 (`TestPullKeepsLocalOnlyFiles`, `TestPullRefusesAlternateExtension`).

---

## File Structure

| File | Responsibility |
|---|---|
| `internal/client/client.go` | HTTP client for `/definitions/*`; `Bundle`, `ApplyItem`, `ApplyResult`, `ValidateResult`, `Error` |
| `internal/client/client_test.go` | Tests against `httptest.Server` fakes |
| `internal/cli/yamlout.go` | `CanonicalYAML`: deterministic, key-ordered YAML |
| `internal/cli/local.go` | `LoadLocal`: read + validate a definitions directory offline; `Problem` |
| `internal/cli/deps.go` | `Deps`, `Remote`, `DefaultDeps`, project config + flag/env resolution, shared error helpers |
| `internal/cli/helpers_test.go` | Test helpers: fake remote, test deps, file helpers |
| `internal/cli/admin.go` | `admin create-user`, `admin create-api-key` |
| `internal/cli/templates/contact.yaml`, `contact-triage.yaml` | `init` scaffolding sources (embedded) |
| `internal/cli/init.go` | `init` |
| `internal/cli/validate.go` | `validate` |
| `internal/cli/push.go` | `push` |
| `internal/cli/diff.go` | `diff` |
| `internal/cli/pull.go` | `pull` |
| `internal/cli/commands.go` | `Commands(d)`: the list registered in `root.go` |
| `internal/cli/e2e_test.go` | push→pull round trip against a real `httpapi.NewRouter` |

## Task order and parallelism

- **Parallel group A:** Tasks 1, 2 and 3 (disjoint files: `internal/client/*`, `internal/cli/yamlout*`, `internal/cli/local*`). Task 3 imports `client.Bundle`, so if Task 3 runs in parallel with Task 1, its implementer creates `internal/client/client.go` with only the `Bundle` type block from Task 1 Step 3, and merges it when Task 1 lands. Only Task 2 adds a module (`gopkg.in/yaml.v3`), so resolve any `go.mod` conflict at merge with `go mod tidy`.
- **Task 4:** sequential, after group A.
- **Parallel group B:** Tasks 5, 6, 7 and 8 (disjoint files).
- **Task 9:** sequential, last.

---

### Task 1: Go API client (`internal/client`)

**Parallel group:** A

**Files:**
- Create: `internal/client/client.go`
- Test: `internal/client/client_test.go`

**Interfaces:**
- Consumes: `definition.Form`, `definition.Workflow`, `definition.Problem` (Plan 02, spec §6.4).
- Produces:
  ```go
  type Bundle struct { Forms []definition.Form `json:"forms"`; Workflows []definition.Workflow `json:"workflows"` }
  type ApplyItem struct { Kind string; Slug string; Version int; Changed bool; Created bool } // json: kind, slug, version, changed, created
  type ApplyResult struct { Items []ApplyItem `json:"items"` }
  type ValidateResult struct { Valid bool `json:"valid"` }
  type Error struct { Status int; Code, Message string; Details []definition.Problem }
  func (e *Error) Error() string
  func New(baseURL, apiKey string) *Client
  func (c *Client) Validate(ctx context.Context, b Bundle) (ValidateResult, error)
  func (c *Client) Apply(ctx context.Context, b Bundle, dryRun bool) (ApplyResult, error)
  func (c *Client) Export(ctx context.Context) (Bundle, error)
  ```
  Transport failures return an error whose text starts with `cannot reach openforms server at <baseURL>`. Nil slices in a `Bundle` are sent as `[]`.

- [ ] **Step 1: Write the failing tests**

`internal/client/client_test.go`:
```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/client/ -v`
Expected: FAIL: build errors such as `undefined: New`, `undefined: Bundle`.

- [ ] **Step 3: Write the implementation**

`internal/client/client.go`:
```go
// Package client is a small Go client for the openforms HTTP API, used by the CLI.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/openforms/openforms/internal/definition"
)

// Bundle is the wire shape of /api/v1/definitions (export, validate, apply).
type Bundle struct {
	Forms     []definition.Form     `json:"forms"`
	Workflows []definition.Workflow `json:"workflows"`
}

// ApplyItem mirrors definitions.ApplyItem as JSON (spec §7.3).
type ApplyItem struct {
	Kind    string `json:"kind"`
	Slug    string `json:"slug"`
	Version int    `json:"version"`
	Changed bool   `json:"changed"`
	Created bool   `json:"created"`
}

type ApplyResult struct {
	Items []ApplyItem `json:"items"`
}

type ValidateResult struct {
	Valid bool `json:"valid"`
}

// Error is an API error decoded from the error envelope (spec §7.1).
type Error struct {
	Status  int
	Code    string
	Message string
	Details []definition.Problem
}

func (e *Error) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d)", e.Code, e.Message, e.Status)
}

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) Validate(ctx context.Context, b Bundle) (ValidateResult, error) {
	var out ValidateResult
	err := c.do(ctx, http.MethodPost, "/api/v1/definitions/validate", normalize(b), &out)
	return out, err
}

func (c *Client) Apply(ctx context.Context, b Bundle, dryRun bool) (ApplyResult, error) {
	q := url.Values{"source": {"cli"}}
	if dryRun {
		q.Set("dryRun", "true")
	}
	var out ApplyResult
	err := c.do(ctx, http.MethodPost, "/api/v1/definitions/apply?"+q.Encode(), normalize(b), &out)
	return out, err
}

func (c *Client) Export(ctx context.Context) (Bundle, error) {
	var out Bundle
	err := c.do(ctx, http.MethodGet, "/api/v1/definitions", nil, &out)
	return normalize(out), err
}

func normalize(b Bundle) Bundle {
	if b.Forms == nil {
		b.Forms = []definition.Form{}
	}
	if b.Workflows == nil {
		b.Workflows = []definition.Workflow{}
	}
	return b
}

func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var rdr io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, rdr)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach openforms server at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return fmt.Errorf("read response from %s: %w", c.baseURL, err)
	}
	if resp.StatusCode >= 300 {
		return decodeError(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response from %s%s: %w", c.baseURL, path, err)
	}
	return nil
}

func decodeError(status int, data []byte) error {
	var env struct {
		Error struct {
			Code    string               `json:"code"`
			Message string               `json:"message"`
			Details []definition.Problem `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal(data, &env); err == nil && env.Error.Code != "" {
		return &Error{Status: status, Code: env.Error.Code, Message: env.Error.Message, Details: env.Error.Details}
	}
	msg := strings.TrimSpace(string(data))
	if msg == "" {
		msg = http.StatusText(status)
	}
	if len(msg) > 200 {
		msg = msg[:200]
	}
	return &Error{Status: status, Code: "http_error", Message: msg}
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/client/ -v`
Expected: PASS for all 7 tests (`TestApplyPostsBundleWithSourceAndDryRun`, `TestApplyWithoutDryRunOmitsParam`, `TestValidateReturnsAPIErrorWithDetails`, `TestValidateOK`, `TestExportDecodesBundle`, `TestNonJSONErrorBody`, `TestUnreachableServer`).

- [ ] **Step 5: Commit**

```bash
git add internal/client/
git commit -m "feat(client): add Go API client for definitions endpoints"
```

---

### Task 2: Canonical YAML output (`CanonicalYAML`)

**Parallel group:** A

**Files:**
- Create: `internal/cli/yamlout.go`
- Test: `internal/cli/yamlout_test.go`

**Interfaces:**
- Consumes: `definition.ParseForm`, `definition.ParseWorkflow`, `definition.Form`, `definition.Workflow` (Plan 02).
- Produces: `func CanonicalYAML(v any) ([]byte, error)`. It JSON-marshals `v` and emits YAML with 2-space indentation, with map keys ordered by the fixed `keyOrder` list and unknown keys after those, alphabetically. Scalars are encoded by `yaml.v3`, so strings that look like other types get quoted. Output is idempotent: `CanonicalYAML(parse(CanonicalYAML(x))) == CanonicalYAML(x)`.

**Key order (binding for pulled files):** `slug, key, value, type, title, label, description, workflow, initial, settings, public, submitLabel, confirmationMessage, required, placeholder, help, options, validation, minLength, maxLength, pattern, min, max, showIf, field, equals, notEquals, in, from, to, guard, roles, requireFields, actions, url, subject, body, user, role, color, terminal, states, fields, onSubmit, transitions`, then any other key alphabetically. With this order, a form reads `slug, title, description, workflow, settings, fields`, a workflow reads `slug, title, initial, states, fields, onSubmit, transitions`, a field reads `key, type, label, required, …`, and a transition reads `key, label, from, to, guard, actions`.

- [ ] **Step 1: Add the dependency**

Run: `go get gopkg.in/yaml.v3@latest`
Expected: `go.mod` gains `gopkg.in/yaml.v3`.

- [ ] **Step 2: Write the failing tests**

`internal/cli/yamlout_test.go`:
```go
package cli

import (
	"reflect"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

func TestCanonicalYAMLKeyOrder(t *testing.T) {
	f := definition.Form{
		Slug:     "contact",
		Title:    "Contact us",
		Settings: definition.FormSettings{Public: true},
		Fields: []definition.Field{
			{Key: "name", Type: definition.FieldText, Label: "Name", Required: true},
			{Key: "topic", Type: definition.FieldSelect, Label: "Topic",
				Options: []definition.Option{{Value: "sales", Label: "Sales"}}},
		},
	}
	got, err := CanonicalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `slug: contact
title: Contact us
settings:
  public: true
fields:
  - key: name
    type: text
    label: Name
    required: true
  - key: topic
    type: select
    label: Topic
    options:
      - value: sales
        label: Sales
`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestCanonicalYAMLWorkflowKeyOrder(t *testing.T) {
	w := definition.Workflow{
		Slug: "triage", Title: "Triage", Initial: "new",
		States: []definition.State{{Key: "new", Label: "New"}, {Key: "done", Label: "Done", Terminal: true}},
		Transitions: []definition.Transition{{
			Key: "finish", Label: "Finish", From: []string{"new"}, To: "done",
			Guard: definition.Guard{Roles: []string{"support"}},
		}},
	}
	got, err := CanonicalYAML(w)
	if err != nil {
		t.Fatal(err)
	}
	want := `slug: triage
title: Triage
initial: new
states:
  - key: new
    label: New
  - key: done
    label: Done
    terminal: true
transitions:
  - key: finish
    label: Finish
    from:
      - new
    to: done
    guard:
      roles:
        - support
`
	if string(got) != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

func sampleForm() definition.Form {
	two, hundred := 2, 100
	zero, fifty := 0.0, 50.0
	return definition.Form{
		Slug: "job", Title: "Job", Description: "Apply now", Workflow: "hiring",
		Settings: definition.FormSettings{Public: true, SubmitLabel: "Send", ConfirmationMessage: "Thanks"},
		Fields: []definition.Field{
			{Key: "name", Type: definition.FieldText, Label: "Name", Required: true,
				Validation: &definition.Validation{MinLength: &two, MaxLength: &hundred}},
			{Key: "years", Type: definition.FieldNumber, Label: "Years",
				Validation: &definition.Validation{Min: &zero, Max: &fifty}},
			{Key: "role", Type: definition.FieldSelect, Label: "Role", Options: []definition.Option{
				{Value: "engineer", Label: "Engineer"}, {Value: "designer", Label: "Designer"}}},
			{Key: "portfolio", Type: definition.FieldURL, Label: "Portfolio",
				ShowIf: &definition.Condition{Field: "role", Equals: "designer"}},
		},
	}
}

func sampleWorkflow() definition.Workflow {
	return definition.Workflow{
		Slug: "hiring", Title: "Hiring", Initial: "new",
		States: []definition.State{
			{Key: "new", Label: "New", Color: "gray"},
			{Key: "rejected", Label: "Rejected", Color: "red", Terminal: true},
		},
		Fields: []definition.WorkflowField{{Key: "reason", Type: definition.FieldTextarea, Label: "Reason"}},
		OnSubmit: []definition.Action{{Type: definition.ActionAssign, Role: "reviewer"}},
		Transitions: []definition.Transition{{
			Key: "reject", Label: "Reject", From: []string{"new"}, To: "rejected",
			Guard: definition.Guard{Roles: []string{"reviewer"}, RequireFields: []string{"reason"}},
			Actions: []definition.Action{
				{Type: definition.ActionEmail, To: "{{submission.data.email}}", Subject: "Update", Body: "Hi {{submission.data.name}}"},
				{Type: definition.ActionWebhook, URL: "https://example.com/hook"},
			},
		}},
	}
}

func TestCanonicalYAMLRoundTripsForm(t *testing.T) {
	f := sampleForm()
	out, err := CanonicalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := definition.ParseForm(out)
	if err != nil {
		t.Fatalf("ParseForm(canonical): %v\n%s", err, out)
	}
	if !reflect.DeepEqual(back, f) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v\nyaml:\n%s", back, f, out)
	}
}

func TestCanonicalYAMLRoundTripsWorkflow(t *testing.T) {
	w := sampleWorkflow()
	out, err := CanonicalYAML(w)
	if err != nil {
		t.Fatal(err)
	}
	back, err := definition.ParseWorkflow(out)
	if err != nil {
		t.Fatalf("ParseWorkflow(canonical): %v\n%s", err, out)
	}
	if !reflect.DeepEqual(back, w) {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v\nyaml:\n%s", back, w, out)
	}
}

func TestCanonicalYAMLIsIdempotent(t *testing.T) {
	first, err := CanonicalYAML(sampleWorkflow())
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := definition.ParseWorkflow(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalYAML(parsed)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatalf("not idempotent:\n%s\n---\n%s", first, second)
	}
}

// Review Focus #2: strings that look like booleans, numbers or null must stay strings.
func TestCanonicalYAMLKeepsAmbiguousStrings(t *testing.T) {
	f := definition.Form{
		Slug: "tricky", Title: "123", Settings: definition.FormSettings{Public: false},
		Fields: []definition.Field{
			{Key: "answer", Type: definition.FieldSelect, Label: "true", Placeholder: "null",
				Options: []definition.Option{
					{Value: "yes", Label: "on"}, {Value: "no", Label: "off"},
					{Value: "123", Label: "1e3"}, {Value: "null", Label: "~"},
				}},
			{Key: "detail", Type: definition.FieldText, Label: "{{submission.data.email}}",
				ShowIf: &definition.Condition{Field: "answer", In: []any{"yes", "123"}}},
		},
	}
	out, err := CanonicalYAML(f)
	if err != nil {
		t.Fatal(err)
	}
	back, err := definition.ParseForm(out)
	if err != nil {
		t.Fatalf("ParseForm: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(back, f) {
		t.Fatalf("ambiguous strings changed type:\n got %+v\nwant %+v\nyaml:\n%s", back, f, out)
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run CanonicalYAML -v`
Expected: FAIL: `undefined: CanonicalYAML`.

- [ ] **Step 4: Write the implementation**

`internal/cli/yamlout.go`:
```go
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// keyOrder fixes the order of mapping keys in pulled files so output is
// byte-stable and reads naturally. Keys not listed sort after these, alphabetically.
var keyOrder = []string{
	"slug", "key", "value", "type", "title", "label", "description", "workflow", "initial",
	"settings", "public", "submitLabel", "confirmationMessage", "required", "placeholder", "help",
	"options", "validation", "minLength", "maxLength", "pattern", "min", "max",
	"showIf", "field", "equals", "notEquals", "in",
	"from", "to", "guard", "roles", "requireFields", "actions",
	"url", "subject", "body", "user", "role", "color", "terminal",
	"states", "fields", "onSubmit", "transitions",
}

var keyRank = func() map[string]int {
	m := make(map[string]int, len(keyOrder))
	for i, k := range keyOrder {
		m[k] = i
	}
	return m
}()

// CanonicalYAML renders v (via its JSON form) as deterministic YAML.
func CanonicalYAML(v any) ([]byte, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var tree any
	if err := dec.Decode(&tree); err != nil {
		return nil, err
	}
	node, err := toNode(tree)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(node); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func toNode(v any) (*yaml.Node, error) {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return lessKey(keys[i], keys[j]) })
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for _, k := range keys {
			kn, err := scalar(k)
			if err != nil {
				return nil, err
			}
			vn, err := toNode(t[k])
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, kn, vn)
		}
		return n, nil
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, item := range t {
			c, err := toNode(item)
			if err != nil {
				return nil, err
			}
			n.Content = append(n.Content, c)
		}
		return n, nil
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return scalar(i)
		}
		f, err := t.Float64()
		if err != nil {
			return nil, fmt.Errorf("invalid number %q: %w", t, err)
		}
		return scalar(f)
	default: // string, bool, nil
		return scalar(t)
	}
}

// scalar lets yaml.v3 pick tag and quoting, so "yes" or "123" stay strings.
func scalar(v any) (*yaml.Node, error) {
	n := &yaml.Node{}
	if err := n.Encode(v); err != nil {
		return nil, err
	}
	return n, nil
}

func lessKey(a, b string) bool {
	ra, oka := keyRank[a]
	rb, okb := keyRank[b]
	switch {
	case oka && okb:
		return ra < rb
	case oka:
		return true
	case okb:
		return false
	default:
		return a < b
	}
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run CanonicalYAML -v`
Expected: PASS for `TestCanonicalYAMLKeyOrder`, `TestCanonicalYAMLWorkflowKeyOrder`, `TestCanonicalYAMLRoundTripsForm`, `TestCanonicalYAMLRoundTripsWorkflow`, `TestCanonicalYAMLIsIdempotent`, `TestCanonicalYAMLKeepsAmbiguousStrings`.

If `TestCanonicalYAMLRoundTripsForm` fails only on a `nil` vs empty slice difference (e.g. `Fields: []` vs `nil`), the fault is in Plan 02's parser normalisation, not in this task. Fix it by making `sampleForm`/`sampleWorkflow` use exactly the zero values `ParseForm` produces. Never weaken the comparison.

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum internal/cli/yamlout.go internal/cli/yamlout_test.go
git commit -m "feat(cli): add canonical, key-ordered YAML output"
```

---

### Task 3: Offline loading and validation of a definitions directory (`LoadLocal`)

**Parallel group:** A

**Files:**
- Create: `internal/cli/local.go`
- Test: `internal/cli/local_test.go`

**Interfaces:**
- Consumes: `definition.ParseForm`, `definition.ParseWorkflow`, `definition.ValidateBundle`, `*definition.ValidationError` (Plan 02); `client.Bundle` (Task 1).
- Produces:
  ```go
  type Problem struct { File, Path, Message string }   // File is slash-separated, relative to the definitions dir, e.g. "forms/contact.yaml"
  func (p Problem) String() string                     // "file:path: message" or "file: message"
  type LocalBundle struct {
      Bundle        client.Bundle      // Forms/Workflows never nil
      FormFiles     map[string]string  // slug → relative file
      WorkflowFiles map[string]string
  }
  func LoadLocal(dir string) (LocalBundle, []Problem, error) // error only for I/O failures; problems sorted by File, then Path
  func definitionFiles(dir string) ([]string, error)          // *.yaml|*.yml|*.json (case-insensitive), sorted; missing dir → nil
  func printProblems(w io.Writer, problems []Problem)
  ```

- [ ] **Step 1: Write the failing tests**

`internal/cli/local_test.go`:
```go
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validFormYAML = `slug: contact
title: Contact
workflow: triage
settings:
  public: true
fields:
  - key: email
    type: email
    label: Email
    required: true
`

const validWorkflowYAML = `slug: triage
title: Triage
initial: new
states:
  - key: new
    label: New
  - key: done
    label: Done
    terminal: true
transitions:
  - key: finish
    label: Finish
    from: [new]
    to: done
    guard: {}
`

func put(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLocalValid(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/contact.yaml", validFormYAML)
	put(t, dir, "workflows/triage.yml", validWorkflowYAML)
	put(t, dir, "forms/README.md", "not a definition")

	lb, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if len(lb.Bundle.Forms) != 1 || lb.Bundle.Forms[0].Slug != "contact" {
		t.Fatalf("forms = %+v", lb.Bundle.Forms)
	}
	if len(lb.Bundle.Workflows) != 1 || lb.Bundle.Workflows[0].Slug != "triage" {
		t.Fatalf("workflows = %+v", lb.Bundle.Workflows)
	}
	if lb.FormFiles["contact"] != "forms/contact.yaml" || lb.WorkflowFiles["triage"] != "workflows/triage.yml" {
		t.Fatalf("files = %v %v", lb.FormFiles, lb.WorkflowFiles)
	}
}

func TestLoadLocalMissingDirsIsEmpty(t *testing.T) {
	lb, problems, err := LoadLocal(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(problems) != 0 {
		t.Fatalf("err=%v problems=%v", err, problems)
	}
	if lb.Bundle.Forms == nil || lb.Bundle.Workflows == nil || len(lb.Bundle.Forms)+len(lb.Bundle.Workflows) != 0 {
		t.Fatalf("bundle = %+v", lb.Bundle)
	}
}

func TestLoadLocalReportsSchemaProblemsPerFile(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/broken.yaml", "slug: broken\ntitle: Broken\nsettings: {public: true}\nfields:\n  - key: a\n    type: colour\n    label: A\n")
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Fatal("expected problems")
	}
	for _, p := range problems {
		if p.File != "forms/broken.yaml" {
			t.Fatalf("problem attributed to %q: %v", p.File, p)
		}
	}
}

func TestLoadLocalSlugMustMatchFileName(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/feedback.yaml", strings.Replace(validFormYAML, "workflow: triage\n", "", 1))
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := `forms/feedback.yaml:slug: slug "contact" must match file name "feedback"`
	if len(problems) != 1 || problems[0].String() != want {
		t.Fatalf("problems = %v, want [%s]", problems, want)
	}
}

func TestLoadLocalDuplicateSlugAcrossExtensions(t *testing.T) {
	dir := t.TempDir()
	noWF := strings.Replace(validFormYAML, "workflow: triage\n", "", 1)
	put(t, dir, "forms/contact.yaml", noWF)
	put(t, dir, "forms/contact.json", `{"slug":"contact","title":"Contact","settings":{"public":true},"fields":[]}`)
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "duplicate form slug") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestLoadLocalUnknownWorkflowReference(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/contact.yaml", validFormYAML) // references workflow "triage", which is absent
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := `forms/contact.yaml:workflow: unknown workflow "triage" (not found in workflows/)`
	if len(problems) != 1 || problems[0].String() != want {
		t.Fatalf("problems = %v, want [%s]", problems, want)
	}
}

// Review Focus #1: files saved on Windows.
func TestLoadLocalAcceptsCRLFAndBOM(t *testing.T) {
	dir := t.TempDir()
	crlf := strings.ReplaceAll(validFormYAML, "\n", "\r\n")
	put(t, dir, "forms/contact.yaml", "\xef\xbb\xbf"+crlf)
	put(t, dir, "workflows/triage.yaml", strings.ReplaceAll(validWorkflowYAML, "\n", "\r\n"))
	lb, problems, err := LoadLocal(dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("err=%v problems=%v", err, problems)
	}
	if lb.Bundle.Forms[0].Fields[0].Key != "email" {
		t.Fatalf("form = %+v", lb.Bundle.Forms[0])
	}
}

func TestPrintProblems(t *testing.T) {
	var buf bytes.Buffer
	printProblems(&buf, []Problem{
		{File: "forms/a.yaml", Path: "fields[0].type", Message: "bad type"},
		{File: "forms/b.yaml", Message: "yaml: line 2: did not find expected key"},
	})
	want := "forms/a.yaml:fields[0].type: bad type\nforms/b.yaml: yaml: line 2: did not find expected key\n2 problem(s) found\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'LoadLocal|PrintProblems' -v`
Expected: FAIL: `undefined: LoadLocal`, `undefined: printProblems`.

- [ ] **Step 3: Write the implementation**

`internal/cli/local.go`:
```go
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

// Problem is one validation finding for a local definition file.
type Problem struct {
	File    string // slash-separated, relative to the definitions directory
	Path    string // path inside the document, e.g. "fields[2].showIf.field"
	Message string
}

func (p Problem) String() string {
	if p.Path == "" {
		return fmt.Sprintf("%s: %s", p.File, p.Message)
	}
	return fmt.Sprintf("%s:%s: %s", p.File, p.Path, p.Message)
}

// LocalBundle is a parsed definitions directory.
type LocalBundle struct {
	Bundle        client.Bundle
	FormFiles     map[string]string
	WorkflowFiles map[string]string
}

var utf8BOM = []byte("\xef\xbb\xbf")

// LoadLocal reads <dir>/forms/* and <dir>/workflows/*, parses and validates every
// file, and checks cross-references. The error is reserved for I/O failures.
func LoadLocal(dir string) (LocalBundle, []Problem, error) {
	lb := LocalBundle{
		Bundle:        client.Bundle{Forms: []definition.Form{}, Workflows: []definition.Workflow{}},
		FormFiles:     map[string]string{},
		WorkflowFiles: map[string]string{},
	}
	var problems []Problem

	for _, kind := range []string{"forms", "workflows"} {
		files, err := definitionFiles(filepath.Join(dir, kind))
		if err != nil {
			return lb, nil, err
		}
		for _, path := range files {
			rel := kind + "/" + filepath.Base(path)
			raw, err := os.ReadFile(path)
			if err != nil {
				return lb, nil, err
			}
			raw = bytes.TrimPrefix(raw, utf8BOM)
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

			var slug string
			if kind == "forms" {
				f, err := definition.ParseForm(raw)
				if err != nil {
					problems = append(problems, toProblems(rel, err)...)
					continue
				}
				slug = f.Slug
				if p, ok := checkSlug(rel, "form", slug, base, lb.FormFiles); !ok {
					problems = append(problems, p)
					continue
				}
				lb.FormFiles[slug] = rel
				lb.Bundle.Forms = append(lb.Bundle.Forms, f)
			} else {
				w, err := definition.ParseWorkflow(raw)
				if err != nil {
					problems = append(problems, toProblems(rel, err)...)
					continue
				}
				slug = w.Slug
				if p, ok := checkSlug(rel, "workflow", slug, base, lb.WorkflowFiles); !ok {
					problems = append(problems, p)
					continue
				}
				lb.WorkflowFiles[slug] = rel
				lb.Bundle.Workflows = append(lb.Bundle.Workflows, w)
			}
		}
	}

	for _, f := range lb.Bundle.Forms {
		if f.Workflow != "" {
			if _, ok := lb.WorkflowFiles[f.Workflow]; !ok {
				problems = append(problems, Problem{
					File: lb.FormFiles[f.Slug], Path: "workflow",
					Message: fmt.Sprintf("unknown workflow %q (not found in workflows/)", f.Workflow),
				})
			}
		}
	}

	// Backstop: any remaining bundle-level rule from the definition package.
	if len(problems) == 0 {
		if err := definition.ValidateBundle(lb.Bundle.Forms, lb.Bundle.Workflows, nil); err != nil {
			problems = append(problems, toProblems("(bundle)", err)...)
		}
	}

	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Path < problems[j].Path
	})
	return lb, problems, nil
}

func checkSlug(rel, kind, slug, base string, seen map[string]string) (Problem, bool) {
	if slug != base {
		return Problem{File: rel, Path: "slug", Message: fmt.Sprintf("slug %q must match file name %q", slug, base)}, false
	}
	if prev, dup := seen[slug]; dup {
		return Problem{File: rel, Path: "slug", Message: fmt.Sprintf("duplicate %s slug %q (also defined in %s)", kind, slug, prev)}, false
	}
	return Problem{}, true
}

func toProblems(file string, err error) []Problem {
	var ve *definition.ValidationError
	if errors.As(err, &ve) && len(ve.Problems) > 0 {
		out := make([]Problem, 0, len(ve.Problems))
		for _, p := range ve.Problems {
			out = append(out, Problem{File: file, Path: p.Path, Message: p.Message})
		}
		return out
	}
	return []Problem{{File: file, Message: err.Error()}}
}

// definitionFiles lists *.yaml, *.yml and *.json files directly inside dir.
func definitionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func printProblems(w io.Writer, problems []Problem) {
	for _, p := range problems {
		fmt.Fprintln(w, p.String())
	}
	fmt.Fprintf(w, "%d problem(s) found\n", len(problems))
}
```

Duplicate-slug note: `contact.json` sorts before `contact.yaml`, so the `.yaml` file is the one reported as the duplicate. The test only checks the message, not which file.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'LoadLocal|PrintProblems' -v`
Expected: PASS for all 8 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/local.go internal/cli/local_test.go
git commit -m "feat(cli): load and validate local definition directories offline"
```

---

### Task 4: Command dependencies, project config and remote resolution

**Files:**
- Create: `internal/cli/deps.go`
- Create: `internal/cli/helpers_test.go`
- Test: `internal/cli/deps_test.go`

**Interfaces:**
- Consumes: `client.New`, `client.Bundle`, `client.ApplyResult`, `client.ValidateResult`, `*client.Error` (Task 1); `config.Load`, `db.Open`, `db.Migrate` (Plan 01); `auth.NewService`, `(*auth.Service).EnsureDefaultOrg`, `auth.RoleAdmin` (Plan 01); `Problem`, `printProblems` (Task 3).
- Produces:
  ```go
  type Remote interface {
      Validate(ctx context.Context, b client.Bundle) (client.ValidateResult, error)
      Apply(ctx context.Context, b client.Bundle, dryRun bool) (client.ApplyResult, error)
      Export(ctx context.Context) (client.Bundle, error)
  }
  type OpenAuthFunc func(ctx context.Context) (svc *auth.Service, orgID uuid.UUID, closeFn func(), err error)
  type Deps struct {
      Out, Err  io.Writer
      WorkDir   string
      Getenv    func(string) string
      NewClient func(server, apiKey string) Remote
      OpenAuth  OpenAuthFunc
  }
  func DefaultDeps() Deps
  var ErrProblems, ErrDrift error
  const ConfigFile = "openforms.yaml"
  type ProjectConfig struct { Server string `json:"server"`; Dir string `json:"dir"` }
  func LoadProjectConfig(workDir string) (ProjectConfig, error)          // missing file → zero value, nil
  type remoteFlags struct { server, apiKey, dir string }
  func addDirFlag(cmd *cobra.Command, dir *string)
  func addRemoteFlags(cmd *cobra.Command, f *remoteFlags)                 // --server, --api-key, --dir
  func (d Deps) abs(p string) string
  func (d Deps) resolveDir(flag string, pc ProjectConfig) string          // absolute
  func (d Deps) resolveRemote(f remoteFlags, pc ProjectConfig) (server, apiKey string, err error)
  func explainRemoteError(errOut io.Writer, err error) error
  ```
- Test helpers (in `helpers_test.go`, used by Tasks 5–9): `fakeRemote`, `newTestDeps(workDir string, env map[string]string, r Remote) (Deps, *testIO)`, `execute(cmd *cobra.Command, args ...string) error`, `readRel(t, dir, rel string) string`. `put` comes from Task 3's test file.

- [ ] **Step 1: Write the test helpers and failing tests**

`internal/cli/helpers_test.go`:
```go
package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
)

type fakeRemote struct {
	applied   []client.Bundle
	dryRuns   []bool
	applyRes  client.ApplyResult
	applyErr  error
	export    client.Bundle
	exportErr error
}

func (f *fakeRemote) Validate(ctx context.Context, b client.Bundle) (client.ValidateResult, error) {
	return client.ValidateResult{Valid: true}, nil
}

func (f *fakeRemote) Apply(ctx context.Context, b client.Bundle, dryRun bool) (client.ApplyResult, error) {
	f.applied = append(f.applied, b)
	f.dryRuns = append(f.dryRuns, dryRun)
	return f.applyRes, f.applyErr
}

func (f *fakeRemote) Export(ctx context.Context) (client.Bundle, error) {
	return f.export, f.exportErr
}

type testIO struct{ out, err bytes.Buffer }

func newTestDeps(workDir string, env map[string]string, r Remote) (Deps, *testIO) {
	tio := &testIO{}
	return Deps{
		Out:     &tio.out,
		Err:     &tio.err,
		WorkDir: workDir,
		Getenv:  func(k string) string { return env[k] },
		NewClient: func(server, apiKey string) Remote {
			return r
		},
		OpenAuth: func(context.Context) (*auth.Service, uuid.UUID, func(), error) {
			return nil, uuid.Nil, nil, errors.New("OpenAuth not configured in this test")
		},
	}, tio
}

// execute runs a freshly constructed command with args; cobra's own output is discarded
// because commands write through Deps.Out / Deps.Err.
func execute(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readRel(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// remoteEnv is the env used by tests that talk to a fake remote.
var remoteEnv = map[string]string{"OPENFORMS_URL": "http://fake", "OPENFORMS_API_KEY": "ofk_fake"}
```

`internal/cli/deps_test.go`:
```go
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	pc, err := LoadProjectConfig(dir)
	if err != nil || pc != (ProjectConfig{}) {
		t.Fatalf("missing file: pc=%+v err=%v", pc, err)
	}
	put(t, dir, ConfigFile, "server: http://file:8080\ndir: defs\n")
	pc, err = LoadProjectConfig(dir)
	if err != nil || pc.Server != "http://file:8080" || pc.Dir != "defs" {
		t.Fatalf("pc=%+v err=%v", pc, err)
	}
	put(t, dir, ConfigFile, "servr: typo\n")
	if _, err := LoadProjectConfig(dir); err == nil || !strings.Contains(err.Error(), ConfigFile) {
		t.Fatalf("unknown key must fail and name the file, err=%v", err)
	}
}

func TestResolveDir(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if got := d.resolveDir("", ProjectConfig{}); got != filepath.Join(wd, "openforms") {
		t.Fatalf("default = %s", got)
	}
	if got := d.resolveDir("", ProjectConfig{Dir: "defs"}); got != filepath.Join(wd, "defs") {
		t.Fatalf("config = %s", got)
	}
	if got := d.resolveDir("flagdir", ProjectConfig{Dir: "defs"}); got != filepath.Join(wd, "flagdir") {
		t.Fatalf("flag = %s", got)
	}
	abs := filepath.Join(t.TempDir(), "x")
	if got := d.resolveDir(abs, ProjectConfig{}); got != abs {
		t.Fatalf("absolute = %s", got)
	}
}

func TestResolveRemotePrecedence(t *testing.T) {
	pc := ProjectConfig{Server: "http://file"}
	env := map[string]string{"OPENFORMS_URL": "http://env", "OPENFORMS_API_KEY": "ofk_env"}
	d, _ := newTestDeps(t.TempDir(), env, nil)

	s, k, err := d.resolveRemote(remoteFlags{server: "http://flag", apiKey: "ofk_flag"}, pc)
	if err != nil || s != "http://flag" || k != "ofk_flag" {
		t.Fatalf("flags: %s %s %v", s, k, err)
	}
	s, k, err = d.resolveRemote(remoteFlags{}, pc)
	if err != nil || s != "http://env" || k != "ofk_env" {
		t.Fatalf("env: %s %s %v", s, k, err)
	}

	dNoEnv, _ := newTestDeps(t.TempDir(), map[string]string{"OPENFORMS_API_KEY": "ofk_env"}, nil)
	s, _, err = dNoEnv.resolveRemote(remoteFlags{}, pc)
	if err != nil || s != "http://file" {
		t.Fatalf("file: %s %v", s, err)
	}

	_, _, err = dNoEnv.resolveRemote(remoteFlags{}, ProjectConfig{})
	if err == nil || !strings.Contains(err.Error(), "no server configured") {
		t.Fatalf("missing server err = %v", err)
	}
	dNoKey, _ := newTestDeps(t.TempDir(), nil, nil)
	_, _, err = dNoKey.resolveRemote(remoteFlags{}, pc)
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("missing key err = %v", err)
	}
}

// Review Focus #4: auth failures must tell the user what to fix.
func TestExplainRemoteError(t *testing.T) {
	var buf bytes.Buffer
	err := explainRemoteError(&buf, &client.Error{Status: 401, Code: "unauthenticated", Message: "missing credentials"})
	if err == nil || !strings.Contains(err.Error(), "check --api-key or OPENFORMS_API_KEY") {
		t.Fatalf("401: %v", err)
	}
	err = explainRemoteError(&buf, &client.Error{Status: 403, Code: "forbidden", Message: "admin only"})
	if err == nil || !strings.Contains(err.Error(), `needs the "admin" role`) {
		t.Fatalf("403: %v", err)
	}
	err = explainRemoteError(&buf, &client.Error{Status: 422, Code: "validation_failed", Message: "invalid",
		Details: []definition.Problem{{Path: "forms[0].title", Message: "required"}}})
	if !errors.Is(err, ErrProblems) || !strings.Contains(buf.String(), "server:forms[0].title: required") {
		t.Fatalf("422: err=%v out=%q", err, buf.String())
	}
	plain := fmt.Errorf("boom")
	if got := explainRemoteError(&buf, plain); got != plain {
		t.Fatalf("passthrough: %v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'ProjectConfig|ResolveDir|ResolveRemote|ExplainRemoteError' -v`
Expected: FAIL: `undefined: Deps`, `undefined: LoadProjectConfig`, and so on.

- [ ] **Step 3: Write the implementation**

`internal/cli/deps.go`:
```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
)

// Remote is the subset of the API the CLI needs; *client.Client implements it.
type Remote interface {
	Validate(ctx context.Context, b client.Bundle) (client.ValidateResult, error)
	Apply(ctx context.Context, b client.Bundle, dryRun bool) (client.ApplyResult, error)
	Export(ctx context.Context) (client.Bundle, error)
}

// OpenAuthFunc opens the database and returns an auth service scoped to the default org.
type OpenAuthFunc func(ctx context.Context) (svc *auth.Service, orgID uuid.UUID, closeFn func(), err error)

// Deps carries everything a command touches in the outside world, so tests can swap it.
type Deps struct {
	Out, Err  io.Writer
	WorkDir   string
	Getenv    func(string) string
	NewClient func(server, apiKey string) Remote
	OpenAuth  OpenAuthFunc
}

func DefaultDeps() Deps {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return Deps{
		Out:       os.Stdout,
		Err:       os.Stderr,
		WorkDir:   wd,
		Getenv:    os.Getenv,
		NewClient: func(server, apiKey string) Remote { return client.New(server, apiKey) },
		OpenAuth:  openAuthFromEnv,
	}
}

var (
	// ErrProblems is returned after validation problems have been printed.
	ErrProblems = errors.New("definitions are invalid")
	// ErrDrift is returned by `pull --check` when local files differ from the server.
	ErrDrift = errors.New("local definitions differ from the server")
)

const ConfigFile = "openforms.yaml"

// ProjectConfig is openforms.yaml. API keys are deliberately not supported here.
type ProjectConfig struct {
	Server string `json:"server"`
	Dir    string `json:"dir"`
}

func LoadProjectConfig(workDir string) (ProjectConfig, error) {
	var pc ProjectConfig
	raw, err := os.ReadFile(filepath.Join(workDir, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return pc, nil
	}
	if err != nil {
		return pc, err
	}
	if err := yaml.UnmarshalStrict(raw, &pc); err != nil {
		return pc, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return pc, nil
}

type remoteFlags struct {
	server, apiKey, dir string
}

func addDirFlag(cmd *cobra.Command, dir *string) {
	cmd.Flags().StringVar(dir, "dir", "", "definitions directory (default: dir from openforms.yaml, else ./openforms)")
}

func addRemoteFlags(cmd *cobra.Command, f *remoteFlags) {
	addDirFlag(cmd, &f.dir)
	cmd.Flags().StringVar(&f.server, "server", "", "openforms server URL (default: $OPENFORMS_URL, else server from openforms.yaml)")
	cmd.Flags().StringVar(&f.apiKey, "api-key", "", "API key with the admin role (default: $OPENFORMS_API_KEY)")
}

func (d Deps) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(d.WorkDir, p)
}

func (d Deps) resolveDir(flag string, pc ProjectConfig) string {
	dir := flag
	if dir == "" {
		dir = pc.Dir
	}
	if dir == "" {
		dir = "openforms"
	}
	return d.abs(dir)
}

func (d Deps) resolveRemote(f remoteFlags, pc ProjectConfig) (string, string, error) {
	server := f.server
	if server == "" {
		server = d.Getenv("OPENFORMS_URL")
	}
	if server == "" {
		server = pc.Server
	}
	if server == "" {
		return "", "", errors.New("no server configured: pass --server, set OPENFORMS_URL, or add `server:` to openforms.yaml")
	}
	key := f.apiKey
	if key == "" {
		key = d.Getenv("OPENFORMS_API_KEY")
	}
	if key == "" {
		return "", "", errors.New("no API key: pass --api-key or set OPENFORMS_API_KEY (create one with `openforms admin create-api-key --name cli --roles admin`)")
	}
	return server, key, nil
}

// explainRemoteError turns API errors into actionable messages. 422 details are
// printed as problems and ErrProblems is returned.
func explainRemoteError(errOut io.Writer, err error) error {
	var apiErr *client.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == http.StatusUnauthorized:
		return fmt.Errorf("authentication failed (%s): check --api-key or OPENFORMS_API_KEY", apiErr.Code)
	case apiErr.Status == http.StatusForbidden:
		return fmt.Errorf("permission denied (%s): the API key needs the %q role", apiErr.Code, auth.RoleAdmin)
	case apiErr.Status == http.StatusUnprocessableEntity && len(apiErr.Details) > 0:
		ps := make([]Problem, 0, len(apiErr.Details))
		for _, p := range apiErr.Details {
			ps = append(ps, Problem{File: "server", Path: p.Path, Message: p.Message})
		}
		printProblems(errOut, ps)
		return ErrProblems
	}
	return err
}

func openAuthFromEnv(ctx context.Context) (*auth.Service, uuid.UUID, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, uuid.Nil, nil, err
	}
	if cfg.DatabaseURL == "" {
		return nil, uuid.Nil, nil, errors.New("OPENFORMS_DATABASE_URL is required for admin commands")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, uuid.Nil, nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, uuid.Nil, nil, err
	}
	svc := auth.NewService(pool)
	orgID, err := svc.EnsureDefaultOrg(ctx)
	if err != nil {
		pool.Close()
		return nil, uuid.Nil, nil, err
	}
	return svc, orgID, pool.Close, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'ProjectConfig|ResolveDir|ResolveRemote|ExplainRemoteError' -v`
Expected: PASS for `TestLoadProjectConfig`, `TestResolveDir`, `TestResolveRemotePrecedence`, `TestExplainRemoteError`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/deps.go internal/cli/deps_test.go internal/cli/helpers_test.go
git commit -m "feat(cli): add injectable command deps and config/flag/env resolution"
```

---

### Task 5: `admin create-user` and `admin create-api-key`

**Parallel group:** B

**Files:**
- Create: `internal/cli/admin.go`
- Test: `internal/cli/admin_test.go`

**Interfaces:**
- Consumes: `Deps`, `OpenAuthFunc`, `execute`, `newTestDeps` (Task 4); `(*auth.Service).CreateUser`, `CreateAPIKey`, `GetUserByEmail`, `PrincipalFromAPIKey`, `auth.ErrEmailTaken` (Plan 01); `testutil.NewEnv` (Plan 01).
- Produces: `func NewAdminCmd(d Deps) *cobra.Command` (subcommands `create-user`, `create-api-key`); `func splitRoles(s string) []string` (comma-separated, trimmed, empties dropped, never nil).

- [ ] **Step 1: Write the failing tests**

`internal/cli/admin_test.go`:
```go
package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/testutil"
)

func adminDeps(t *testing.T) (Deps, *testIO, *testutil.Env) {
	t.Helper()
	env := testutil.NewEnv(t)
	d, tio := newTestDeps(t.TempDir(), nil, nil)
	d.OpenAuth = func(context.Context) (*auth.Service, uuid.UUID, func(), error) {
		return env.Auth, env.OrgID, func() {}, nil
	}
	return d, tio, env
}

func TestSplitRoles(t *testing.T) {
	if got := splitRoles(" admin, reviewer ,,"); !reflect.DeepEqual(got, []string{"admin", "reviewer"}) {
		t.Fatalf("got %v", got)
	}
	if got := splitRoles(""); got == nil || len(got) != 0 {
		t.Fatalf("empty must be non-nil empty slice, got %#v", got)
	}
}

func TestAdminCreateUser(t *testing.T) {
	d, tio, env := adminDeps(t)
	err := execute(NewAdminCmd(d), "create-user", "--email", "Ada@Example.com", "--password", "correct-horse", "--roles", "admin,reviewer")
	if err != nil {
		t.Fatalf("create-user: %v", err)
	}
	u, err := env.Auth.GetUserByEmail(context.Background(), env.OrgID, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Ada" || !reflect.DeepEqual(u.Roles, []string{"admin", "reviewer"}) {
		t.Fatalf("user = %+v", u)
	}
	want := "Created user ada@example.com (" + u.ID.String() + ") with roles [admin, reviewer]\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestAdminCreateUserDuplicate(t *testing.T) {
	d, _, _ := adminDeps(t)
	args := []string{"create-user", "--email", "bob@example.com", "--password", "password123"}
	if err := execute(NewAdminCmd(d), args...); err != nil {
		t.Fatal(err)
	}
	err := execute(NewAdminCmd(d), args...)
	if err == nil || !strings.Contains(err.Error(), "a user with email bob@example.com already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminCreateUserRequiresFlags(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil) // OpenAuth fails if reached
	err := execute(NewAdminCmd(d), "create-user", "--email", "x@example.com")
	if err == nil || !strings.Contains(err.Error(), "--email and --password are required") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminCreateAPIKey(t *testing.T) {
	d, tio, env := adminDeps(t)
	if err := execute(NewAdminCmd(d), "create-api-key", "--name", "ci", "--roles", "admin"); err != nil {
		t.Fatal(err)
	}
	var key string
	for _, line := range strings.Split(tio.out.String(), "\n") {
		if strings.HasPrefix(line, "ofk_") {
			key = line
		}
	}
	if len(key) != 44 {
		t.Fatalf("no ofk_ key line (len 44) in output:\n%s", tio.out.String())
	}
	p, err := env.Auth.PrincipalFromAPIKey(context.Background(), key)
	if err != nil || !p.IsAdmin() || p.Name != "ci" {
		t.Fatalf("principal=%+v err=%v", p, err)
	}
	if !strings.Contains(tio.out.String(), "it will not be shown again") {
		t.Fatalf("missing warning:\n%s", tio.out.String())
	}
}

func TestAdminCreateAPIKeyRequiresName(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil)
	err := execute(NewAdminCmd(d), "create-api-key", "--roles", "admin")
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("err = %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `docker compose up -d postgres && go test ./internal/cli/ -run 'SplitRoles|Admin' -v`
Expected: FAIL: `undefined: NewAdminCmd`, `undefined: splitRoles`.

- [ ] **Step 3: Write the implementation**

`internal/cli/admin.go`:
```go
package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
)

func NewAdminCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands that talk directly to the database (needs OPENFORMS_DATABASE_URL)",
	}
	cmd.AddCommand(newCreateUserCmd(d), newCreateAPIKeyCmd(d))
	return cmd
}

func newCreateUserCmd(d Deps) *cobra.Command {
	var email, name, password, roles string
	cmd := &cobra.Command{
		Use:          "create-user",
		Short:        "Create a user who can sign in to the admin UI",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" || password == "" {
				return errors.New("--email and --password are required")
			}
			if name == "" {
				name = strings.SplitN(email, "@", 2)[0]
			}
			svc, orgID, closeFn, err := d.OpenAuth(cmd.Context())
			if err != nil {
				return err
			}
			defer closeFn()
			u, err := svc.CreateUser(cmd.Context(), orgID, email, name, password, splitRoles(roles))
			if errors.Is(err, auth.ErrEmailTaken) {
				return fmt.Errorf("a user with email %s already exists", strings.ToLower(email))
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Created user %s (%s) with roles [%s]\n", u.Email, u.ID, strings.Join(u.Roles, ", "))
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address (required)")
	cmd.Flags().StringVar(&name, "name", "", "display name (default: part of the email before @)")
	cmd.Flags().StringVar(&password, "password", "", "password, at least 8 characters (required)")
	cmd.Flags().StringVar(&roles, "roles", "", "comma-separated roles, e.g. admin,reviewer")
	return cmd
}

func newCreateAPIKeyCmd(d Deps) *cobra.Command {
	var name, roles string
	cmd := &cobra.Command{
		Use:          "create-api-key",
		Short:        "Create an API key (printed once)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return errors.New("--name is required")
			}
			svc, orgID, closeFn, err := d.OpenAuth(cmd.Context())
			if err != nil {
				return err
			}
			defer closeFn()
			plaintext, k, err := svc.CreateAPIKey(cmd.Context(), orgID, name, splitRoles(roles))
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Created API key %q (prefix %s) with roles [%s]\n", k.Name, k.Prefix, strings.Join(k.Roles, ", "))
			fmt.Fprintln(d.Out, plaintext)
			fmt.Fprintln(d.Out, "Store this key now; it will not be shown again.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "a label for the key, e.g. ci (required)")
	cmd.Flags().StringVar(&roles, "roles", "", "comma-separated roles; use admin for push/pull")
	return cmd
}

func splitRoles(s string) []string {
	out := []string{}
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'SplitRoles|Admin' -v`
Expected: PASS for `TestSplitRoles`, `TestAdminCreateUser`, `TestAdminCreateUserDuplicate`, `TestAdminCreateUserRequiresFlags`, `TestAdminCreateAPIKey`, `TestAdminCreateAPIKeyRequiresName`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/admin.go internal/cli/admin_test.go
git commit -m "feat(cli): add admin create-user and create-api-key commands"
```

---

### Task 6: `init` and `validate`

**Parallel group:** B

**Files:**
- Create: `internal/cli/templates/contact.yaml`
- Create: `internal/cli/templates/contact-triage.yaml`
- Create: `internal/cli/init.go`
- Create: `internal/cli/validate.go`
- Test: `internal/cli/init_validate_test.go`

**Interfaces:**
- Consumes: `Deps`, `LoadProjectConfig`, `addDirFlag`, `ErrProblems`, `ConfigFile` (Task 4); `LoadLocal`, `printProblems` (Task 3); `CanonicalYAML` (Task 2); `definition.ParseForm`, `definition.ParseWorkflow` (Plan 02).
- Produces: `func NewInitCmd(d Deps) *cobra.Command`, `func NewValidateCmd(d Deps) *cobra.Command`. `init` writes the canonical YAML of the embedded templates, so a freshly scaffolded project is already in `pull` format and `pull --check` passes right after `push`.

- [ ] **Step 1: Write the templates**

`internal/cli/templates/contact.yaml`:
```yaml
slug: contact
title: Contact us
description: Questions, feedback or support requests. We usually reply within one working day.
workflow: contact-triage
settings:
  public: true
  submitLabel: Send message
  confirmationMessage: Thanks for reaching out. We'll reply by email soon.
fields:
  - key: name
    type: text
    label: Your name
    required: true
    validation:
      minLength: 2
      maxLength: 100
  - key: email
    type: email
    label: Email
    required: true
  - key: topic
    type: select
    label: Topic
    required: true
    options:
      - value: support
        label: Product support
      - value: sales
        label: Sales
      - value: other
        label: Something else
  - key: orderNumber
    type: text
    label: Order number
    help: Helps us find your purchase faster.
    showIf:
      field: topic
      equals: support
  - key: message
    type: textarea
    label: Message
    required: true
    validation:
      maxLength: 5000
```

`internal/cli/templates/contact-triage.yaml`:
```yaml
slug: contact-triage
title: Contact triage
initial: new
states:
  - key: new
    label: New
    color: gray
  - key: in_progress
    label: In progress
    color: blue
  - key: resolved
    label: Resolved
    color: green
    terminal: true
  - key: spam
    label: Spam
    color: red
    terminal: true
fields:
  - key: resolution
    type: textarea
    label: Resolution notes
onSubmit:
  - type: assign
    role: support
transitions:
  - key: start
    label: Start working
    from: [new]
    to: in_progress
    guard:
      roles: [support]
  - key: resolve
    label: Resolve
    from: [in_progress]
    to: resolved
    guard:
      roles: [support]
      requireFields: [resolution]
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: "Re: your message"
        body: "Hi {{submission.data.name}}, {{submission.fields.resolution}}"
  - key: mark_spam
    label: Mark as spam
    from: [new, in_progress]
    to: spam
    guard:
      roles: [support]
```

- [ ] **Step 2: Write the failing tests**

`internal/cli/init_validate_test.go`:
```go
package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

var scaffolded = []string{"openforms.yaml", "openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"}

func TestInitScaffoldsProject(t *testing.T) {
	wd := t.TempDir()
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	for _, rel := range scaffolded {
		if _, err := os.Stat(filepath.Join(wd, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	if got := readRel(t, wd, "openforms.yaml"); got != "server: http://localhost:8080\ndir: openforms\n" {
		t.Fatalf("openforms.yaml = %q", got)
	}
	if !strings.Contains(tio.out.String(), "openforms push") {
		t.Fatalf("expected next steps in output:\n%s", tio.out.String())
	}
}

func TestInitIntoSubdirectory(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d), "myproject"); err != nil {
		t.Fatal(err)
	}
	readRel(t, wd, "myproject/openforms/forms/contact.yaml")
}

func TestInitRefusesToOverwrite(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", "mine")
	d, _ := newTestDeps(wd, nil, nil)
	err := execute(NewInitCmd(d))
	if err == nil || !strings.Contains(err.Error(), "openforms/forms/contact.yaml already exists") {
		t.Fatalf("err = %v", err)
	}
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); got != "mine" {
		t.Fatalf("file was overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(wd, "openforms.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init must write nothing when any target exists, stat err = %v", err)
	}
}

func TestInitWritesCanonicalYAML(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	formSrc := readRel(t, wd, "openforms/forms/contact.yaml")
	f, err := definition.ParseForm([]byte(formSrc))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := CanonicalYAML(f)
	if string(again) != formSrc {
		t.Fatalf("scaffolded form is not canonical:\n%s\n---\n%s", formSrc, again)
	}
	wfSrc := readRel(t, wd, "openforms/workflows/contact-triage.yaml")
	w, err := definition.ParseWorkflow([]byte(wfSrc))
	if err != nil {
		t.Fatal(err)
	}
	again, _ = CanonicalYAML(w)
	if string(again) != wfSrc {
		t.Fatalf("scaffolded workflow is not canonical:\n%s\n---\n%s", wfSrc, again)
	}
}

func TestValidateScaffoldedProject(t *testing.T) {
	wd := t.TempDir()
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	tio.out.Reset()
	if err := execute(NewValidateCmd(d)); err != nil {
		t.Fatalf("validate: %v\n%s", err, tio.err.String())
	}
	if tio.out.String() != "OK: 1 form(s), 1 workflow(s) valid\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestValidateReportsProblemsAndFails(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "defs/forms/feedback.yaml", "slug: contact2\ntitle: Feedback\nsettings: {public: true}\nfields: []\n")
	d, tio := newTestDeps(wd, nil, nil)
	err := execute(NewValidateCmd(d), "--dir", "defs")
	if !errors.Is(err, ErrProblems) {
		t.Fatalf("err = %v", err)
	}
	want := "forms/feedback.yaml:slug: slug \"contact2\" must match file name \"feedback\"\n1 problem(s) found\n"
	if tio.err.String() != want {
		t.Fatalf("stderr = %q, want %q", tio.err.String(), want)
	}
}

func TestValidateUsesDirFromProjectConfig(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms.yaml", "dir: defs\n")
	put(t, wd, "defs/forms/contact.yaml", "slug: contact\ntitle: Contact\nsettings: {public: true}\nfields: []\n")
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewValidateCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "OK: 1 form(s), 0 workflow(s) valid\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'Init|Validate' -v`
Expected: FAIL: `undefined: NewInitCmd`, `undefined: NewValidateCmd`.

- [ ] **Step 4: Write the implementation**

`internal/cli/init.go`:
```go
package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/definition"
)

//go:embed templates/contact.yaml
var tplContactForm []byte

//go:embed templates/contact-triage.yaml
var tplContactTriage []byte

const initConfig = "server: http://localhost:8080\ndir: openforms\n"

func NewInitCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:          "init [dir]",
		Short:        "Scaffold openforms.yaml and a sample form + workflow",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := d.WorkDir
			if len(args) == 1 {
				target = d.abs(args[0])
			}
			form, err := definition.ParseForm(tplContactForm)
			if err != nil {
				return fmt.Errorf("built-in form template is invalid: %w", err)
			}
			wf, err := definition.ParseWorkflow(tplContactTriage)
			if err != nil {
				return fmt.Errorf("built-in workflow template is invalid: %w", err)
			}
			formYAML, err := CanonicalYAML(form)
			if err != nil {
				return err
			}
			wfYAML, err := CanonicalYAML(wf)
			if err != nil {
				return err
			}
			files := []struct {
				rel  string
				data []byte
			}{
				{ConfigFile, []byte(initConfig)},
				{"openforms/forms/contact.yaml", formYAML},
				{"openforms/workflows/contact-triage.yaml", wfYAML},
			}
			for _, f := range files {
				_, err := os.Stat(filepath.Join(target, filepath.FromSlash(f.rel)))
				if err == nil {
					return fmt.Errorf("%s already exists; refusing to overwrite", f.rel)
				}
				if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			for _, f := range files {
				p := filepath.Join(target, filepath.FromSlash(f.rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, f.data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(d.Out, "created %s\n", f.rel)
			}
			fmt.Fprintln(d.Out, "\nNext steps:")
			fmt.Fprintln(d.Out, "  openforms validate")
			fmt.Fprintln(d.Out, "  export OPENFORMS_API_KEY=<key from `openforms admin create-api-key --name cli --roles admin`>")
			fmt.Fprintln(d.Out, "  openforms push")
			return nil
		},
	}
}
```

`internal/cli/validate.go`:
```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewValidateCmd(d Deps) *cobra.Command {
	var dirFlag string
	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "Validate local form and workflow definitions (offline)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(dirFlag, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			fmt.Fprintf(d.Out, "OK: %d form(s), %d workflow(s) valid\n", len(lb.Bundle.Forms), len(lb.Bundle.Workflows))
			return nil
		},
	}
	addDirFlag(cmd, &dirFlag)
	return cmd
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'Init|Validate' -v`
Expected: PASS for all 7 tests. If `TestInitScaffoldsProject` fails with "built-in form template is invalid", the template breaks a Plan 02 rule. Fix the template so it's valid, and don't loosen the parser.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/templates internal/cli/init.go internal/cli/validate.go internal/cli/init_validate_test.go
git commit -m "feat(cli): add init scaffolding and offline validate"
```

---

### Task 7: `push` and `diff`

**Parallel group:** B

**Files:**
- Create: `internal/cli/push.go`
- Create: `internal/cli/diff.go`
- Test: `internal/cli/push_diff_test.go`

**Interfaces:**
- Consumes: `Deps`, `remoteFlags`, `addRemoteFlags`, `resolveDir`, `resolveRemote`, `explainRemoteError`, `ErrProblems`, `fakeRemote`, `remoteEnv` (Task 4); `LoadLocal`, `printProblems` (Task 3); `client.ApplyResult`, `client.ApplyItem`, `client.New` (Task 1); `definition.Canonical` (Plan 02).
- Produces: `func NewPushCmd(d Deps) *cobra.Command`, `func NewDiffCmd(d Deps) *cobra.Command`, `func applyStatus(it client.ApplyItem) string` (`created` | `updated` | `unchanged`), `func diffBundles(local, remote client.Bundle) ([]string, error)`.

`push` output format:
```
KIND      SLUG            VERSION  STATUS
workflow  contact-triage  1        created
form      contact         1        created
```
(`text/tabwriter`, min width 0, padding 2; rows in server order.) In dry-run mode the first line is `Dry run: nothing was applied.`

`diff` lines are sorted by kind (`form` before `workflow`), then slug: `~ form contact`, `+ workflow triage (local only)`, `- form old (remote only)`. With no differences it prints `No differences.` and exits 0.

- [ ] **Step 1: Write the failing tests**

`internal/cli/push_diff_test.go`:
```go
package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

const contactNoWF = "slug: contact\ntitle: Contact\nsettings:\n  public: true\nfields: []\n"

func TestPushAppliesLocalBundleAndPrintsTable(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyRes: client.ApplyResult{Items: []client.ApplyItem{
		{Kind: "form", Slug: "contact", Version: 1, Changed: true, Created: true},
	}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPushCmd(d)); err != nil {
		t.Fatalf("push: %v\n%s", err, tio.err.String())
	}
	if len(fr.applied) != 1 || fr.dryRuns[0] {
		t.Fatalf("applied=%d dryRuns=%v", len(fr.applied), fr.dryRuns)
	}
	if got := fr.applied[0].Forms[0].Slug; got != "contact" {
		t.Fatalf("sent slug %q", got)
	}
	want := "KIND  SLUG     VERSION  STATUS\nform  contact  1        created\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPushDryRun(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyRes: client.ApplyResult{Items: []client.ApplyItem{{Kind: "form", Slug: "contact", Version: 3, Changed: true}}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPushCmd(d), "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if !fr.dryRuns[0] {
		t.Fatal("dry-run flag not forwarded")
	}
	if !strings.HasPrefix(tio.out.String(), "Dry run: nothing was applied.\n") || !strings.Contains(tio.out.String(), "updated") {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestPushStopsOnLocalProblems(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/wrong.yaml", contactNoWF)
	fr := &fakeRemote{}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPushCmd(d))
	if !errors.Is(err, ErrProblems) || len(fr.applied) != 0 {
		t.Fatalf("err=%v applied=%d", err, len(fr.applied))
	}
	if !strings.Contains(tio.err.String(), "forms/wrong.yaml:slug:") {
		t.Fatalf("stderr = %q", tio.err.String())
	}
}

func TestPushPrintsServerValidationProblems(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyErr: &client.Error{Status: 422, Code: "validation_failed", Message: "invalid",
		Details: []definition.Problem{{Path: "forms[0].fields", Message: "must have at least one field"}}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPushCmd(d))
	if !errors.Is(err, ErrProblems) || !strings.Contains(tio.err.String(), "server:forms[0].fields: must have at least one field") {
		t.Fatalf("err=%v stderr=%q", err, tio.err.String())
	}
}

// Review Focus #3: unreachable server gives a readable error.
func TestPushUnreachableServer(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	d, _ := newTestDeps(wd, map[string]string{"OPENFORMS_URL": base, "OPENFORMS_API_KEY": "ofk_x"}, nil)
	d.NewClient = func(server, key string) Remote { return client.New(server, key) }
	err := execute(NewPushCmd(d))
	if err == nil || !strings.Contains(err.Error(), "cannot reach openforms server at "+base) {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyStatus(t *testing.T) {
	cases := map[string]client.ApplyItem{
		"created":   {Created: true, Changed: true},
		"updated":   {Changed: true},
		"unchanged": {},
	}
	for want, it := range cases {
		if got := applyStatus(it); got != want {
			t.Fatalf("applyStatus(%+v) = %s, want %s", it, got, want)
		}
	}
}

func TestDiffBundles(t *testing.T) {
	a := definition.Form{Slug: "a", Title: "A", Fields: []definition.Field{}}
	aChanged := a
	aChanged.Title = "A2"
	b := definition.Form{Slug: "b", Title: "B", Fields: []definition.Field{}}
	c := definition.Form{Slug: "c", Title: "C", Fields: []definition.Field{}}
	w := definition.Workflow{Slug: "w", Title: "W", Initial: "s", States: []definition.State{{Key: "s", Label: "S"}}}

	local := client.Bundle{Forms: []definition.Form{aChanged, b}, Workflows: []definition.Workflow{w}}
	remote := client.Bundle{Forms: []definition.Form{a, b, c}, Workflows: []definition.Workflow{}}
	got, err := diffBundles(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"~ form a", "- form c (remote only)", "+ workflow w (local only)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDiffCommand(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	local, _, _ := LoadLocal(wd + "/openforms")
	fr := &fakeRemote{export: local.Bundle}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewDiffCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "No differences.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}

	fr.export = client.Bundle{Forms: []definition.Form{}, Workflows: []definition.Workflow{}}
	tio.out.Reset()
	if err := execute(NewDiffCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "+ form contact (local only)\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'Push|ApplyStatus|Diff' -v`
Expected: FAIL: `undefined: NewPushCmd`, `undefined: applyStatus`, `undefined: diffBundles`, `undefined: NewDiffCmd`.

- [ ] **Step 3: Write the implementation**

`internal/cli/push.go`:
```go
package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
)

func NewPushCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	var dryRun bool
	cmd := &cobra.Command{
		Use:          "push",
		Short:        "Validate local definitions and apply them to the server",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(rf.dir, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			res, err := d.NewClient(server, key).Apply(cmd.Context(), lb.Bundle, dryRun)
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			if dryRun {
				fmt.Fprintln(d.Out, "Dry run: nothing was applied.")
			}
			printApplyTable(d.Out, res)
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without applying")
	return cmd
}

func applyStatus(it client.ApplyItem) string {
	switch {
	case it.Created:
		return "created"
	case it.Changed:
		return "updated"
	default:
		return "unchanged"
	}
}

func printApplyTable(w io.Writer, res client.ApplyResult) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tSLUG\tVERSION\tSTATUS")
	for _, it := range res.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", it.Kind, it.Slug, it.Version, applyStatus(it))
	}
	tw.Flush()
}
```

`internal/cli/diff.go`:
```go
package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func NewDiffCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	cmd := &cobra.Command{
		Use:          "diff",
		Short:        "Show which definitions differ between local files and the server",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(rf.dir, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			remote, err := d.NewClient(server, key).Export(cmd.Context())
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			lines, err := diffBundles(lb.Bundle, remote)
			if err != nil {
				return err
			}
			if len(lines) == 0 {
				fmt.Fprintln(d.Out, "No differences.")
				return nil
			}
			for _, l := range lines {
				fmt.Fprintln(d.Out, l)
			}
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	return cmd
}

type defKey struct{ kind, slug string }

func bundleHashes(b client.Bundle) (map[defKey]string, error) {
	m := map[defKey]string{}
	for _, f := range b.Forms {
		_, h, err := definition.Canonical(f)
		if err != nil {
			return nil, err
		}
		m[defKey{"form", f.Slug}] = h
	}
	for _, w := range b.Workflows {
		_, h, err := definition.Canonical(w)
		if err != nil {
			return nil, err
		}
		m[defKey{"workflow", w.Slug}] = h
	}
	return m, nil
}

// diffBundles compares by canonical hash; unchanged definitions are omitted.
func diffBundles(local, remote client.Bundle) ([]string, error) {
	l, err := bundleHashes(local)
	if err != nil {
		return nil, err
	}
	r, err := bundleHashes(remote)
	if err != nil {
		return nil, err
	}
	keys := map[defKey]bool{}
	for k := range l {
		keys[k] = true
	}
	for k := range r {
		keys[k] = true
	}
	sorted := make([]defKey, 0, len(keys))
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].kind != sorted[j].kind {
			return sorted[i].kind < sorted[j].kind
		}
		return sorted[i].slug < sorted[j].slug
	})
	var out []string
	for _, k := range sorted {
		lh, inLocal := l[k]
		rh, inRemote := r[k]
		switch {
		case inLocal && !inRemote:
			out = append(out, fmt.Sprintf("+ %s %s (local only)", k.kind, k.slug))
		case !inLocal && inRemote:
			out = append(out, fmt.Sprintf("- %s %s (remote only)", k.kind, k.slug))
		case lh != rh:
			out = append(out, fmt.Sprintf("~ %s %s", k.kind, k.slug))
		}
	}
	return out, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'Push|ApplyStatus|Diff' -v`
Expected: PASS for `TestPushAppliesLocalBundleAndPrintsTable`, `TestPushDryRun`, `TestPushStopsOnLocalProblems`, `TestPushPrintsServerValidationProblems`, `TestPushUnreachableServer`, `TestApplyStatus`, `TestDiffBundles`, `TestDiffCommand`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/push.go internal/cli/diff.go internal/cli/push_diff_test.go
git commit -m "feat(cli): add push (with dry-run) and diff commands"
```

---

### Task 8: `pull` (with `--check` drift detection)

**Parallel group:** B

**Files:**
- Create: `internal/cli/pull.go`
- Test: `internal/cli/pull_test.go`

**Interfaces:**
- Consumes: `Deps`, `remoteFlags`, `addRemoteFlags`, `resolveDir`, `resolveRemote`, `explainRemoteError`, `ErrDrift`, `fakeRemote`, `remoteEnv` (Task 4); `definitionFiles` (Task 3); `CanonicalYAML` (Task 2); `client.Bundle` (Task 1).
- Produces: `func NewPullCmd(d Deps) *cobra.Command`; `type pullChange struct { Rel, Path string; Data []byte; Status string }` (`created` | `updated` | `unchanged`); `func planPull(dir string, b client.Bundle) (changes []pullChange, localOnly []string, err error)`.

Behaviour:
- Remote definitions are written to `<dir>/forms/<slug>.yaml` and `<dir>/workflows/<slug>.yaml` as `CanonicalYAML`. Files whose bytes are already equal aren't rewritten.
- If `<slug>.yml` or `<slug>.json` exists for a remote slug, pull fails before writing anything. The message names the file and asks the user to rename it.
- Local files with no matching remote slug are never touched. They're reported as `note: forms/x.yaml exists locally but not on the server (left untouched)`.
- Output: one `created <rel>` / `updated <rel>` line per written file, then `Pulled N form(s), M workflow(s): X created, Y updated, Z unchanged.`
- `--check`: writes nothing. Prints `would create <rel>` / `would update <rel>` and returns `ErrDrift` (exit 1) if there are any, otherwise prints `Up to date.`

- [ ] **Step 1: Write the failing tests**

`internal/cli/pull_test.go`:
```go
package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func remoteBundle() client.Bundle {
	return client.Bundle{
		Forms: []definition.Form{{Slug: "contact", Title: "Contact", Settings: definition.FormSettings{Public: true},
			Fields: []definition.Field{{Key: "email", Type: definition.FieldEmail, Label: "Email", Required: true}}}},
		Workflows: []definition.Workflow{{Slug: "triage", Title: "Triage", Initial: "new",
			States: []definition.State{{Key: "new", Label: "New"}}, Transitions: []definition.Transition{}}},
	}
}

func TestPullWritesCanonicalFiles(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatalf("pull: %v", err)
	}
	wantForm, _ := CanonicalYAML(remoteBundle().Forms[0])
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); got != string(wantForm) {
		t.Fatalf("form file:\n%s\nwant:\n%s", got, wantForm)
	}
	readRel(t, wd, "openforms/workflows/triage.yaml")
	want := "created forms/contact.yaml\ncreated workflows/triage.yaml\nPulled 1 form(s), 1 workflow(s): 2 created, 0 updated, 0 unchanged.\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPullIsStableAndSkipsUnchanged(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(wd, "openforms", "forms", "contact.yaml")
	before, _ := os.Stat(p)
	tio.out.Reset()
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unchanged file was rewritten")
	}
	if tio.out.String() != "Pulled 1 form(s), 1 workflow(s): 0 created, 0 updated, 2 unchanged.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestPullCheckDetectsDriftWithoutWriting(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", "slug: contact\ntitle: Old title\nsettings:\n  public: true\nfields: []\n")
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPullCmd(d), "--check")
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("err = %v", err)
	}
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); !strings.Contains(got, "Old title") {
		t.Fatal("--check must not write")
	}
	if _, err := os.Stat(filepath.Join(wd, "openforms", "workflows", "triage.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("--check must not create files")
	}
	want := "would update forms/contact.yaml\nwould create workflows/triage.yaml\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPullCheckUpToDate(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	tio.out.Reset()
	if err := execute(NewPullCmd(d), "--check"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if tio.out.String() != "Up to date.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

// Review Focus #5: local-only files survive a pull.
func TestPullKeepsLocalOnlyFiles(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/draft.yaml", "slug: draft\n")
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	if readRel(t, wd, "openforms/forms/draft.yaml") != "slug: draft\n" {
		t.Fatal("local-only file modified")
	}
	if !strings.Contains(tio.out.String(), "note: forms/draft.yaml exists locally but not on the server (left untouched)") {
		t.Fatalf("out = %q", tio.out.String())
	}
}

// Review Focus #5: alternate extensions block the pull instead of creating a duplicate slug.
func TestPullRefusesAlternateExtension(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.json", "{}")
	fr := &fakeRemote{export: remoteBundle()}
	d, _ := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPullCmd(d))
	if err == nil || !strings.Contains(err.Error(), "forms/contact.json exists; pull only manages .yaml files, rename it to forms/contact.yaml") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(wd, "openforms", "forms", "contact.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("pull wrote files despite the conflict")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run Pull -v`
Expected: FAIL: `undefined: NewPullCmd`.

- [ ] **Step 3: Write the implementation**

`internal/cli/pull.go`:
```go
package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
)

type pullChange struct {
	Rel    string // e.g. "forms/contact.yaml"
	Path   string // absolute
	Data   []byte
	Status string // created | updated | unchanged
}

func NewPullCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	var check bool
	cmd := &cobra.Command{
		Use:          "pull",
		Short:        "Write the server's definitions to local files as canonical YAML",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			dir := d.resolveDir(rf.dir, pc)
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			b, err := d.NewClient(server, key).Export(cmd.Context())
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			changes, localOnly, err := planPull(dir, b)
			if err != nil {
				return err
			}
			if check {
				drift := 0
				for _, c := range changes {
					if c.Status != "unchanged" {
						drift++
						fmt.Fprintf(d.Out, "would %s %s\n", strings.TrimSuffix(c.Status, "d"), c.Rel)
					}
				}
				if drift > 0 {
					return ErrDrift
				}
				fmt.Fprintln(d.Out, "Up to date.")
				return nil
			}
			counts := map[string]int{}
			for _, c := range changes {
				counts[c.Status]++
				if c.Status == "unchanged" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(c.Path, c.Data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(d.Out, "%s %s\n", c.Status, c.Rel)
			}
			for _, rel := range localOnly {
				fmt.Fprintf(d.Out, "note: %s exists locally but not on the server (left untouched)\n", rel)
			}
			fmt.Fprintf(d.Out, "Pulled %d form(s), %d workflow(s): %d created, %d updated, %d unchanged.\n",
				len(b.Forms), len(b.Workflows), counts["created"], counts["updated"], counts["unchanged"])
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	cmd.Flags().BoolVar(&check, "check", false, "write nothing; exit 1 if any file would change (CI drift check)")
	return cmd
}

// planPull computes what pull would write, without touching the filesystem.
func planPull(dir string, b client.Bundle) ([]pullChange, []string, error) {
	var changes []pullChange
	remote := map[string]bool{}
	add := func(kind, slug string, v any) error {
		remote[kind+"/"+slug] = true
		for _, ext := range []string{".yml", ".json"} {
			if _, err := os.Stat(filepath.Join(dir, kind, slug+ext)); err == nil {
				return fmt.Errorf("%s/%s%s exists; pull only manages .yaml files, rename it to %s/%s.yaml", kind, slug, ext, kind, slug)
			}
		}
		data, err := CanonicalYAML(v)
		if err != nil {
			return err
		}
		c := pullChange{Rel: kind + "/" + slug + ".yaml", Path: filepath.Join(dir, kind, slug+".yaml"), Data: data, Status: "created"}
		existing, err := os.ReadFile(c.Path)
		switch {
		case err == nil && bytes.Equal(existing, data):
			c.Status = "unchanged"
		case err == nil:
			c.Status = "updated"
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		changes = append(changes, c)
		return nil
	}
	for _, f := range b.Forms {
		if err := add("forms", f.Slug, f); err != nil {
			return nil, nil, err
		}
	}
	for _, w := range b.Workflows {
		if err := add("workflows", w.Slug, w); err != nil {
			return nil, nil, err
		}
	}
	var localOnly []string
	for _, kind := range []string{"forms", "workflows"} {
		files, err := definitionFiles(filepath.Join(dir, kind))
		if err != nil {
			return nil, nil, err
		}
		for _, p := range files {
			base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if !remote[kind+"/"+base] {
				localOnly = append(localOnly, kind+"/"+filepath.Base(p))
			}
		}
	}
	return changes, localOnly, nil
}
```

(`strings.TrimSuffix(status, "d")` turns `created`/`updated` into `create`/`update` for the `would …` lines.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run Pull -v`
Expected: PASS for `TestPullWritesCanonicalFiles`, `TestPullIsStableAndSkipsUnchanged`, `TestPullCheckDetectsDriftWithoutWriting`, `TestPullCheckUpToDate`, `TestPullKeepsLocalOnlyFiles`, `TestPullRefusesAlternateExtension`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/pull.go internal/cli/pull_test.go
git commit -m "feat(cli): add pull with canonical output and --check drift detection"
```

---

### Task 9: Register commands and add the end-to-end round-trip test

**Files:**
- Create: `internal/cli/commands.go`
- Modify: `internal/cli/root.go` (one line, next to the `AddCommand` call(s) Plan 01 added for `serve`/`migrate`)
- Test: `internal/cli/e2e_test.go`

**Interfaces:**
- Consumes: every constructor from Tasks 5–8; `testutil.NewEnv`, `(*testutil.Env).APIKey` (Plan 01); `httpapi.NewRouter`, `httpapi.Deps` (Plans 01/03); `definitions.NewStore` (Plan 03); `submissions.NewService` (Plan 03); `auth.RoleAdmin`.
- Produces: `func Commands(d Deps) []*cobra.Command`. It returns, in order, `admin`, `init`, `validate`, `push`, `pull` and `diff`, and Plan 09 appends `seed` here.

- [ ] **Step 1: Write the failing end-to-end test**

`internal/cli/e2e_test.go`:
```go
package cli

import (
	"errors"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

func TestCommandsRegistersAll(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil)
	var names []string
	for _, c := range Commands(d) {
		names = append(names, c.Name())
	}
	if got := strings.Join(names, ","); got != "admin,init,validate,push,pull,diff" {
		t.Fatalf("commands = %s", got)
	}
}

func TestPushPullRoundTripAgainstRealServer(t *testing.T) {
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	subs := submissions.NewService(env.Pool, defs)
	srv := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Config: env.Config, Auth: env.Auth, Defs: defs, Subs: subs, OrgID: env.OrgID,
	}))
	t.Cleanup(srv.Close)
	key := env.APIKey(t, auth.RoleAdmin)

	run := func(workDir, apiKey string, args ...string) (string, string, error) {
		d, tio := newTestDeps(workDir, map[string]string{"OPENFORMS_URL": srv.URL, "OPENFORMS_API_KEY": apiKey}, nil)
		d.NewClient = func(server, k string) Remote { return client.New(server, k) }
		root := &cobra.Command{Use: "openforms", SilenceUsage: true}
		root.AddCommand(Commands(d)...)
		err := execute(root, args...)
		return tio.out.String(), tio.err.String(), err
	}
	mustMatch := func(out, pattern string) {
		t.Helper()
		if !regexp.MustCompile(pattern).MatchString(out) {
			t.Fatalf("output does not match %q:\n%s", pattern, out)
		}
	}

	projA := t.TempDir()
	if _, _, err := run(projA, key, "init"); err != nil {
		t.Fatal(err)
	}

	// Review Focus #4: wrong key gives an actionable message.
	if _, _, err := run(projA, "ofk_wrong", "push"); err == nil || !strings.Contains(err.Error(), "check --api-key or OPENFORMS_API_KEY") {
		t.Fatalf("wrong key err = %v", err)
	}

	out, errOut, err := run(projA, key, "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, errOut)
	}
	mustMatch(out, `(?m)^workflow\s+contact-triage\s+1\s+created$`)
	mustMatch(out, `(?m)^form\s+contact\s+1\s+created$`)

	out, _, err = run(projA, key, "push")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+1\s+unchanged$`)

	if out, _, err = run(projA, key, "diff"); err != nil || out != "No differences.\n" {
		t.Fatalf("diff: %q %v", out, err)
	}
	if out, _, err = run(projA, key, "pull", "--check"); err != nil || out != "Up to date.\n" {
		t.Fatalf("pull --check: %q %v", out, err)
	}

	projB := t.TempDir()
	if _, _, err := run(projB, key, "pull"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"} {
		if readRel(t, projA, rel) != readRel(t, projB, rel) {
			t.Fatalf("%s differs between pushed and pulled copies", rel)
		}
	}

	// Change the form in A: diff, dry-run, push, then B is out of date.
	formA := readRel(t, projA, "openforms/forms/contact.yaml")
	put(t, projA, "openforms/forms/contact.yaml", strings.Replace(formA, "title: Contact us", "title: Talk to us", 1))
	if out, _, err = run(projA, key, "diff"); err != nil || out != "~ form contact\n" {
		t.Fatalf("diff after edit: %q %v", out, err)
	}
	out, _, err = run(projA, key, "push", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+2\s+updated$`)
	out, _, err = run(projA, key, "push")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+2\s+updated$`)

	if _, _, err := run(projB, key, "pull", "--check"); !errors.Is(err, ErrDrift) {
		t.Fatalf("pull --check in stale project: err = %v", err)
	}
	if _, _, err := run(projB, key, "pull"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readRel(t, projB, "openforms/forms/contact.yaml"), "title: Talk to us") {
		t.Fatal("pull did not bring the new title")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run 'CommandsRegistersAll|PushPullRoundTrip' -v`
Expected: FAIL: `undefined: Commands`.

- [ ] **Step 3: Write `commands.go`**

`internal/cli/commands.go`:
```go
package cli

import "github.com/spf13/cobra"

// Commands returns the developer-facing commands added by Plan 05 (Plan 09 appends seed).
func Commands(d Deps) []*cobra.Command {
	return []*cobra.Command{
		NewAdminCmd(d),
		NewInitCmd(d),
		NewValidateCmd(d),
		NewPushCmd(d),
		NewPullCmd(d),
		NewDiffCmd(d),
	}
}
```

- [ ] **Step 4: Register in `root.go`**

Run: `grep -n "AddCommand" internal/cli/root.go`
Expected: the line(s) where Plan 01 registers `serve` and `migrate` on the root command (for example `root.AddCommand(newServeCmd(), newMigrateCmd())`).

Directly after that line, add this line, using the same receiver variable the existing call uses:
```go
	root.AddCommand(Commands(DefaultDeps())...)
```

Then confirm the binary exposes the commands:

Run: `go run ./cmd/openforms --help`
Expected: the "Available Commands" list includes `admin`, `diff`, `init`, `migrate`, `pull`, `push`, `serve`, `validate`.

- [ ] **Step 5: Run the whole package and verify it passes**

Run: `docker compose up -d postgres && go test ./internal/client/ ./internal/cli/ -v`
Expected: PASS for every test in both packages, including `TestCommandsRegistersAll` and `TestPushPullRoundTripAgainstRealServer`.

Then run the full suite to confirm nothing else broke:

Run: `go test ./...`
Expected: `ok` for every package.

- [ ] **Step 6: Smoke-test the real binary (manual)**

```bash
docker compose up -d postgres
export OPENFORMS_DATABASE_URL='postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable'
go run ./cmd/openforms serve &            # leave running
KEY=$(go run ./cmd/openforms admin create-api-key --name cli --roles admin | grep '^ofk_')
tmp=$(mktemp -d) && cd "$tmp"
go run github.com/openforms/openforms/cmd/openforms init   # or the built binary
OPENFORMS_API_KEY=$KEY openforms validate && OPENFORMS_API_KEY=$KEY openforms push && OPENFORMS_API_KEY=$KEY openforms pull --check
```
Expected: `OK: 1 form(s), 1 workflow(s) valid`, a push table showing both items `created`, and `Up to date.`. Stop the server afterwards.

- [ ] **Step 7: Commit**

```bash
git add internal/cli/commands.go internal/cli/root.go internal/cli/e2e_test.go
git commit -m "feat(cli): register developer commands and add push/pull round-trip test"
```

---

## Self-review notes (for the executor)

- **Spec coverage (§8):** `admin create-user` and `create-api-key` are Task 5. `init` and `validate` are Task 6. `push` and `diff` are Task 7. `pull` and `--check` are Task 8. §6.14 `internal/client` is Task 1. Config resolution is Task 4. `seed --demo` belongs to Plan 09, which should append `NewSeedCmd(d)` to `Commands`.
- **Decisions this plan makes where the spec was silent:**
  1. `push`/`validate` require cross-references (`form.workflow`) to resolve **within the local directory**, since `ValidateBundle(…, nil)` is offline.
  2. `diff` always exits 0.
  3. `pull` writes only `.yaml` and refuses when a `.yml`/`.json` file for the same slug exists.
  4. `init` writes canonicalised templates, so a fresh project is already in pull format.
  5. `openforms.yaml` is parsed strictly, and unknown keys are an error.
  6. Admin commands run migrations and ensure the default org before acting.

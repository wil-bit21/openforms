# openforms Plan 02 — Definition Language Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents) once every earlier sequential task is done.

**Goal:** Ship the openforms definition language: JSON Schemas for forms and workflows, shared Go/TS conformance fixtures, and the pure-Go `internal/definition` package (types, YAML/JSON parsing, semantic validation, canonical hashing, visibility, submission and workflow-field validation).

**Architecture:** `schemas/` is a root Go package that embeds the two JSON Schemas (draft 2020-12) and the conformance fixtures; the same files are consumed by the TypeScript SDK (Plan 06) and the editors (Plan 08). `internal/definition` parses YAML or JSON (`sigs.k8s.io/yaml`), validates structure against the embedded schemas (`santhosh-tekuri/jsonschema/v6`), then applies the semantic rules of spec §5.3, returning `*ValidationError` with JSON-pointer-ish problem paths. Submission logic (visibility, cleaning, type checks) lives in one evaluator so `Visible` and `ValidateSubmission` can never disagree. No database, no HTTP.

**Tech Stack:** Go ≥ 1.24, `github.com/santhosh-tekuri/jsonschema/v6`, `sigs.k8s.io/yaml`, `golang.org/x/text` (English messages for schema errors), standard library (`net/mail`, `net/url`, `regexp`, `time`, `crypto/sha256`).

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md` — binding sections: §5 (all), §6.3, §6.4. Roadmap: `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md` (this plan is **Wave 1, Lane B**, running in parallel with Plan 01).

## Global Constraints

- Go ≥ 1.24, module `github.com/openforms/openforms` at repo root.
- JSON Schema: `github.com/santhosh-tekuri/jsonschema/v6` (Go), `ajv` (TS editors) — so schemas must be draft 2020-12 and use only portable keywords.
- YAML: `sigs.k8s.io/yaml` (Go), `yaml` npm package (TS).
- All JSON over the wire is **camelCase**.
- The Go server is **authoritative** for validation. TS validation exists only for UX and must pass the shared conformance fixtures (§5.4).
- Slug regex `^[a-z0-9][a-z0-9-]{0,62}$`; field/state/transition key regex `^[a-zA-Z][a-zA-Z0-9_]{0,63}$`.
- Form field types: `text|textarea|email|number|select|multiselect|checkbox|date|url`. Workflow field types: `text|textarea|number|select|checkbox|date`.
- State colors: `gray|blue|green|yellow|red|purple`. Action types: `webhook|email|assign`.
- Problem paths for submissions: `data.<key>`; for workflow fields: `fields.<key>`.
- `Canonical(def)` = JSON with sorted keys, no insignificant whitespace; `hash` = sha256 hex of canonical bytes.
- Commit after every task; conventional commit prefixes (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).

### Contract clarifications made by this plan (binding for Plans 03–08)

0. `Problem`/`ValidationError` live only in `internal/definition/problem.go`, a file Plan 01 also creates (identical content). Plan 02 creates it only if absent; all other definition helpers go in separate files.

1. Spec §6.4 lists `FieldTextarea = "textarea"` etc. inside a const block; in Go those would be **untyped**. This plan declares **every** `FieldType` and `ActionType` constant with its explicit type (`FieldTextarea FieldType = "textarea"`).
2. **String answers are trimmed** (`strings.TrimSpace`) before validation and stored trimmed; a string that is empty after trimming counts as "not provided". TS must use `.trim()`.
3. **Lengths are counted in Unicode code points** (`utf8.RuneCountInString`; TS: `[...s].length`).
4. **`pattern` is anchored** (whole-value match, like HTML `pattern`): compiled as `^(?:<pattern>)$`. An uncompilable pattern is ignored at submission time (it is rejected when the definition is validated).
5. Visibility uses the **cleaned value** of the controlling field: an invalid controller value counts as absent. `equals` on an absent controller → hidden; `notEquals` on an absent (but visible) controller → visible; `in` on an absent controller → hidden. For `multiselect` controllers, `equals x` = contains x, `notEquals x` = does not contain x, `in [..]` = contains any of them. A hidden controller always hides its dependants.
6. `ValidateWorkflowFields` validates a **patch**: it returns a normalized map with the same keys; a key whose value is `null` or a blank string maps to `nil`, meaning "unset — delete this key when merging". Callers (Plan 04 engine) merge with: `nil` → `delete(fields, k)`, otherwise `fields[k] = v`.
7. A problem at the document root has `Path == ""`.
8. Both schemas have `$id`s `https://openforms.dev/schemas/form.schema.json` and `https://openforms.dev/schemas/workflow.schema.json`, and accept an optional top-level `"$schema"` string (for editor tooling) that is dropped when decoding into Go structs. TS/ajv must use `ajv/dist/2020`.
9. Fixture error comparisons are **order-insensitive** (compare sorted path lists).

## Review Focus

1. **YAML 1.1 implicit booleans** — an author writes `value: yes` for an option: the parser must report a problem at `fields[0].options[0].value` (not silently coerce to `true` and fail later). Test added in Task 7 (`TestParseFormYAMLImplicitBoolean`).
2. **Duplicate keys in a YAML document** — `title:` twice must be rejected with a root-level problem, not "last one wins". Test added in Task 7 (`TestParseFormRejectsDuplicateKeys`).
3. **HTML-form-style payloads** — a `multiselect` sent as a single string (`"go"`) or a `checkbox` sent as `"true"` must yield a field problem, never a panic or a silent coercion. Pinned by fixtures (Task 2) and exercised in Task 6 (`TestCheckValue`: "multi string", "checkbox string") and Task 8 (fixture runner).
4. **Non-ASCII answers vs length limits** — `"👍👍"` with `maxLength: 2` must pass in Go *and* TS (code points, not UTF-16 units or bytes). Pinned by fixture "lengths are counted in code points" (Task 2), run in Task 8; unit case in Task 6.
5. **A definition that would crash at submission time** — an invalid `pattern` or a `showIf` that references itself/a later field must be rejected with an exact path when validating the definition, and the submission path must never panic on such a form. Tests in Task 4 (`invalid pattern`, `self reference showIf`, `showIf later field`) and Task 6 (`bad pattern ignored`).

---

## File Structure

| File | Responsibility |
|---|---|
| `schemas/schemas.go` | package `schemas`: embeds schemas (`FS`) and fixtures (`Fixtures`) |
| `schemas/form.schema.json` | structural schema for form definitions |
| `schemas/workflow.schema.json` | structural schema for workflow definitions |
| `schemas/fixtures/visibility.json` | Go/TS conformance cases for visibility |
| `schemas/fixtures/submission.json` | Go/TS conformance cases for cleaning + validation |
| `schemas/fixtures/README.md` | rules the fixtures encode (for TS implementers) |
| `schemas/schemas_test.go` | schemas compile; spec examples valid; fixture forms valid |
| `internal/definition/doc.go` | package documentation |
| `internal/definition/types.go` | all definition types + `Workflow` lookup methods |
| `internal/definition/problem.go` | `Problem`, `ValidationError` — **identical to Plan 01's file** (created only if absent; on merge keep one copy) |
| `internal/definition/problems_helpers.go` | internal `problems` collector, `joinPath` |
| `internal/definition/common.go` | name regexes, shared option validation |
| `internal/definition/canonical.go` | `Canonical` |
| `internal/definition/validate_form.go` | `ValidateForm` |
| `internal/definition/validate_workflow.go` | `ValidateWorkflow`, action rules |
| `internal/definition/values.go` | `checkValue`: per-type answer validation/normalization |
| `internal/definition/parse.go` | `ParseForm`, `ParseWorkflow`, schema loading, schema-error mapping |
| `internal/definition/bundle.go` | `ValidateBundle` |
| `internal/definition/submission.go` | evaluator, `Visible`, `ValidateSubmission` |
| `internal/definition/workflow_fields.go` | `ValidateWorkflowFields` |
| `internal/definition/*_test.go` | tests (internal package `definition`, so unexported helpers are testable) |

Task order and parallelism:

```
Task 1 (types/errors/common) → Task 2 (schemas + fixtures)
   → Parallel group A: Task 3 (canonical) | Task 4 (form rules) | Task 5 (workflow rules) | Task 6 (values)
   → Parallel group B: Task 7 (parse + bundle) | Task 8 (visibility + submission) | Task 9 (workflow fields)
   → Final verification
```

---

### Task 1: Module bootstrap, types, errors and shared helpers

**Files:**
- Create (only if missing): `go.mod`
- Create: `internal/definition/doc.go`
- Create: `internal/definition/types.go`
- Create **only if absent**: `internal/definition/problem.go` (Plan 01 creates the identical file; on the Wave 1 merge keep one copy)
- Create: `internal/definition/problems_helpers.go`
- Create: `internal/definition/common.go`
- Create: `internal/definition/helpers_test.go`
- Test: `internal/definition/types_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces (spec §6.4, exact names): `FieldType` + constants `FieldText, FieldTextarea, FieldEmail, FieldNumber, FieldSelect, FieldMultiselect, FieldCheckbox, FieldDate, FieldURL`; `Option`, `Validation`, `Condition`, `Field`, `FormSettings`, `Form`, `State`, `WorkflowField`, `Guard`, `ActionType` + `ActionWebhook, ActionEmail, ActionAssign`; `Action`, `Transition`, `Workflow`; methods `(Workflow) State(key) (State, bool)`, `(Workflow) Transition(key) (Transition, bool)`, `(Workflow) Field(key) (WorkflowField, bool)`; `Problem{Path, Message}`, `ValidationError{Problems}` with `Error() string`.
- Note: `Problem` and `ValidationError` are owned jointly with Plan 01 (httpapi needs them). They live **only** in `problem.go`; never define them in any other file.
- Produces (package-internal, used by Tasks 3–9): `type problems []Problem` with `add(path, msg string)`, `merge(prefix string, err error)`, `err() error`; `joinPath(prefix, path string) string`; `slugRe`, `keyRe` (`*regexp.Regexp`); `validateOptions(ps *problems, path string, allowed bool, notAllowedMsg string, opts []Option)`.
- Produces (test helpers, `helpers_test.go`): `problemPaths(t, err) []string` (sorted), `hasPath(paths, want) bool`, `intPtr(int) *int`, `floatPtr(float64) *float64`.

- [ ] **Step 1: Make sure the module exists**

This plan runs in parallel with Plan 01, in a worktree branch the orchestrator created. If you are executing standalone and `git rev-parse --is-inside-work-tree` fails, run `git init -b main` first.

Run:
```bash
test -f go.mod || go mod init github.com/openforms/openforms
head -1 go.mod
go version
```
Expected: `module github.com/openforms/openforms` and a Go version ≥ 1.24. If `go.mod` was just created, also run `go mod edit -go=1.24`.

- [ ] **Step 2: Write the failing tests**

`internal/definition/helpers_test.go`:
```go
package definition

import (
	"errors"
	"sort"
	"testing"
)

// problemPaths returns the sorted problem paths of a *ValidationError (nil for nil error).
func problemPaths(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	out := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		out[i] = p.Path
	}
	sort.Strings(out)
	return out
}

func hasPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

func intPtr(v int) *int             { return &v }
func floatPtr(v float64) *float64   { return &v }
```

`internal/definition/types_test.go`:
```go
package definition

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormJSONShape(t *testing.T) {
	f := Form{
		Slug:     "contact",
		Title:    "Contact",
		Settings: FormSettings{Public: true},
		Fields:   []Field{{Key: "name", Type: FieldText, Label: "Name", Required: true}},
	}
	b, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"slug":"contact","title":"Contact","settings":{"public":true},"fields":[{"key":"name","type":"text","label":"Name","required":true}]}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}

func TestConstantsAreTyped(t *testing.T) {
	var ft FieldType = FieldTextarea // compile-time check: typed constant
	var at ActionType = ActionAssign
	if ft != "textarea" || at != "assign" {
		t.Fatalf("unexpected constant values %q %q", ft, at)
	}
}

func TestWorkflowLookups(t *testing.T) {
	w := Workflow{
		States:      []State{{Key: "new", Label: "New"}, {Key: "done", Label: "Done", Terminal: true}},
		Fields:      []WorkflowField{{Key: "score", Type: FieldNumber, Label: "Score"}},
		Transitions: []Transition{{Key: "finish", Label: "Finish", From: []string{"new"}, To: "done"}},
	}
	if s, ok := w.State("done"); !ok || !s.Terminal {
		t.Fatalf("State(done) = %+v, %v", s, ok)
	}
	if _, ok := w.State("missing"); ok {
		t.Fatal("State(missing) should not be found")
	}
	if tr, ok := w.Transition("finish"); !ok || tr.To != "done" {
		t.Fatalf("Transition(finish) = %+v, %v", tr, ok)
	}
	if _, ok := w.Transition("nope"); ok {
		t.Fatal("Transition(nope) should not be found")
	}
	if f, ok := w.Field("score"); !ok || f.Type != FieldNumber {
		t.Fatalf("Field(score) = %+v, %v", f, ok)
	}
	if _, ok := w.Field("nope"); ok {
		t.Fatal("Field(nope) should not be found")
	}
}

// Asserts content, not exact wording, so it holds for Plan 01's copy of problem.go too.
func TestValidationErrorMessage(t *testing.T) {
	err := &ValidationError{Problems: []Problem{
		{Path: "", Message: "document is empty"},
		{Path: "fields[0].key", Message: "is required"},
	}}
	msg := err.Error()
	for _, want := range []string{"document is empty", "fields[0].key", "is required"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("Error() = %q, missing %q", msg, want)
		}
	}
	if (&ValidationError{}).Error() == "" {
		t.Fatal("empty ValidationError must still have a message")
	}
}

func TestProblemsMergeAndJoin(t *testing.T) {
	var ps problems
	ps.merge("forms[0]", &ValidationError{Problems: []Problem{{Path: "slug", Message: "bad"}, {Path: "", Message: "root"}}})
	ps.merge("forms[1]", nil)
	got := problemPaths(t, ps.err())
	want := []string{"forms[0]", "forms[0].slug"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
	var empty problems
	if empty.err() != nil {
		t.Fatal("empty problems must return nil error")
	}
}

func TestValidateOptions(t *testing.T) {
	var ps problems
	validateOptions(&ps, "fields[0]", true, "", nil)
	validateOptions(&ps, "fields[1]", false, "options are only allowed on select and multiselect fields", []Option{{Value: "a", Label: "A"}})
	validateOptions(&ps, "fields[2]", true, "", []Option{
		{Value: "a", Label: "A"},
		{Value: "a", Label: "Again"},
		{Value: " b", Label: "B"},
		{Value: "", Label: ""},
	})
	got := problemPaths(t, ps.err())
	for _, want := range []string{
		"fields[0].options",
		"fields[1].options",
		"fields[2].options[1].value",
		"fields[2].options[2].value",
		"fields[2].options[3].value",
		"fields[2].options[3].label",
	} {
		if !hasPath(got, want) {
			t.Errorf("missing problem %q in %v", want, got)
		}
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/definition/ -v`
Expected: FAIL — build errors such as `undefined: Form`, `undefined: ValidationError`, `undefined: validateOptions`.

- [ ] **Step 4: Write the implementation**

`internal/definition/doc.go`:
```go
// Package definition implements the openforms definition language: form and
// workflow types, YAML/JSON parsing against the embedded JSON Schemas,
// semantic validation (spec §5.3), canonical hashing (§5.5) and submission
// semantics (§5.4). It has no database or HTTP dependencies.
package definition
```

`internal/definition/types.go`:
```go
package definition

// FieldType is the type of a form field or workflow field.
type FieldType string

const (
	FieldText        FieldType = "text"
	FieldTextarea    FieldType = "textarea"
	FieldEmail       FieldType = "email"
	FieldNumber      FieldType = "number"
	FieldSelect      FieldType = "select"
	FieldMultiselect FieldType = "multiselect"
	FieldCheckbox    FieldType = "checkbox"
	FieldDate        FieldType = "date"
	FieldURL         FieldType = "url"
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Validation struct {
	MinLength *int     `json:"minLength,omitempty"`
	MaxLength *int     `json:"maxLength,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
}

// Condition controls field visibility. Exactly one of Equals, NotEquals, In is set.
type Condition struct {
	Field     string `json:"field"`
	Equals    any    `json:"equals,omitempty"`
	NotEquals any    `json:"notEquals,omitempty"`
	In        []any  `json:"in,omitempty"`
}

type Field struct {
	Key         string      `json:"key"`
	Type        FieldType   `json:"type"`
	Label       string      `json:"label"`
	Help        string      `json:"help,omitempty"`
	Placeholder string      `json:"placeholder,omitempty"`
	Required    bool        `json:"required,omitempty"`
	Options     []Option    `json:"options,omitempty"`
	Validation  *Validation `json:"validation,omitempty"`
	ShowIf      *Condition  `json:"showIf,omitempty"`
}

type FormSettings struct {
	Public              bool   `json:"public"`
	SubmitLabel         string `json:"submitLabel,omitempty"`
	ConfirmationMessage string `json:"confirmationMessage,omitempty"`
}

type Form struct {
	Slug        string       `json:"slug"`
	Title       string       `json:"title"`
	Description string       `json:"description,omitempty"`
	Workflow    string       `json:"workflow,omitempty"`
	Settings    FormSettings `json:"settings"`
	Fields      []Field      `json:"fields"`
}

type State struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Color    string `json:"color,omitempty"`
	Terminal bool   `json:"terminal,omitempty"`
}

type WorkflowField struct {
	Key     string    `json:"key"`
	Type    FieldType `json:"type"`
	Label   string    `json:"label"`
	Options []Option  `json:"options,omitempty"`
}

type Guard struct {
	Roles         []string `json:"roles,omitempty"`
	RequireFields []string `json:"requireFields,omitempty"`
}

// ActionType is the kind of side effect a transition or submission triggers.
type ActionType string

const (
	ActionWebhook ActionType = "webhook"
	ActionEmail   ActionType = "email"
	ActionAssign  ActionType = "assign"
)

type Action struct {
	Type    ActionType `json:"type"`
	URL     string     `json:"url,omitempty"`
	To      string     `json:"to,omitempty"`
	Subject string     `json:"subject,omitempty"`
	Body    string     `json:"body,omitempty"`
	User    string     `json:"user,omitempty"`
	Role    string     `json:"role,omitempty"`
}

type Transition struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	From    []string `json:"from"`
	To      string   `json:"to"`
	Guard   Guard    `json:"guard"`
	Actions []Action `json:"actions,omitempty"`
}

type Workflow struct {
	Slug        string          `json:"slug"`
	Title       string          `json:"title"`
	Initial     string          `json:"initial"`
	States      []State         `json:"states"`
	Fields      []WorkflowField `json:"fields,omitempty"`
	OnSubmit    []Action        `json:"onSubmit,omitempty"`
	Transitions []Transition    `json:"transitions"`
}

// State returns the state with the given key.
func (w Workflow) State(key string) (State, bool) {
	for _, s := range w.States {
		if s.Key == key {
			return s, true
		}
	}
	return State{}, false
}

// Transition returns the transition with the given key.
func (w Workflow) Transition(key string) (Transition, bool) {
	for _, t := range w.Transitions {
		if t.Key == key {
			return t, true
		}
	}
	return Transition{}, false
}

// Field returns the workflow field with the given key.
func (w Workflow) Field(key string) (WorkflowField, bool) {
	for _, f := range w.Fields {
		if f.Key == key {
			return f, true
		}
	}
	return WorkflowField{}, false
}
```

`internal/definition/problem.go` — create **only if it does not exist yet**. It is identical to the file Plan 01 creates (spec §6.4); if Plan 01's copy is already present, leave it untouched. On the Wave 1 merge keep exactly one copy (if the two differ only in the `Error()` wording, keep Plan 01's):
```go
package definition

import "strings"

// Problem is one validation failure. Path is JSON-pointer-ish, e.g.
// "fields[2].showIf.field" or "data.email"; "" means the whole document.
type Problem struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError is returned for any invalid definition or submission.
type ValidationError struct {
	Problems []Problem
}

func (e *ValidationError) Error() string {
	if len(e.Problems) == 0 {
		return "validation failed"
	}
	parts := make([]string, len(e.Problems))
	for i, p := range e.Problems {
		if p.Path == "" {
			parts[i] = p.Message
		} else {
			parts[i] = p.Path + ": " + p.Message
		}
	}
	return "validation failed: " + strings.Join(parts, "; ")
}
```

`internal/definition/problems_helpers.go`:
```go
package definition

// problems collects Problems while validating.
type problems []Problem

func (ps *problems) add(path, msg string) {
	*ps = append(*ps, Problem{Path: path, Message: msg})
}

// merge appends the problems of err (if any) with prefix prepended to their paths.
func (ps *problems) merge(prefix string, err error) {
	if err == nil {
		return
	}
	ve, ok := err.(*ValidationError)
	if !ok {
		ps.add(prefix, err.Error())
		return
	}
	for _, p := range ve.Problems {
		ps.add(joinPath(prefix, p.Path), p.Message)
	}
}

func (ps problems) err() error {
	if len(ps) == 0 {
		return nil
	}
	return &ValidationError{Problems: []Problem(ps)}
}

func joinPath(prefix, path string) string {
	switch {
	case prefix == "":
		return path
	case path == "":
		return prefix
	default:
		return prefix + "." + path
	}
}
```

`internal/definition/common.go`:
```go
package definition

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	keyRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
)

const (
	slugRule = "must match ^[a-z0-9][a-z0-9-]{0,62}$"
	keyRule  = "must match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$"
)

// validateOptions checks an options list. allowed reports whether the owning
// field type takes options; when it does, at least one option is required.
func validateOptions(ps *problems, path string, allowed bool, notAllowedMsg string, opts []Option) {
	if !allowed {
		if len(opts) > 0 {
			ps.add(path+".options", notAllowedMsg)
		}
		return
	}
	if len(opts) == 0 {
		ps.add(path+".options", "at least one option is required")
		return
	}
	seen := make(map[string]bool, len(opts))
	for j, o := range opts {
		op := fmt.Sprintf("%s.options[%d]", path, j)
		switch {
		case o.Value == "":
			ps.add(op+".value", "is required")
		case strings.TrimSpace(o.Value) != o.Value:
			ps.add(op+".value", "must not start or end with whitespace")
		case seen[o.Value]:
			ps.add(op+".value", fmt.Sprintf("duplicate option value %q", o.Value))
		}
		seen[o.Value] = true
		if strings.TrimSpace(o.Label) == "" {
			ps.add(op+".label", "is required")
		}
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/definition/ -v`
Expected: PASS for `TestFormJSONShape`, `TestConstantsAreTyped`, `TestWorkflowLookups`, `TestValidationErrorMessage`, `TestProblemsMergeAndJoin`, `TestValidateOptions`.

- [ ] **Step 6: Commit**

```bash
gofmt -l internal/definition   # Expected: no output
git add go.mod internal/definition
git commit -m "feat(definition): add definition types, validation errors and option rules"
```

---

### Task 2: JSON Schemas, conformance fixtures and the `schemas` package

**Files:**
- Create: `schemas/schemas.go`
- Create: `schemas/form.schema.json`
- Create: `schemas/workflow.schema.json`
- Create: `schemas/fixtures/visibility.json`
- Create: `schemas/fixtures/submission.json`
- Create: `schemas/fixtures/README.md`
- Test: `schemas/schemas_test.go`
- Modify: `go.mod`, `go.sum` (adds `github.com/santhosh-tekuri/jsonschema/v6`)

**Interfaces:**
- Consumes: nothing from Task 1 (independent package), but runs after it so `go.mod` exists.
- Produces: `schemas.FS embed.FS` (files `form.schema.json`, `workflow.schema.json`), `schemas.Fixtures embed.FS` (files `fixtures/visibility.json`, `fixtures/submission.json`). Schema `$id`s: `https://openforms.dev/schemas/form.schema.json`, `https://openforms.dev/schemas/workflow.schema.json`. Fixture shapes: visibility `[{name, form, data, visible}]`; submission `[{name, form, data, clean|null, errorPaths}]`. Consumed by Tasks 7–8 and by Plans 06/08 (TS).

- [ ] **Step 1: Add the JSON Schema dependency**

Run: `go get github.com/santhosh-tekuri/jsonschema/v6@latest`
Expected: `go: added github.com/santhosh-tekuri/jsonschema/v6 v6.x.y`.

- [ ] **Step 2: Write the failing test**

`schemas/schemas_test.go`:
```go
package schemas_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/openforms/openforms/schemas"
)

const (
	formID     = "https://openforms.dev/schemas/form.schema.json"
	workflowID = "https://openforms.dev/schemas/workflow.schema.json"
)

func compile(t *testing.T) (form, workflow *jsonschema.Schema) {
	t.Helper()
	c := jsonschema.NewCompiler()
	for id, file := range map[string]string{formID: "form.schema.json", workflowID: "workflow.schema.json"} {
		b, err := schemas.FS.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("%s is not valid JSON: %v", file, err)
		}
		if err := c.AddResource(id, doc); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	if form, err = c.Compile(formID); err != nil {
		t.Fatalf("compile form schema: %v", err)
	}
	if workflow, err = c.Compile(workflowID); err != nil {
		t.Fatalf("compile workflow schema: %v", err)
	}
	return form, workflow
}

func instance(t *testing.T, s string) any {
	t.Helper()
	v, err := jsonschema.UnmarshalJSON(strings.NewReader(s))
	if err != nil {
		t.Fatalf("bad test JSON: %v", err)
	}
	return v
}

const specForm = `{
  "$schema": "https://openforms.dev/schemas/form.schema.json",
  "slug": "job-application", "title": "Job application", "description": "Apply to join the team.",
  "workflow": "hiring",
  "settings": {"public": true, "submitLabel": "Send application", "confirmationMessage": "Thanks! We'll be in touch."},
  "fields": [
    {"key": "name", "type": "text", "label": "Full name", "required": true, "placeholder": "Ada Lovelace",
     "help": "As on your passport", "validation": {"minLength": 2, "maxLength": 100}},
    {"key": "role", "type": "select", "label": "Role", "required": true,
     "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
    {"key": "portfolio", "type": "url", "label": "Portfolio URL", "showIf": {"field": "role", "equals": "designer"}}
  ]
}`

const specWorkflow = `{
  "slug": "hiring", "title": "Hiring pipeline", "initial": "new",
  "states": [
    {"key": "new", "label": "New", "color": "gray"},
    {"key": "screening", "label": "Screening", "color": "blue"},
    {"key": "interview", "label": "Interview", "color": "purple"},
    {"key": "hired", "label": "Hired", "color": "green", "terminal": true},
    {"key": "rejected", "label": "Rejected", "color": "red", "terminal": true}
  ],
  "fields": [
    {"key": "score", "type": "number", "label": "Score"},
    {"key": "rejectionReason", "type": "textarea", "label": "Rejection reason"}
  ],
  "onSubmit": [
    {"type": "assign", "role": "reviewer"},
    {"type": "email", "to": "{{submission.data.email}}", "subject": "We got your application", "body": "Hi {{submission.data.name}}"}
  ],
  "transitions": [
    {"key": "screen", "label": "Start screening", "from": ["new"], "to": "screening", "guard": {"roles": ["reviewer"]}},
    {"key": "invite", "label": "Invite to interview", "from": ["screening"], "to": "interview",
     "guard": {"roles": ["reviewer"], "requireFields": ["score"]},
     "actions": [{"type": "webhook", "url": "https://example.com/hooks/interview"}]},
    {"key": "hire", "label": "Hire", "from": ["interview"], "to": "hired", "guard": {"roles": ["hiring-manager"]}},
    {"key": "reject", "label": "Reject", "from": ["new", "screening", "interview"], "to": "rejected",
     "guard": {"roles": ["reviewer", "hiring-manager"], "requireFields": ["rejectionReason"]},
     "actions": [{"type": "email", "to": "{{submission.data.email}}", "subject": "Your application", "body": "{{submission.fields.rejectionReason}}"}]}
  ]
}`

func TestSpecExamplesAreValid(t *testing.T) {
	form, workflow := compile(t)
	if err := form.Validate(instance(t, specForm)); err != nil {
		t.Fatalf("spec form example rejected: %v", err)
	}
	if err := workflow.Validate(instance(t, specWorkflow)); err != nil {
		t.Fatalf("spec workflow example rejected: %v", err)
	}
}

func TestSchemasRejectStructuralErrors(t *testing.T) {
	form, workflow := compile(t)
	bad := []struct {
		name   string
		schema *jsonschema.Schema
		doc    string
	}{
		{"form unknown property", form, `{"slug":"a","title":"A","fields":[{"key":"x","type":"text","label":"X","colour":"red"}]}`},
		{"form missing fields", form, `{"slug":"a","title":"A"}`},
		{"form empty fields", form, `{"slug":"a","title":"A","fields":[]}`},
		{"form bad slug", form, `{"slug":"A B","title":"A","fields":[{"key":"x","type":"text","label":"X"}]}`},
		{"form bad field type", form, `{"slug":"a","title":"A","fields":[{"key":"x","type":"color","label":"X"}]}`},
		{"form option value not string", form, `{"slug":"a","title":"A","fields":[{"key":"x","type":"select","label":"X","options":[{"value":true,"label":"Yes"}]}]}`},
		{"workflow bad color", workflow, `{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A","color":"orange"}]}`},
		{"workflow email field type", workflow, `{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"fields":[{"key":"e","type":"email","label":"E"}]}`},
		{"workflow bad action type", workflow, `{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"onSubmit":[{"type":"sms"}]}`},
		{"workflow transition without from", workflow, `{"slug":"w","title":"W","initial":"a","states":[{"key":"a","label":"A"}],"transitions":[{"key":"t","label":"T","from":[],"to":"a"}]}`},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.schema.Validate(instance(t, tc.doc)); err == nil {
				t.Fatal("expected schema validation error")
			}
		})
	}
}

type fixtureCase struct {
	Name       string          `json:"name"`
	Form       json.RawMessage `json:"form"`
	Data       json.RawMessage `json:"data"`
	Visible    map[string]bool `json:"visible"`
	Clean      json.RawMessage `json:"clean"`
	ErrorPaths []string        `json:"errorPaths"`
}

func loadFixture(t *testing.T, name string) []fixtureCase {
	t.Helper()
	b, err := schemas.Fixtures.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var cases []fixtureCase
	if err := json.Unmarshal(b, &cases); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s has no cases", name)
	}
	return cases
}

func TestFixturesAreWellFormed(t *testing.T) {
	form, _ := compile(t)
	names := map[string]bool{}
	for _, file := range []string{"visibility.json", "submission.json"} {
		for _, c := range loadFixture(t, file) {
			if c.Name == "" || names[file+c.Name] {
				t.Errorf("%s: empty or duplicate case name %q", file, c.Name)
			}
			names[file+c.Name] = true
			if err := form.Validate(instance(t, string(c.Form))); err != nil {
				t.Errorf("%s / %s: fixture form violates schema: %v", file, c.Name, err)
			}
			if len(c.Data) == 0 || c.Data[0] != '{' {
				t.Errorf("%s / %s: data must be an object", file, c.Name)
			}
			switch file {
			case "visibility.json":
				if len(c.Visible) == 0 {
					t.Errorf("%s / %s: visible map required", file, c.Name)
				}
			case "submission.json":
				isNull := string(c.Clean) == "null" || len(c.Clean) == 0
				if isNull == (len(c.ErrorPaths) == 0) {
					t.Errorf("%s / %s: exactly one of clean (non-null) or errorPaths (non-empty) required", file, c.Name)
				}
			}
		}
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./schemas/ -v`
Expected: FAIL — `no non-test Go files in .../schemas` or `undefined: schemas.FS`.

- [ ] **Step 4: Write `schemas/schemas.go`**

```go
// Package schemas holds the JSON Schemas for openforms definitions and the
// conformance fixtures shared by the Go server and the TypeScript SDK.
package schemas

import "embed"

// FS contains form.schema.json and workflow.schema.json.
//
//go:embed form.schema.json workflow.schema.json
var FS embed.FS

// Fixtures contains fixtures/visibility.json and fixtures/submission.json.
//
//go:embed fixtures/*.json
var Fixtures embed.FS
```

- [ ] **Step 5: Write `schemas/form.schema.json`**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://openforms.dev/schemas/form.schema.json",
  "title": "openforms form definition",
  "type": "object",
  "additionalProperties": false,
  "required": ["slug", "title", "fields"],
  "properties": {
    "$schema": { "type": "string" },
    "slug": { "$ref": "#/$defs/slug" },
    "title": { "type": "string", "minLength": 1, "maxLength": 200 },
    "description": { "type": "string", "maxLength": 2000 },
    "workflow": { "$ref": "#/$defs/slug" },
    "settings": { "$ref": "#/$defs/settings" },
    "fields": {
      "type": "array",
      "minItems": 1,
      "maxItems": 200,
      "items": { "$ref": "#/$defs/field" }
    }
  },
  "$defs": {
    "slug": { "type": "string", "pattern": "^[a-z0-9][a-z0-9-]{0,62}$" },
    "key": { "type": "string", "pattern": "^[a-zA-Z][a-zA-Z0-9_]{0,63}$" },
    "fieldType": {
      "enum": ["text", "textarea", "email", "number", "select", "multiselect", "checkbox", "date", "url"]
    },
    "settings": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "public": { "type": "boolean" },
        "submitLabel": { "type": "string", "maxLength": 60 },
        "confirmationMessage": { "type": "string", "maxLength": 2000 }
      }
    },
    "option": {
      "type": "object",
      "additionalProperties": false,
      "required": ["value", "label"],
      "properties": {
        "value": { "type": "string", "minLength": 1, "maxLength": 200 },
        "label": { "type": "string", "minLength": 1, "maxLength": 200 }
      }
    },
    "validation": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "minLength": { "type": "integer", "minimum": 0 },
        "maxLength": { "type": "integer", "minimum": 0 },
        "pattern": { "type": "string", "minLength": 1, "maxLength": 500 },
        "min": { "type": "number" },
        "max": { "type": "number" }
      }
    },
    "condition": {
      "type": "object",
      "additionalProperties": false,
      "required": ["field"],
      "properties": {
        "field": { "$ref": "#/$defs/key" },
        "equals": {},
        "notEquals": {},
        "in": { "type": "array" }
      }
    },
    "field": {
      "type": "object",
      "additionalProperties": false,
      "required": ["key", "type", "label"],
      "properties": {
        "key": { "$ref": "#/$defs/key" },
        "type": { "$ref": "#/$defs/fieldType" },
        "label": { "type": "string", "minLength": 1, "maxLength": 500 },
        "help": { "type": "string", "maxLength": 1000 },
        "placeholder": { "type": "string", "maxLength": 200 },
        "required": { "type": "boolean" },
        "options": { "type": "array", "maxItems": 500, "items": { "$ref": "#/$defs/option" } },
        "validation": { "$ref": "#/$defs/validation" },
        "showIf": { "$ref": "#/$defs/condition" }
      }
    }
  }
}
```

- [ ] **Step 6: Write `schemas/workflow.schema.json`**

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://openforms.dev/schemas/workflow.schema.json",
  "title": "openforms workflow definition",
  "type": "object",
  "additionalProperties": false,
  "required": ["slug", "title", "initial", "states"],
  "properties": {
    "$schema": { "type": "string" },
    "slug": { "$ref": "#/$defs/slug" },
    "title": { "type": "string", "minLength": 1, "maxLength": 200 },
    "initial": { "$ref": "#/$defs/key" },
    "states": {
      "type": "array",
      "minItems": 1,
      "maxItems": 100,
      "items": { "$ref": "#/$defs/state" }
    },
    "fields": {
      "type": "array",
      "maxItems": 100,
      "items": { "$ref": "#/$defs/workflowField" }
    },
    "onSubmit": {
      "type": "array",
      "maxItems": 20,
      "items": { "$ref": "#/$defs/action" }
    },
    "transitions": {
      "type": "array",
      "maxItems": 200,
      "items": { "$ref": "#/$defs/transition" }
    }
  },
  "$defs": {
    "slug": { "type": "string", "pattern": "^[a-z0-9][a-z0-9-]{0,62}$" },
    "key": { "type": "string", "pattern": "^[a-zA-Z][a-zA-Z0-9_]{0,63}$" },
    "option": {
      "type": "object",
      "additionalProperties": false,
      "required": ["value", "label"],
      "properties": {
        "value": { "type": "string", "minLength": 1, "maxLength": 200 },
        "label": { "type": "string", "minLength": 1, "maxLength": 200 }
      }
    },
    "state": {
      "type": "object",
      "additionalProperties": false,
      "required": ["key", "label"],
      "properties": {
        "key": { "$ref": "#/$defs/key" },
        "label": { "type": "string", "minLength": 1, "maxLength": 100 },
        "color": { "enum": ["gray", "blue", "green", "yellow", "red", "purple"] },
        "terminal": { "type": "boolean" }
      }
    },
    "workflowField": {
      "type": "object",
      "additionalProperties": false,
      "required": ["key", "type", "label"],
      "properties": {
        "key": { "$ref": "#/$defs/key" },
        "type": { "enum": ["text", "textarea", "number", "select", "checkbox", "date"] },
        "label": { "type": "string", "minLength": 1, "maxLength": 200 },
        "options": { "type": "array", "maxItems": 500, "items": { "$ref": "#/$defs/option" } }
      }
    },
    "guard": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "roles": { "type": "array", "items": { "type": "string", "minLength": 1, "maxLength": 64 } },
        "requireFields": { "type": "array", "items": { "$ref": "#/$defs/key" } }
      }
    },
    "action": {
      "type": "object",
      "additionalProperties": false,
      "required": ["type"],
      "properties": {
        "type": { "enum": ["webhook", "email", "assign"] },
        "url": { "type": "string", "maxLength": 2000 },
        "to": { "type": "string", "maxLength": 500 },
        "subject": { "type": "string", "maxLength": 500 },
        "body": { "type": "string", "maxLength": 20000 },
        "user": { "type": "string", "maxLength": 320 },
        "role": { "type": "string", "maxLength": 64 }
      }
    },
    "transition": {
      "type": "object",
      "additionalProperties": false,
      "required": ["key", "label", "from", "to"],
      "properties": {
        "key": { "$ref": "#/$defs/key" },
        "label": { "type": "string", "minLength": 1, "maxLength": 100 },
        "from": { "type": "array", "minItems": 1, "items": { "$ref": "#/$defs/key" } },
        "to": { "$ref": "#/$defs/key" },
        "guard": { "$ref": "#/$defs/guard" },
        "actions": { "type": "array", "maxItems": 20, "items": { "$ref": "#/$defs/action" } }
      }
    }
  }
}
```

- [ ] **Step 7: Write `schemas/fixtures/visibility.json`**

```json
[
  {
    "name": "no conditions: every field visible",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "a", "type": "text", "label": "A"},
      {"key": "b", "type": "number", "label": "B"}
    ]},
    "data": {},
    "visible": {"a": true, "b": true}
  },
  {
    "name": "equals: controller matches",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "designer"},
    "visible": {"role": true, "portfolio": true}
  },
  {
    "name": "equals: controller differs",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "engineer"},
    "visible": {"role": true, "portfolio": false}
  },
  {
    "name": "equals: controller not provided hides",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {},
    "visible": {"role": true, "portfolio": false}
  },
  {
    "name": "notEquals: controller not provided shows",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "notes", "type": "text", "label": "Notes", "showIf": {"field": "role", "notEquals": "designer"}}
    ]},
    "data": {},
    "visible": {"role": true, "notes": true}
  },
  {
    "name": "notEquals: controller matches hides",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "notes", "type": "text", "label": "Notes", "showIf": {"field": "role", "notEquals": "designer"}}
    ]},
    "data": {"role": "designer"},
    "visible": {"role": true, "notes": false}
  },
  {
    "name": "in: controller is a member",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "team", "type": "text", "label": "Team", "showIf": {"field": "role", "in": ["engineer", "designer"]}}
    ]},
    "data": {"role": "engineer"},
    "visible": {"role": true, "team": true}
  },
  {
    "name": "in: controller is not a member",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}, {"value": "manager", "label": "Manager"}]},
      {"key": "team", "type": "text", "label": "Team", "showIf": {"field": "role", "in": ["engineer", "designer"]}}
    ]},
    "data": {"role": "manager"},
    "visible": {"role": true, "team": false}
  },
  {
    "name": "chain: hidden controller hides dependants even if raw data matches",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "hasPet", "type": "checkbox", "label": "Has pet"},
      {"key": "petKind", "type": "select", "label": "Pet kind", "options": [{"value": "dog", "label": "Dog"}, {"value": "cat", "label": "Cat"}], "showIf": {"field": "hasPet", "equals": true}},
      {"key": "petName", "type": "text", "label": "Pet name", "showIf": {"field": "petKind", "equals": "dog"}}
    ]},
    "data": {"hasPet": false, "petKind": "dog", "petName": "Rex"},
    "visible": {"hasPet": true, "petKind": false, "petName": false}
  },
  {
    "name": "chain: all conditions hold",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "hasPet", "type": "checkbox", "label": "Has pet"},
      {"key": "petKind", "type": "select", "label": "Pet kind", "options": [{"value": "dog", "label": "Dog"}, {"value": "cat", "label": "Cat"}], "showIf": {"field": "hasPet", "equals": true}},
      {"key": "petName", "type": "text", "label": "Pet name", "showIf": {"field": "petKind", "equals": "dog"}}
    ]},
    "data": {"hasPet": true, "petKind": "dog"},
    "visible": {"hasPet": true, "petKind": true, "petName": true}
  },
  {
    "name": "chain: middle condition fails",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "hasPet", "type": "checkbox", "label": "Has pet"},
      {"key": "petKind", "type": "select", "label": "Pet kind", "options": [{"value": "dog", "label": "Dog"}, {"value": "cat", "label": "Cat"}], "showIf": {"field": "hasPet", "equals": true}},
      {"key": "petName", "type": "text", "label": "Pet name", "showIf": {"field": "petKind", "equals": "dog"}}
    ]},
    "data": {"hasPet": true, "petKind": "cat"},
    "visible": {"hasPet": true, "petKind": true, "petName": false}
  },
  {
    "name": "invalid controller value counts as absent",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "hasPet", "type": "checkbox", "label": "Has pet"},
      {"key": "petKind", "type": "select", "label": "Pet kind", "options": [{"value": "dog", "label": "Dog"}, {"value": "cat", "label": "Cat"}], "showIf": {"field": "hasPet", "equals": true}}
    ]},
    "data": {"hasPet": "yes"},
    "visible": {"hasPet": true, "petKind": false}
  },
  {
    "name": "hidden controller beats notEquals-on-absent",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "hasPet", "type": "checkbox", "label": "Has pet"},
      {"key": "petKind", "type": "select", "label": "Pet kind", "options": [{"value": "dog", "label": "Dog"}, {"value": "cat", "label": "Cat"}], "showIf": {"field": "hasPet", "equals": true}},
      {"key": "notDog", "type": "text", "label": "Not a dog", "showIf": {"field": "petKind", "notEquals": "dog"}}
    ]},
    "data": {"hasPet": false},
    "visible": {"hasPet": true, "petKind": false, "notDog": false}
  },
  {
    "name": "multiselect: equals means contains",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}, {"value": "sql", "label": "SQL"}]},
      {"key": "goYears", "type": "number", "label": "Years of Go", "showIf": {"field": "skills", "equals": "go"}},
      {"key": "dbNotes", "type": "text", "label": "Database notes", "showIf": {"field": "skills", "in": ["sql"]}},
      {"key": "noGo", "type": "text", "label": "Why not Go?", "showIf": {"field": "skills", "notEquals": "go"}}
    ]},
    "data": {"skills": ["ts", "go"]},
    "visible": {"skills": true, "goYears": true, "dbNotes": false, "noGo": false}
  },
  {
    "name": "multiselect: in means contains any; notEquals means does not contain",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}, {"value": "sql", "label": "SQL"}]},
      {"key": "goYears", "type": "number", "label": "Years of Go", "showIf": {"field": "skills", "equals": "go"}},
      {"key": "dbNotes", "type": "text", "label": "Database notes", "showIf": {"field": "skills", "in": ["sql"]}},
      {"key": "noGo", "type": "text", "label": "Why not Go?", "showIf": {"field": "skills", "notEquals": "go"}}
    ]},
    "data": {"skills": ["sql"]},
    "visible": {"skills": true, "goYears": false, "dbNotes": true, "noGo": true}
  },
  {
    "name": "multiselect: empty selection counts as absent",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}, {"value": "sql", "label": "SQL"}]},
      {"key": "goYears", "type": "number", "label": "Years of Go", "showIf": {"field": "skills", "equals": "go"}},
      {"key": "dbNotes", "type": "text", "label": "Database notes", "showIf": {"field": "skills", "in": ["sql"]}},
      {"key": "noGo", "type": "text", "label": "Why not Go?", "showIf": {"field": "skills", "notEquals": "go"}}
    ]},
    "data": {"skills": []},
    "visible": {"skills": true, "goYears": false, "dbNotes": false, "noGo": true}
  },
  {
    "name": "number equals uses numeric JSON equality",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "age", "type": "number", "label": "Age"},
      {"key": "adultNote", "type": "text", "label": "Note", "showIf": {"field": "age", "equals": 18}}
    ]},
    "data": {"age": 18},
    "visible": {"age": true, "adultNote": true}
  },
  {
    "name": "number sent as string is invalid so condition fails",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "age", "type": "number", "label": "Age"},
      {"key": "adultNote", "type": "text", "label": "Note", "showIf": {"field": "age", "equals": 18}}
    ]},
    "data": {"age": "18"},
    "visible": {"age": true, "adultNote": false}
  },
  {
    "name": "checkbox equals false",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "agree", "type": "checkbox", "label": "I agree"},
      {"key": "why", "type": "textarea", "label": "Why not?", "showIf": {"field": "agree", "equals": false}}
    ]},
    "data": {"agree": false},
    "visible": {"agree": true, "why": true}
  },
  {
    "name": "checkbox not provided does not equal false",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "agree", "type": "checkbox", "label": "I agree"},
      {"key": "why", "type": "textarea", "label": "Why not?", "showIf": {"field": "agree", "equals": false}}
    ]},
    "data": {},
    "visible": {"agree": true, "why": false}
  },
  {
    "name": "controller string value is trimmed before comparison",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "nickname", "type": "text", "label": "Nickname"},
      {"key": "greeting", "type": "text", "label": "Greeting", "showIf": {"field": "nickname", "equals": "ada"}}
    ]},
    "data": {"nickname": "  ada  "},
    "visible": {"nickname": true, "greeting": true}
  }
]
```

- [ ] **Step 8: Write `schemas/fixtures/submission.json`**

```json
[
  {
    "name": "valid text answer",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": "Ada"},
    "clean": {"name": "Ada"},
    "errorPaths": []
  },
  {
    "name": "required field missing",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "required field empty string",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": ""},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "required field whitespace only",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": "   "},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "strings are trimmed",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": "  Ada Lovelace  "},
    "clean": {"name": "Ada Lovelace"},
    "errorPaths": []
  },
  {
    "name": "unknown keys are dropped",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": "Ada", "admin": true},
    "clean": {"name": "Ada"},
    "errorPaths": []
  },
  {
    "name": "null optional value is dropped",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "nickname", "type": "text", "label": "Nickname"}
    ]},
    "data": {"nickname": null},
    "clean": {},
    "errorPaths": []
  },
  {
    "name": "text must be a string",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": 5},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "object for text field is rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true}
    ]},
    "data": {"name": {"first": "Ada"}},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "minLength violated",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "validation": {"minLength": 2, "maxLength": 5}}
    ]},
    "data": {"name": "A"},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "maxLength violated",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "validation": {"minLength": 2, "maxLength": 5}}
    ]},
    "data": {"name": "Adalove"},
    "clean": null,
    "errorPaths": ["data.name"]
  },
  {
    "name": "lengths are counted in code points",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "tag", "type": "text", "label": "Tag", "validation": {"maxLength": 2}}
    ]},
    "data": {"tag": "👍👍"},
    "clean": {"tag": "👍👍"},
    "errorPaths": []
  },
  {
    "name": "pattern must match the whole value",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "code", "type": "text", "label": "Code", "validation": {"pattern": "[A-Z]{3}"}}
    ]},
    "data": {"code": "ABCD"},
    "clean": null,
    "errorPaths": ["data.code"]
  },
  {
    "name": "pattern matches",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "code", "type": "text", "label": "Code", "validation": {"pattern": "[A-Z]{3}"}}
    ]},
    "data": {"code": "ABC"},
    "clean": {"code": "ABC"},
    "errorPaths": []
  },
  {
    "name": "email valid",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "email", "type": "email", "label": "Email"}
    ]},
    "data": {"email": "ada@example.com"},
    "clean": {"email": "ada@example.com"},
    "errorPaths": []
  },
  {
    "name": "email with display name rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "email", "type": "email", "label": "Email"}
    ]},
    "data": {"email": "Ada <ada@example.com>"},
    "clean": null,
    "errorPaths": ["data.email"]
  },
  {
    "name": "email without at-sign rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "email", "type": "email", "label": "Email"}
    ]},
    "data": {"email": "ada.example.com"},
    "clean": null,
    "errorPaths": ["data.email"]
  },
  {
    "name": "url valid",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "site", "type": "url", "label": "Site"}
    ]},
    "data": {"site": "https://example.com/portfolio?x=1"},
    "clean": {"site": "https://example.com/portfolio?x=1"},
    "errorPaths": []
  },
  {
    "name": "url with ftp scheme rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "site", "type": "url", "label": "Site"}
    ]},
    "data": {"site": "ftp://example.com/file"},
    "clean": null,
    "errorPaths": ["data.site"]
  },
  {
    "name": "url without scheme rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "site", "type": "url", "label": "Site"}
    ]},
    "data": {"site": "example.com"},
    "clean": null,
    "errorPaths": ["data.site"]
  },
  {
    "name": "date leap day valid",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "start", "type": "date", "label": "Start"}
    ]},
    "data": {"start": "2024-02-29"},
    "clean": {"start": "2024-02-29"},
    "errorPaths": []
  },
  {
    "name": "date that does not exist rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "start", "type": "date", "label": "Start"}
    ]},
    "data": {"start": "2023-02-29"},
    "clean": null,
    "errorPaths": ["data.start"]
  },
  {
    "name": "date in wrong format rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "start", "type": "date", "label": "Start"}
    ]},
    "data": {"start": "29/02/2024"},
    "clean": null,
    "errorPaths": ["data.start"]
  },
  {
    "name": "number within bounds",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "rating", "type": "number", "label": "Rating", "validation": {"min": 0, "max": 10}}
    ]},
    "data": {"rating": 3.5},
    "clean": {"rating": 3.5},
    "errorPaths": []
  },
  {
    "name": "number bounds are inclusive",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "low", "type": "number", "label": "Low", "validation": {"min": 0, "max": 10}},
      {"key": "high", "type": "number", "label": "High", "validation": {"min": 0, "max": 10}}
    ]},
    "data": {"low": 0, "high": 10},
    "clean": {"low": 0, "high": 10},
    "errorPaths": []
  },
  {
    "name": "number above max rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "rating", "type": "number", "label": "Rating", "validation": {"min": 0, "max": 10}}
    ]},
    "data": {"rating": 10.5},
    "clean": null,
    "errorPaths": ["data.rating"]
  },
  {
    "name": "number sent as string rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "rating", "type": "number", "label": "Rating"}
    ]},
    "data": {"rating": "5"},
    "clean": null,
    "errorPaths": ["data.rating"]
  },
  {
    "name": "integer number preserved",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "age", "type": "number", "label": "Age"}
    ]},
    "data": {"age": 42},
    "clean": {"age": 42},
    "errorPaths": []
  },
  {
    "name": "select valid option",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]}
    ]},
    "data": {"role": "designer"},
    "clean": {"role": "designer"},
    "errorPaths": []
  },
  {
    "name": "select unknown option rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]}
    ]},
    "data": {"role": "ceo"},
    "clean": null,
    "errorPaths": ["data.role"]
  },
  {
    "name": "multiselect valid keeps order",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": ["ts", "go"]},
    "clean": {"skills": ["ts", "go"]},
    "errorPaths": []
  },
  {
    "name": "multiselect unknown option rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": ["go", "rust"]},
    "clean": null,
    "errorPaths": ["data.skills"]
  },
  {
    "name": "multiselect duplicates rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": ["go", "go"]},
    "clean": null,
    "errorPaths": ["data.skills"]
  },
  {
    "name": "multiselect sent as single string rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": "go"},
    "clean": null,
    "errorPaths": ["data.skills"]
  },
  {
    "name": "multiselect empty and required rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "required": true, "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": []},
    "clean": null,
    "errorPaths": ["data.skills"]
  },
  {
    "name": "multiselect empty and optional dropped",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "skills", "type": "multiselect", "label": "Skills", "options": [{"value": "go", "label": "Go"}, {"value": "ts", "label": "TypeScript"}]}
    ]},
    "data": {"skills": []},
    "clean": {},
    "errorPaths": []
  },
  {
    "name": "required checkbox must be true",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "agree", "type": "checkbox", "label": "I agree", "required": true}
    ]},
    "data": {"agree": false},
    "clean": null,
    "errorPaths": ["data.agree"]
  },
  {
    "name": "required checkbox true accepted",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "agree", "type": "checkbox", "label": "I agree", "required": true}
    ]},
    "data": {"agree": true},
    "clean": {"agree": true},
    "errorPaths": []
  },
  {
    "name": "optional checkbox false is kept",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "newsletter", "type": "checkbox", "label": "Newsletter"}
    ]},
    "data": {"newsletter": false},
    "clean": {"newsletter": false},
    "errorPaths": []
  },
  {
    "name": "checkbox sent as string rejected",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "newsletter", "type": "checkbox", "label": "Newsletter"}
    ]},
    "data": {"newsletter": "true"},
    "clean": null,
    "errorPaths": ["data.newsletter"]
  },
  {
    "name": "hidden field value is dropped",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "required": true, "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "required": true, "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "engineer", "portfolio": "https://ada.dev"},
    "clean": {"role": "engineer"},
    "errorPaths": []
  },
  {
    "name": "hidden required field is not enforced",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "required": true, "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "required": true, "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "engineer"},
    "clean": {"role": "engineer"},
    "errorPaths": []
  },
  {
    "name": "visible required field is enforced",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "required": true, "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "required": true, "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "designer"},
    "clean": null,
    "errorPaths": ["data.portfolio"]
  },
  {
    "name": "hidden field with invalid value is ignored",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "required": true, "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "required": true, "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "engineer", "portfolio": "not a url"},
    "clean": {"role": "engineer"},
    "errorPaths": []
  },
  {
    "name": "invalid controller hides its dependant",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "role", "type": "select", "label": "Role", "required": true, "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
      {"key": "portfolio", "type": "url", "label": "Portfolio", "required": true, "showIf": {"field": "role", "equals": "designer"}}
    ]},
    "data": {"role": "ceo", "portfolio": "https://ada.dev"},
    "clean": null,
    "errorPaths": ["data.role"]
  },
  {
    "name": "every invalid field is reported",
    "form": {"slug": "t", "title": "T", "settings": {"public": true}, "fields": [
      {"key": "name", "type": "text", "label": "Name", "required": true},
      {"key": "email", "type": "email", "label": "Email", "required": true},
      {"key": "age", "type": "number", "label": "Age"}
    ]},
    "data": {"email": "x", "age": "old"},
    "clean": null,
    "errorPaths": ["data.name", "data.email", "data.age"]
  }
]
```

- [ ] **Step 9: Write `schemas/fixtures/README.md`**

````markdown
# Conformance fixtures

Shared by the Go server (`internal/definition`) and the TypeScript SDK
(`@openforms/sdk`). Both test suites iterate **every** case; the Go server is
authoritative, and TS must agree on every case.

- `visibility.json`: `[{ name, form, data, visible }]`. `visible` lists every field key.
- `submission.json`: `[{ name, form, data, clean, errorPaths }]`. When `clean` is
  non-null the submission is valid and the cleaned data must deep-equal it.
  Otherwise the sorted problem paths must equal the sorted `errorPaths`
  (comparison is order-insensitive).

Rules the fixtures encode (spec §5.4 plus Plan 02 clarifications):

1. String answers are trimmed; blank after trimming = not provided.
2. `null`, blank strings and empty arrays are "not provided"; `false` is a provided checkbox value.
3. Lengths count Unicode code points (`[...s].length` in JS).
4. `pattern` is anchored: the whole value must match `^(?:pattern)$`.
5. `email` is a bare address `local@domain` (no display name, no angle brackets, no spaces).
6. `url` is absolute `http`/`https` with a host. `date` is a real calendar date in `YYYY-MM-DD`.
7. Visibility uses the *cleaned* controller value; invalid = absent. `equals`/`in` on
   absent → hidden; `notEquals` on absent → visible; a hidden controller hides its
   dependants. For `multiselect` controllers: `equals` = contains, `notEquals` = does
   not contain, `in` = contains any.
8. Hidden fields and unknown keys are dropped; hidden fields are never validated.
````

- [ ] **Step 10: Run the tests to verify they pass**

Run: `go test ./schemas/ -v`
Expected: PASS for `TestSpecExamplesAreValid`, `TestSchemasRejectStructuralErrors` (10 subtests), `TestFixturesAreWellFormed`.

- [ ] **Step 11: Commit**

```bash
git add go.mod go.sum schemas
git commit -m "feat(schemas): add form/workflow JSON Schemas and conformance fixtures"
```

---

### Task 3: Canonical JSON and hashing

**Parallel group A** (with Tasks 4, 5, 6 — disjoint files).

**Files:**
- Create: `internal/definition/canonical.go`
- Test: `internal/definition/canonical_test.go`

**Interfaces:**
- Consumes: `Form`, `Field`, `FieldText` (Task 1).
- Produces: `func Canonical(v any) (canonical []byte, hash string, err error)` — sorted keys, compact, HTML characters not escaped, no trailing newline; hash = lowercase hex sha256 of the canonical bytes. Used by Task 7 tests and Plan 03 (`definitions.Store`).

- [ ] **Step 1: Write the failing test**

`internal/definition/canonical_test.go`:
```go
package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestCanonicalSortsKeysAndCompacts(t *testing.T) {
	in := map[string]any{"b": 1, "a": map[string]any{"d": true, "c": "<x> & y"}}
	got, hash, err := Canonical(in)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":{"c":"<x> & y","d":true},"b":1}`
	if string(got) != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	sum := sha256.Sum256([]byte(want))
	if hash != hex.EncodeToString(sum[:]) {
		t.Fatalf("hash %s does not match sha256 of canonical bytes", hash)
	}
}

func TestCanonicalStructAndMapAgree(t *testing.T) {
	f := Form{Slug: "contact", Title: "Contact", Fields: []Field{{Key: "name", Type: FieldText, Label: "Name"}}}
	m := map[string]any{
		"title":    "Contact",
		"fields":   []any{map[string]any{"label": "Name", "type": "text", "key": "name"}},
		"slug":     "contact",
		"settings": map[string]any{"public": false},
	}
	b1, h1, err := Canonical(f)
	if err != nil {
		t.Fatal(err)
	}
	b2, h2, err := Canonical(m)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash mismatch:\n%s\n%s", b1, b2)
	}
}

func TestCanonicalPreservesNumbers(t *testing.T) {
	got, _, err := Canonical(map[string]any{"n": 0.1, "i": 42, "big": 12345678901234567})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"big":12345678901234567,"i":42,"n":0.1}`
	if string(got) != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestCanonicalRejectsUnmarshalable(t *testing.T) {
	if _, _, err := Canonical(map[string]any{"f": func() {}}); err == nil {
		t.Fatal("expected error for a func value")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run TestCanonical -v`
Expected: FAIL — `undefined: Canonical`.

- [ ] **Step 3: Write the implementation**

`internal/definition/canonical.go`:
```go
package definition

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// Canonical returns v as canonical JSON (object keys sorted, no insignificant
// whitespace, HTML characters unescaped) and the lowercase hex sha256 of it.
// Two definitions that differ only in key order or formatting hash equally.
func Canonical(v any) ([]byte, string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, "", fmt.Errorf("canonical: marshal: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep numbers exactly as marshalled
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, "", fmt.Errorf("canonical: decode: %w", err)
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(generic); err != nil { // maps encode with sorted keys
		return nil, "", fmt.Errorf("canonical: encode: %w", err)
	}
	out := bytes.TrimRight(buf.Bytes(), "\n")
	sum := sha256.Sum256(out)
	return out, hex.EncodeToString(sum[:]), nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run TestCanonical -v`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add internal/definition/canonical.go internal/definition/canonical_test.go
git commit -m "feat(definition): add canonical JSON encoding and hashing"
```

---

### Task 4: Form semantic validation

**Parallel group A** (with Tasks 3, 5, 6 — disjoint files).

**Files:**
- Create: `internal/definition/validate_form.go`
- Test: `internal/definition/validate_form_test.go`

**Interfaces:**
- Consumes: types, `problems`, `slugRe`, `keyRe`, `slugRule`, `keyRule`, `validateOptions` (Task 1); test helpers (Task 1).
- Produces: `func ValidateForm(f Form) error` — `nil` or `*ValidationError` listing **all** problems. Used by Task 7 (`ParseForm`, `ValidateBundle`), Task 8 (fixture sanity), Plans 03/08.

- [ ] **Step 1: Write the failing test**

`internal/definition/validate_form_test.go`:
```go
package definition

import "testing"

func baseForm() Form {
	return Form{
		Slug:     "job-application",
		Title:    "Job application",
		Workflow: "hiring",
		Settings: FormSettings{Public: true},
		Fields: []Field{
			{Key: "name", Type: FieldText, Label: "Full name", Required: true,
				Validation: &Validation{MinLength: intPtr(2), MaxLength: intPtr(100)}},
			{Key: "role", Type: FieldSelect, Label: "Role", Required: true,
				Options: []Option{{Value: "engineer", Label: "Engineer"}, {Value: "designer", Label: "Designer"}}},
			{Key: "portfolio", Type: FieldURL, Label: "Portfolio",
				ShowIf: &Condition{Field: "role", Equals: "designer"}},
			{Key: "years", Type: FieldNumber, Label: "Years of experience",
				Validation: &Validation{Min: floatPtr(0), Max: floatPtr(50)}},
		},
	}
}

func TestValidateFormAcceptsBase(t *testing.T) {
	if err := ValidateForm(baseForm()); err != nil {
		t.Fatalf("base form should be valid: %v", err)
	}
}

func TestValidateFormRules(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(f *Form)
		wantPath string
	}{
		{"bad slug", func(f *Form) { f.Slug = "Bad Slug" }, "slug"},
		{"empty title", func(f *Form) { f.Title = "  " }, "title"},
		{"bad workflow slug", func(f *Form) { f.Workflow = "Hiring!" }, "workflow"},
		{"no fields", func(f *Form) { f.Fields = nil }, "fields"},
		{"bad key", func(f *Form) { f.Fields[0].Key = "1name" }, "fields[0].key"},
		{"duplicate key", func(f *Form) { f.Fields[1].Key = "name" }, "fields[1].key"},
		{"empty label", func(f *Form) { f.Fields[0].Label = "" }, "fields[0].label"},
		{"unknown type", func(f *Form) { f.Fields[0].Type = "color" }, "fields[0].type"},
		{"select without options", func(f *Form) { f.Fields[1].Options = nil }, "fields[1].options"},
		{"duplicate option value", func(f *Form) { f.Fields[1].Options[1].Value = "engineer" }, "fields[1].options[1].value"},
		{"option value whitespace", func(f *Form) { f.Fields[1].Options[0].Value = " engineer" }, "fields[1].options[0].value"},
		{"options on text", func(f *Form) { f.Fields[0].Options = []Option{{Value: "a", Label: "A"}} }, "fields[0].options"},
		{"negative minLength", func(f *Form) { f.Fields[0].Validation.MinLength = intPtr(-1) }, "fields[0].validation.minLength"},
		{"minLength above maxLength", func(f *Form) { f.Fields[0].Validation.MinLength = intPtr(10); f.Fields[0].Validation.MaxLength = intPtr(5) }, "fields[0].validation.maxLength"},
		{"min above max", func(f *Form) { f.Fields[3].Validation.Min = floatPtr(60) }, "fields[3].validation.max"},
		{"minLength on number", func(f *Form) { f.Fields[3].Validation.MinLength = intPtr(1) }, "fields[3].validation.minLength"},
		{"pattern on number", func(f *Form) { f.Fields[3].Validation.Pattern = "x" }, "fields[3].validation.pattern"},
		{"min on text", func(f *Form) { f.Fields[0].Validation.Min = floatPtr(1) }, "fields[0].validation.min"},
		{"invalid pattern", func(f *Form) { f.Fields[0].Validation.Pattern = "(" }, "fields[0].validation.pattern"},
		{"self reference showIf", func(f *Form) { f.Fields[2].ShowIf.Field = "portfolio" }, "fields[2].showIf.field"},
		{"showIf later field", func(f *Form) { f.Fields[0].ShowIf = &Condition{Field: "role", Equals: "x"} }, "fields[0].showIf.field"},
		{"showIf unknown field", func(f *Form) { f.Fields[2].ShowIf.Field = "nope" }, "fields[2].showIf.field"},
		{"showIf without operator", func(f *Form) { f.Fields[2].ShowIf = &Condition{Field: "role"} }, "fields[2].showIf"},
		{"showIf two operators", func(f *Form) { f.Fields[2].ShowIf = &Condition{Field: "role", Equals: "designer", NotEquals: "engineer"} }, "fields[2].showIf"},
		{"showIf empty in", func(f *Form) { f.Fields[2].ShowIf = &Condition{Field: "role", In: []any{}} }, "fields[2].showIf.in"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := baseForm()
			tc.mutate(&f)
			paths := problemPaths(t, ValidateForm(f))
			if !hasPath(paths, tc.wantPath) {
				t.Fatalf("want problem at %q, got %v", tc.wantPath, paths)
			}
		})
	}
}

func TestValidateFormReportsAllProblems(t *testing.T) {
	f := baseForm()
	f.Slug = "BAD"
	f.Title = ""
	f.Fields[0].Key = "1x"
	paths := problemPaths(t, ValidateForm(f))
	for _, want := range []string{"slug", "title", "fields[0].key"} {
		if !hasPath(paths, want) {
			t.Errorf("missing %q in %v", want, paths)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run TestValidateForm -v`
Expected: FAIL — `undefined: ValidateForm`.

- [ ] **Step 3: Write the implementation**

`internal/definition/validate_form.go`:
```go
package definition

import (
	"fmt"
	"regexp"
	"strings"
)

var formFieldTypes = map[FieldType]bool{
	FieldText: true, FieldTextarea: true, FieldEmail: true, FieldNumber: true, FieldSelect: true,
	FieldMultiselect: true, FieldCheckbox: true, FieldDate: true, FieldURL: true,
}

// stringFieldTypes may carry minLength/maxLength/pattern.
var stringFieldTypes = map[FieldType]bool{FieldText: true, FieldTextarea: true, FieldEmail: true, FieldURL: true}

// ValidateForm applies the semantic rules of spec §5.3 to a form. It reports
// every problem it finds, not just the first.
func ValidateForm(f Form) error {
	var ps problems
	if !slugRe.MatchString(f.Slug) {
		ps.add("slug", slugRule)
	}
	if strings.TrimSpace(f.Title) == "" {
		ps.add("title", "is required")
	}
	if f.Workflow != "" && !slugRe.MatchString(f.Workflow) {
		ps.add("workflow", "must be a valid workflow slug")
	}
	if len(f.Fields) == 0 {
		ps.add("fields", "at least one field is required")
	}
	seen := make(map[string]int, len(f.Fields)) // key → index of first declaration
	for i, fld := range f.Fields {
		p := fmt.Sprintf("fields[%d]", i)
		if !keyRe.MatchString(fld.Key) {
			ps.add(p+".key", keyRule)
		} else if j, dup := seen[fld.Key]; dup {
			ps.add(p+".key", fmt.Sprintf("duplicate key %q (also used by fields[%d])", fld.Key, j))
		} else {
			seen[fld.Key] = i
		}
		if !formFieldTypes[fld.Type] {
			ps.add(p+".type", fmt.Sprintf("unknown field type %q", fld.Type))
		}
		if strings.TrimSpace(fld.Label) == "" {
			ps.add(p+".label", "is required")
		}
		validateOptions(&ps, p, fld.Type == FieldSelect || fld.Type == FieldMultiselect,
			"options are only allowed on select and multiselect fields", fld.Options)
		validateFieldValidation(&ps, p, fld)
		if fld.ShowIf != nil {
			validateCondition(&ps, p+".showIf", *fld.ShowIf, seen, i)
		}
	}
	return ps.err()
}

func validateFieldValidation(ps *problems, p string, fld Field) {
	v := fld.Validation
	if v == nil {
		return
	}
	vp := p + ".validation"
	if !stringFieldTypes[fld.Type] {
		const msg = "is only allowed on text, textarea, email and url fields"
		if v.MinLength != nil {
			ps.add(vp+".minLength", "minLength "+msg)
		}
		if v.MaxLength != nil {
			ps.add(vp+".maxLength", "maxLength "+msg)
		}
		if v.Pattern != "" {
			ps.add(vp+".pattern", "pattern "+msg)
		}
	}
	if fld.Type != FieldNumber {
		if v.Min != nil {
			ps.add(vp+".min", "min is only allowed on number fields")
		}
		if v.Max != nil {
			ps.add(vp+".max", "max is only allowed on number fields")
		}
	}
	if v.MinLength != nil && *v.MinLength < 0 {
		ps.add(vp+".minLength", "must be 0 or greater")
	}
	if v.MaxLength != nil && *v.MaxLength < 0 {
		ps.add(vp+".maxLength", "must be 0 or greater")
	}
	if v.MinLength != nil && v.MaxLength != nil && *v.MinLength > *v.MaxLength {
		ps.add(vp+".maxLength", "must be greater than or equal to minLength")
	}
	if v.Min != nil && v.Max != nil && *v.Min > *v.Max {
		ps.add(vp+".max", "must be greater than or equal to min")
	}
	if v.Pattern != "" {
		if _, err := regexp.Compile(v.Pattern); err != nil {
			ps.add(vp+".pattern", "is not a valid regular expression: "+err.Error())
		}
	}
}

// validateCondition checks a showIf. seen maps field keys to the index of
// their first declaration; the controller must be declared before index i.
func validateCondition(ps *problems, p string, c Condition, seen map[string]int, i int) {
	if j, ok := seen[c.Field]; !ok || j >= i {
		ps.add(p+".field", fmt.Sprintf("must reference a field declared before this one (got %q)", c.Field))
	}
	n := 0
	if c.Equals != nil {
		n++
	}
	if c.NotEquals != nil {
		n++
	}
	if c.In != nil {
		n++
	}
	switch {
	case n != 1:
		ps.add(p, "exactly one of equals, notEquals or in is required")
	case c.In != nil && len(c.In) == 0:
		ps.add(p+".in", "must not be empty")
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run TestValidateForm -v`
Expected: PASS — `TestValidateFormAcceptsBase`, `TestValidateFormRules` (25 subtests), `TestValidateFormReportsAllProblems`.

- [ ] **Step 5: Commit**

```bash
git add internal/definition/validate_form.go internal/definition/validate_form_test.go
git commit -m "feat(definition): add semantic validation for forms"
```

---

### Task 5: Workflow semantic validation

**Parallel group A** (with Tasks 3, 4, 6 — disjoint files).

**Files:**
- Create: `internal/definition/validate_workflow.go`
- Test: `internal/definition/validate_workflow_test.go`

**Interfaces:**
- Consumes: types, `problems`, `slugRe`, `keyRe`, `slugRule`, `keyRule`, `validateOptions` (Task 1).
- Produces: `func ValidateWorkflow(w Workflow) error`; package-internal `validateAction(ps *problems, p string, a Action)`. Used by Task 7, Plans 03/04/08.

- [ ] **Step 1: Write the failing test**

`internal/definition/validate_workflow_test.go`:
```go
package definition

import "testing"

func baseWorkflow() Workflow {
	return Workflow{
		Slug:    "hiring",
		Title:   "Hiring pipeline",
		Initial: "new",
		States: []State{
			{Key: "new", Label: "New", Color: "gray"},
			{Key: "screening", Label: "Screening", Color: "blue"},
			{Key: "interview", Label: "Interview", Color: "purple"},
			{Key: "hired", Label: "Hired", Color: "green", Terminal: true},
			{Key: "rejected", Label: "Rejected", Color: "red", Terminal: true},
		},
		Fields: []WorkflowField{
			{Key: "score", Type: FieldNumber, Label: "Score"},
			{Key: "rejectionReason", Type: FieldTextarea, Label: "Rejection reason"},
			{Key: "source", Type: FieldSelect, Label: "Source",
				Options: []Option{{Value: "referral", Label: "Referral"}, {Value: "website", Label: "Website"}}},
		},
		OnSubmit: []Action{
			{Type: ActionAssign, Role: "reviewer"},
			{Type: ActionEmail, To: "{{submission.data.email}}", Subject: "We got your application", Body: "Hi {{submission.data.name}}"},
		},
		Transitions: []Transition{
			{Key: "screen", Label: "Start screening", From: []string{"new"}, To: "screening",
				Guard: Guard{Roles: []string{"reviewer"}}},
			{Key: "invite", Label: "Invite to interview", From: []string{"screening"}, To: "interview",
				Guard:   Guard{Roles: []string{"reviewer"}, RequireFields: []string{"score"}},
				Actions: []Action{{Type: ActionWebhook, URL: "https://example.com/hooks/interview"}}},
			{Key: "hire", Label: "Hire", From: []string{"interview"}, To: "hired",
				Guard: Guard{Roles: []string{"hiring-manager"}}},
			{Key: "reject", Label: "Reject", From: []string{"new", "screening", "interview"}, To: "rejected",
				Guard:   Guard{Roles: []string{"reviewer", "hiring-manager"}, RequireFields: []string{"rejectionReason"}},
				Actions: []Action{{Type: ActionEmail, To: "{{submission.data.email}}", Subject: "Your application", Body: "{{submission.fields.rejectionReason}}"}}},
		},
	}
}

func TestValidateWorkflowAcceptsBase(t *testing.T) {
	if err := ValidateWorkflow(baseWorkflow()); err != nil {
		t.Fatalf("base workflow should be valid: %v", err)
	}
}

func TestValidateWorkflowRules(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(w *Workflow)
		wantPath string
	}{
		{"bad slug", func(w *Workflow) { w.Slug = "Hiring" }, "slug"},
		{"empty title", func(w *Workflow) { w.Title = "" }, "title"},
		{"no states", func(w *Workflow) { w.States = nil; w.Transitions = nil }, "states"},
		{"bad state key", func(w *Workflow) { w.States[0].Key = "1new" }, "states[0].key"},
		{"duplicate state key", func(w *Workflow) { w.States[1].Key = "new" }, "states[1].key"},
		{"empty state label", func(w *Workflow) { w.States[0].Label = " " }, "states[0].label"},
		{"bad color", func(w *Workflow) { w.States[0].Color = "orange" }, "states[0].color"},
		{"unknown initial", func(w *Workflow) { w.Initial = "draft" }, "initial"},
		{"email workflow field", func(w *Workflow) { w.Fields[0].Type = FieldEmail }, "fields[0].type"},
		{"duplicate workflow field", func(w *Workflow) { w.Fields[1].Key = "score" }, "fields[1].key"},
		{"bad workflow field key", func(w *Workflow) { w.Fields[0].Key = "sco re" }, "fields[0].key"},
		{"empty workflow field label", func(w *Workflow) { w.Fields[0].Label = "" }, "fields[0].label"},
		{"select field without options", func(w *Workflow) { w.Fields[2].Options = nil }, "fields[2].options"},
		{"options on number field", func(w *Workflow) { w.Fields[0].Options = []Option{{Value: "a", Label: "A"}} }, "fields[0].options"},
		{"duplicate transition key", func(w *Workflow) { w.Transitions[1].Key = "screen" }, "transitions[1].key"},
		{"bad transition key", func(w *Workflow) { w.Transitions[0].Key = "start-screening" }, "transitions[0].key"},
		{"empty transition label", func(w *Workflow) { w.Transitions[0].Label = "" }, "transitions[0].label"},
		{"empty from", func(w *Workflow) { w.Transitions[0].From = nil }, "transitions[0].from"},
		{"unknown from", func(w *Workflow) { w.Transitions[0].From = []string{"draft"} }, "transitions[0].from[0]"},
		{"unknown to", func(w *Workflow) { w.Transitions[0].To = "draft" }, "transitions[0].to"},
		{"from terminal", func(w *Workflow) { w.Transitions[0].From = []string{"hired"} }, "transitions[0].from[0]"},
		{"empty role", func(w *Workflow) { w.Transitions[0].Guard.Roles = []string{" "} }, "transitions[0].guard.roles[0]"},
		{"unknown requireField", func(w *Workflow) { w.Transitions[1].Guard.RequireFields = []string{"rating"} }, "transitions[1].guard.requireFields[0]"},
		{"webhook without url", func(w *Workflow) { w.Transitions[1].Actions[0].URL = "" }, "transitions[1].actions[0].url"},
		{"webhook ftp url", func(w *Workflow) { w.Transitions[1].Actions[0].URL = "ftp://example.com" }, "transitions[1].actions[0].url"},
		{"webhook with subject", func(w *Workflow) { w.Transitions[1].Actions[0].Subject = "x" }, "transitions[1].actions[0].subject"},
		{"email without to", func(w *Workflow) { w.Transitions[3].Actions[0].To = "" }, "transitions[3].actions[0].to"},
		{"email without subject", func(w *Workflow) { w.Transitions[3].Actions[0].Subject = "" }, "transitions[3].actions[0].subject"},
		{"email with url", func(w *Workflow) { w.Transitions[3].Actions[0].URL = "https://x.dev" }, "transitions[3].actions[0].url"},
		{"assign with user and role", func(w *Workflow) { w.OnSubmit[0].User = "ada@example.com" }, "onSubmit[0]"},
		{"assign with neither", func(w *Workflow) { w.OnSubmit[0].Role = "" }, "onSubmit[0]"},
		{"assign bad user email", func(w *Workflow) { w.OnSubmit[0] = Action{Type: ActionAssign, User: "ada"} }, "onSubmit[0].user"},
		{"assign with body", func(w *Workflow) { w.OnSubmit[0].Body = "x" }, "onSubmit[0].body"},
		{"unknown action type", func(w *Workflow) { w.OnSubmit[1].Type = "sms" }, "onSubmit[1].type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := baseWorkflow()
			tc.mutate(&w)
			paths := problemPaths(t, ValidateWorkflow(w))
			if !hasPath(paths, tc.wantPath) {
				t.Fatalf("want problem at %q, got %v", tc.wantPath, paths)
			}
		})
	}
}

func TestValidateWorkflowAllowsNoTransitions(t *testing.T) {
	w := Workflow{Slug: "inbox", Title: "Inbox", Initial: "open", States: []State{{Key: "open", Label: "Open"}}}
	if err := ValidateWorkflow(w); err != nil {
		t.Fatalf("a single-state workflow without transitions is valid: %v", err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run TestValidateWorkflow -v`
Expected: FAIL — `undefined: ValidateWorkflow`.

- [ ] **Step 3: Write the implementation**

`internal/definition/validate_workflow.go`:
```go
package definition

import (
	"fmt"
	"net/mail"
	"net/url"
	"strings"
)

var workflowFieldTypes = map[FieldType]bool{
	FieldText: true, FieldTextarea: true, FieldNumber: true, FieldSelect: true, FieldCheckbox: true, FieldDate: true,
}

var stateColors = map[string]bool{"gray": true, "blue": true, "green": true, "yellow": true, "red": true, "purple": true}

// ValidateWorkflow applies the semantic rules of spec §5.3 to a workflow and
// reports every problem it finds.
func ValidateWorkflow(w Workflow) error {
	var ps problems
	if !slugRe.MatchString(w.Slug) {
		ps.add("slug", slugRule)
	}
	if strings.TrimSpace(w.Title) == "" {
		ps.add("title", "is required")
	}

	if len(w.States) == 0 {
		ps.add("states", "at least one state is required")
	}
	states := make(map[string]State, len(w.States))
	for i, s := range w.States {
		p := fmt.Sprintf("states[%d]", i)
		if !keyRe.MatchString(s.Key) {
			ps.add(p+".key", keyRule)
		} else if _, dup := states[s.Key]; dup {
			ps.add(p+".key", fmt.Sprintf("duplicate state key %q", s.Key))
		} else {
			states[s.Key] = s
		}
		if strings.TrimSpace(s.Label) == "" {
			ps.add(p+".label", "is required")
		}
		if s.Color != "" && !stateColors[s.Color] {
			ps.add(p+".color", "must be one of gray, blue, green, yellow, red, purple")
		}
	}
	if _, ok := states[w.Initial]; !ok {
		ps.add("initial", fmt.Sprintf("unknown state %q", w.Initial))
	}

	fields := make(map[string]bool, len(w.Fields))
	for i, f := range w.Fields {
		p := fmt.Sprintf("fields[%d]", i)
		if !keyRe.MatchString(f.Key) {
			ps.add(p+".key", keyRule)
		} else if fields[f.Key] {
			ps.add(p+".key", fmt.Sprintf("duplicate workflow field key %q", f.Key))
		} else {
			fields[f.Key] = true
		}
		if !workflowFieldTypes[f.Type] {
			ps.add(p+".type", fmt.Sprintf("type %q is not allowed for workflow fields (use text, textarea, number, select, checkbox or date)", f.Type))
		}
		if strings.TrimSpace(f.Label) == "" {
			ps.add(p+".label", "is required")
		}
		validateOptions(&ps, p, f.Type == FieldSelect, "options are only allowed on select fields", f.Options)
	}

	for j, a := range w.OnSubmit {
		validateAction(&ps, fmt.Sprintf("onSubmit[%d]", j), a)
	}

	seen := make(map[string]bool, len(w.Transitions))
	for i, t := range w.Transitions {
		p := fmt.Sprintf("transitions[%d]", i)
		if !keyRe.MatchString(t.Key) {
			ps.add(p+".key", keyRule)
		} else if seen[t.Key] {
			ps.add(p+".key", fmt.Sprintf("duplicate transition key %q", t.Key))
		} else {
			seen[t.Key] = true
		}
		if strings.TrimSpace(t.Label) == "" {
			ps.add(p+".label", "is required")
		}
		if len(t.From) == 0 {
			ps.add(p+".from", "at least one source state is required")
		}
		for j, from := range t.From {
			fp := fmt.Sprintf("%s.from[%d]", p, j)
			s, ok := states[from]
			switch {
			case !ok:
				ps.add(fp, fmt.Sprintf("unknown state %q", from))
			case s.Terminal:
				ps.add(fp, fmt.Sprintf("cannot transition out of terminal state %q", from))
			}
		}
		if _, ok := states[t.To]; !ok {
			ps.add(p+".to", fmt.Sprintf("unknown state %q", t.To))
		}
		for j, r := range t.Guard.Roles {
			if strings.TrimSpace(r) == "" {
				ps.add(fmt.Sprintf("%s.guard.roles[%d]", p, j), "must not be empty")
			}
		}
		for j, rf := range t.Guard.RequireFields {
			if !fields[rf] {
				ps.add(fmt.Sprintf("%s.guard.requireFields[%d]", p, j), fmt.Sprintf("unknown workflow field %q", rf))
			}
		}
		for j, a := range t.Actions {
			validateAction(&ps, fmt.Sprintf("%s.actions[%d]", p, j), a)
		}
	}
	return ps.err()
}

// validateAction checks the per-type rules of spec §5.2.
func validateAction(ps *problems, p string, a Action) {
	notAllowed := func(name, val string) {
		if val != "" {
			ps.add(p+"."+name, fmt.Sprintf("not allowed on %s actions", a.Type))
		}
	}
	switch a.Type {
	case ActionWebhook:
		if a.URL == "" {
			ps.add(p+".url", "is required")
		} else if u, err := url.Parse(a.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			ps.add(p+".url", "must be an absolute http(s) URL")
		}
		notAllowed("to", a.To)
		notAllowed("subject", a.Subject)
		notAllowed("body", a.Body)
		notAllowed("user", a.User)
		notAllowed("role", a.Role)
	case ActionEmail:
		if strings.TrimSpace(a.To) == "" {
			ps.add(p+".to", "is required")
		}
		if strings.TrimSpace(a.Subject) == "" {
			ps.add(p+".subject", "is required")
		}
		notAllowed("url", a.URL)
		notAllowed("user", a.User)
		notAllowed("role", a.Role)
	case ActionAssign:
		if (a.User == "") == (a.Role == "") {
			ps.add(p, "exactly one of user or role is required")
		}
		if a.User != "" {
			if addr, err := mail.ParseAddress(a.User); err != nil || addr.Address != a.User {
				ps.add(p+".user", "must be an email address")
			}
		}
		notAllowed("url", a.URL)
		notAllowed("to", a.To)
		notAllowed("subject", a.Subject)
		notAllowed("body", a.Body)
	default:
		ps.add(p+".type", fmt.Sprintf("unknown action type %q", a.Type))
	}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run TestValidateWorkflow -v`
Expected: PASS — `TestValidateWorkflowAcceptsBase`, `TestValidateWorkflowRules` (34 subtests), `TestValidateWorkflowAllowsNoTransitions`.

- [ ] **Step 5: Commit**

```bash
git add internal/definition/validate_workflow.go internal/definition/validate_workflow_test.go
git commit -m "feat(definition): add semantic validation for workflows"
```

---

### Task 6: Per-type answer validation (`checkValue`)

**Parallel group A** (with Tasks 3, 4, 5 — disjoint files).

**Files:**
- Create: `internal/definition/values.go`
- Test: `internal/definition/values_test.go`

**Interfaces:**
- Consumes: `FieldType` constants, `Option`, `Validation` (Task 1); `intPtr`, `floatPtr` (Task 1 helpers).
- Produces (package-internal, used by Tasks 8 and 9):
  - `func checkValue(t FieldType, options []Option, v *Validation, raw any) (val any, provided bool, msg string)` — `msg != ""` means present but invalid; `provided == false && msg == ""` means "not provided" (nil, blank string, empty list). Normalized values: strings trimmed; numbers `float64`; checkbox `bool`; multiselect `[]any` of `string`.
  - `func normalizeJSON(v any) any` — ints/`json.Number`/`float32` → `float64`, `[]string` → `[]any`, recursing into `[]any` and `map[string]any`.
  - `func jsonEqual(a, b any) bool` — deep equality after `normalizeJSON`.

- [ ] **Step 1: Write the failing test**

`internal/definition/values_test.go`:
```go
package definition

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCheckValue(t *testing.T) {
	opts := []Option{{Value: "go", Label: "Go"}, {Value: "ts", Label: "TypeScript"}}
	cases := []struct {
		name     string
		typ      FieldType
		v        *Validation
		raw      any
		want     any
		provided bool
		msg      string
	}{
		{"nil", FieldText, nil, nil, nil, false, ""},
		{"trim", FieldText, nil, "  hi ", "hi", true, ""},
		{"blank", FieldText, nil, "   ", nil, false, ""},
		{"not string", FieldText, nil, 5.0, nil, false, "must be a string"},
		{"object for text", FieldTextarea, nil, map[string]any{"a": 1.0}, nil, false, "must be a string"},
		{"min length runes", FieldText, &Validation{MinLength: intPtr(3)}, "éé", nil, false, "must be at least 3 characters"},
		{"max length runes", FieldText, &Validation{MaxLength: intPtr(2)}, "👍👍", "👍👍", true, ""},
		{"max length exceeded", FieldText, &Validation{MaxLength: intPtr(2)}, "abc", nil, false, "must be at most 2 characters"},
		{"pattern anchored", FieldText, &Validation{Pattern: "[A-Z]{3}"}, "ABCD", nil, false, "must match the required format"},
		{"pattern ok", FieldText, &Validation{Pattern: "[A-Z]{3}"}, "ABC", "ABC", true, ""},
		{"bad pattern ignored", FieldText, &Validation{Pattern: "("}, "x", "x", true, ""},
		{"email ok", FieldEmail, nil, "ada@example.com", "ada@example.com", true, ""},
		{"email display name", FieldEmail, nil, "Ada <ada@example.com>", nil, false, "must be a valid email address"},
		{"email angle brackets", FieldEmail, nil, "<ada@example.com>", nil, false, "must be a valid email address"},
		{"email no at", FieldEmail, nil, "ada", nil, false, "must be a valid email address"},
		{"url ok", FieldURL, nil, "https://example.com", "https://example.com", true, ""},
		{"url no scheme", FieldURL, nil, "example.com", nil, false, "must be an absolute http(s) URL"},
		{"url ftp", FieldURL, nil, "ftp://example.com", nil, false, "must be an absolute http(s) URL"},
		{"date ok", FieldDate, nil, "2024-02-29", "2024-02-29", true, ""},
		{"date impossible", FieldDate, nil, "2023-02-29", nil, false, "must be a date in YYYY-MM-DD format"},
		{"date short", FieldDate, nil, "2024-2-3", nil, false, "must be a date in YYYY-MM-DD format"},
		{"number float", FieldNumber, nil, 3.5, 3.5, true, ""},
		{"number int normalized", FieldNumber, nil, 42, 42.0, true, ""},
		{"number json.Number", FieldNumber, nil, json.Number("7"), 7.0, true, ""},
		{"number string", FieldNumber, nil, "5", nil, false, "must be a number"},
		{"number bool", FieldNumber, nil, true, nil, false, "must be a number"},
		{"number below min", FieldNumber, &Validation{Min: floatPtr(1)}, 0.5, nil, false, "must be at least 1"},
		{"number above max", FieldNumber, &Validation{Max: floatPtr(10)}, 10.5, nil, false, "must be at most 10"},
		{"number at max", FieldNumber, &Validation{Max: floatPtr(10)}, 10.0, 10.0, true, ""},
		{"checkbox false", FieldCheckbox, nil, false, false, true, ""},
		{"checkbox string", FieldCheckbox, nil, "true", nil, false, "must be true or false"},
		{"select ok", FieldSelect, nil, "go", "go", true, ""},
		{"select trimmed", FieldSelect, nil, " go ", "go", true, ""},
		{"select bad", FieldSelect, nil, "rust", nil, false, "must be one of the options"},
		{"multi ok", FieldMultiselect, nil, []any{"ts", "go"}, []any{"ts", "go"}, true, ""},
		{"multi []string", FieldMultiselect, nil, []string{"go"}, []any{"go"}, true, ""},
		{"multi empty", FieldMultiselect, nil, []any{}, nil, false, ""},
		{"multi dup", FieldMultiselect, nil, []any{"go", "go"}, nil, false, "must not contain duplicates"},
		{"multi bad option", FieldMultiselect, nil, []any{"rust"}, nil, false, "must only contain valid options"},
		{"multi string", FieldMultiselect, nil, "go", nil, false, "must be a list of options"},
		{"multi non-string item", FieldMultiselect, nil, []any{1.0}, nil, false, "must be a list of options"},
		{"unknown type", FieldType("color"), nil, "x", nil, false, "unsupported field type"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, provided, msg := checkValue(tc.typ, opts, tc.v, tc.raw)
			if msg != tc.msg {
				t.Fatalf("msg = %q, want %q", msg, tc.msg)
			}
			if provided != tc.provided {
				t.Fatalf("provided = %v, want %v", provided, tc.provided)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("value = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestJSONEqual(t *testing.T) {
	if !jsonEqual(18, 18.0) {
		t.Error("int and float64 18 should be equal")
	}
	if !jsonEqual([]string{"a"}, []any{"a"}) {
		t.Error("[]string and []any should be equal")
	}
	if jsonEqual("18", 18.0) {
		t.Error("string and number must differ")
	}
	if !jsonEqual(map[string]any{"a": []any{1}}, map[string]any{"a": []any{1.0}}) {
		t.Error("nested normalization failed")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run 'TestCheckValue|TestJSONEqual' -v`
Expected: FAIL — `undefined: checkValue`, `undefined: jsonEqual`.

- [ ] **Step 3: Write the implementation**

`internal/definition/values.go`:
```go
package definition

import (
	"encoding/json"
	"fmt"
	"math"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

// checkValue validates and normalizes one answer (spec §5.4 + Plan 02 clarifications).
// provided=false with msg=="" means "not provided" (nil, blank string, empty list).
// msg != "" means the value is present but invalid.
func checkValue(t FieldType, options []Option, v *Validation, raw any) (val any, provided bool, msg string) {
	if raw == nil {
		return nil, false, ""
	}
	switch t {
	case FieldText, FieldTextarea, FieldEmail, FieldURL, FieldDate, FieldSelect:
		s, ok := raw.(string)
		if !ok {
			return nil, false, "must be a string"
		}
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, false, ""
		}
		if m := checkString(t, options, v, s); m != "" {
			return nil, false, m
		}
		return s, true, ""
	case FieldNumber:
		n, ok := toFloat(raw)
		if !ok {
			return nil, false, "must be a number"
		}
		if v != nil && v.Min != nil && n < *v.Min {
			return nil, false, "must be at least " + formatNumber(*v.Min)
		}
		if v != nil && v.Max != nil && n > *v.Max {
			return nil, false, "must be at most " + formatNumber(*v.Max)
		}
		return n, true, ""
	case FieldCheckbox:
		b, ok := raw.(bool)
		if !ok {
			return nil, false, "must be true or false"
		}
		return b, true, ""
	case FieldMultiselect:
		items, ok := toStrings(raw)
		if !ok {
			return nil, false, "must be a list of options"
		}
		if len(items) == 0 {
			return nil, false, ""
		}
		seen := make(map[string]bool, len(items))
		out := make([]any, 0, len(items))
		for _, it := range items {
			if !hasOption(options, it) {
				return nil, false, "must only contain valid options"
			}
			if seen[it] {
				return nil, false, "must not contain duplicates"
			}
			seen[it] = true
			out = append(out, it)
		}
		return out, true, ""
	}
	return nil, false, "unsupported field type"
}

func checkString(t FieldType, options []Option, v *Validation, s string) string {
	switch t {
	case FieldEmail:
		if a, err := mail.ParseAddress(s); err != nil || a.Name != "" || a.Address != s {
			return "must be a valid email address"
		}
	case FieldURL:
		if u, err := url.Parse(s); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return "must be an absolute http(s) URL"
		}
	case FieldDate:
		if d, err := time.Parse("2006-01-02", s); err != nil || d.Format("2006-01-02") != s {
			return "must be a date in YYYY-MM-DD format"
		}
	case FieldSelect:
		if !hasOption(options, s) {
			return "must be one of the options"
		}
	}
	if v == nil {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if v.MinLength != nil && n < *v.MinLength {
		return fmt.Sprintf("must be at least %d characters", *v.MinLength)
	}
	if v.MaxLength != nil && n > *v.MaxLength {
		return fmt.Sprintf("must be at most %d characters", *v.MaxLength)
	}
	if v.Pattern != "" {
		if re := anchored(v.Pattern); re != nil && !re.MatchString(s) {
			return "must match the required format"
		}
	}
	return ""
}

var patternCache sync.Map // pattern string → *regexp.Regexp (nil when uncompilable)

// anchored compiles ^(?:p)$, caching the result. Uncompilable patterns return nil
// and are ignored here; ValidateForm rejects them at definition time.
func anchored(p string) *regexp.Regexp {
	if re, ok := patternCache.Load(p); ok {
		return re.(*regexp.Regexp)
	}
	re, err := regexp.Compile(`^(?:` + p + `)$`)
	if err != nil {
		re = nil
	}
	patternCache.Store(p, re)
	return re
}

func hasOption(options []Option, v string) bool {
	for _, o := range options {
		if o.Value == v {
			return true
		}
	}
	return false
}

func toFloat(v any) (float64, bool) {
	var f float64
	switch n := v.(type) {
	case float64:
		f = n
	case float32:
		f = float64(n)
	case int:
		f = float64(n)
	case int32:
		f = float64(n)
	case int64:
		f = float64(n)
	case json.Number:
		x, err := n.Float64()
		if err != nil {
			return 0, false
		}
		f = x
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

func toStrings(v any) ([]string, bool) {
	switch s := v.(type) {
	case []string:
		return s, true
	case []any:
		out := make([]string, len(s))
		for i, it := range s {
			str, ok := it.(string)
			if !ok {
				return nil, false
			}
			out[i] = str
		}
		return out, true
	}
	return nil, false
}

func formatNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// normalizeJSON maps Go values onto the shapes encoding/json produces, so values
// from definitions, requests and fixtures compare equal.
func normalizeJSON(v any) any {
	if f, ok := toFloat(v); ok {
		return f
	}
	switch x := v.(type) {
	case []string:
		out := make([]any, len(x))
		for i, s := range x {
			out[i] = s
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, it := range x {
			out[i] = normalizeJSON(it)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, it := range x {
			out[k] = normalizeJSON(it)
		}
		return out
	}
	return v
}

func jsonEqual(a, b any) bool { return reflect.DeepEqual(normalizeJSON(a), normalizeJSON(b)) }
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run 'TestCheckValue|TestJSONEqual' -v`
Expected: PASS — `TestCheckValue` (42 subtests), `TestJSONEqual`.

- [ ] **Step 5: Commit**

```bash
git add internal/definition/values.go internal/definition/values_test.go
git commit -m "feat(definition): add per-type answer validation and normalization"
```

---

### Task 7: Parsing (YAML/JSON → schema → semantics) and bundle validation

**Parallel group B** (with Tasks 8, 9 — disjoint files). Requires Tasks 2, 3, 4, 5.

**Files:**
- Create: `internal/definition/parse.go`
- Create: `internal/definition/bundle.go`
- Test: `internal/definition/parse_test.go`
- Test: `internal/definition/bundle_test.go`
- Modify: `go.mod`, `go.sum` (adds `sigs.k8s.io/yaml`, `golang.org/x/text`)

**Interfaces:**
- Consumes: `schemas.FS` (Task 2), `ValidateForm` (Task 4), `ValidateWorkflow` (Task 5), `Canonical` (Task 3, tests only), `problems` (Task 1), `baseForm()`/`baseWorkflow()` test fixtures (Tasks 4/5 test files, same package).
- Produces: `func ParseForm(raw []byte) (Form, error)`, `func ParseWorkflow(raw []byte) (Workflow, error)`, `func ValidateBundle(forms []Form, workflows []Workflow, existingWorkflowSlugs []string) error`. Schema problems use paths like `fields[0].options[0].value`; YAML/JSON syntax errors, duplicate keys and empty documents are root problems (`Path == ""`). Bundle problems are prefixed `forms[i].` / `workflows[i].`. Used by Plans 03 (API), 05 (CLI), 09 (seed).

- [ ] **Step 1: Add dependencies**

Run: `go get sigs.k8s.io/yaml@latest golang.org/x/text@latest`
Expected: `go: added sigs.k8s.io/yaml v1.x.y` (and `golang.org/x/text` added or upgraded).

- [ ] **Step 2: Write the failing tests**

`internal/definition/parse_test.go`:
```go
package definition

import (
	"strings"
	"testing"
)

const jobApplicationYAML = `
slug: job-application
title: Job application
description: Apply to join the team.
workflow: hiring
settings:
  public: true
  submitLabel: Send application
  confirmationMessage: Thanks! We'll be in touch.
fields:
  - key: name
    type: text
    label: Full name
    required: true
    placeholder: Ada Lovelace
    help: As on your passport
    validation: { minLength: 2, maxLength: 100 }
  - key: role
    type: select
    label: Role
    required: true
    options:
      - { value: engineer, label: Engineer }
      - { value: designer, label: Designer }
  - key: portfolio
    type: url
    label: Portfolio URL
    showIf: { field: role, equals: designer }
`

const jobApplicationJSON = `{
  "fields": [
    {"key": "name", "type": "text", "label": "Full name", "required": true, "placeholder": "Ada Lovelace",
     "help": "As on your passport", "validation": {"maxLength": 100, "minLength": 2}},
    {"key": "role", "type": "select", "label": "Role", "required": true,
     "options": [{"value": "engineer", "label": "Engineer"}, {"value": "designer", "label": "Designer"}]},
    {"key": "portfolio", "type": "url", "label": "Portfolio URL", "showIf": {"field": "role", "equals": "designer"}}
  ],
  "settings": {"confirmationMessage": "Thanks! We'll be in touch.", "submitLabel": "Send application", "public": true},
  "workflow": "hiring",
  "description": "Apply to join the team.",
  "title": "Job application",
  "slug": "job-application"
}`

const hiringYAML = `
slug: hiring
title: Hiring pipeline
initial: new
states:
  - { key: new, label: New, color: gray }
  - { key: screening, label: Screening, color: blue }
  - { key: interview, label: Interview, color: purple }
  - { key: hired, label: Hired, color: green, terminal: true }
  - { key: rejected, label: Rejected, color: red, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
onSubmit:
  - { type: assign, role: reviewer }
  - { type: email, to: "{{submission.data.email}}", subject: "We got your application", body: "Hi {{submission.data.name}}" }
transitions:
  - key: screen
    label: Start screening
    from: [new]
    to: screening
    guard: { roles: [reviewer] }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "https://example.com/hooks/interview" }
  - key: hire
    label: Hire
    from: [interview]
    to: hired
    guard: { roles: [hiring-manager] }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
`

func TestParseFormYAML(t *testing.T) {
	f, err := ParseForm([]byte(jobApplicationYAML))
	if err != nil {
		t.Fatal(err)
	}
	if f.Slug != "job-application" || f.Workflow != "hiring" || !f.Settings.Public || len(f.Fields) != 3 {
		t.Fatalf("unexpected form: %+v", f)
	}
	if f.Fields[0].Validation == nil || *f.Fields[0].Validation.MinLength != 2 {
		t.Fatalf("validation not decoded: %+v", f.Fields[0].Validation)
	}
	if f.Fields[2].ShowIf == nil || f.Fields[2].ShowIf.Equals != "designer" {
		t.Fatalf("showIf not decoded: %+v", f.Fields[2].ShowIf)
	}
}

func TestParseFormYAMLAndJSONAgree(t *testing.T) {
	fy, err := ParseForm([]byte(jobApplicationYAML))
	if err != nil {
		t.Fatal(err)
	}
	fj, err := ParseForm([]byte(jobApplicationJSON))
	if err != nil {
		t.Fatal(err)
	}
	_, hy, _ := Canonical(fy)
	_, hj, _ := Canonical(fj)
	if hy != hj {
		t.Fatal("YAML and JSON sources of the same form must hash equally")
	}
}

func TestParseWorkflowYAML(t *testing.T) {
	w, err := ParseWorkflow([]byte(hiringYAML))
	if err != nil {
		t.Fatal(err)
	}
	if w.Initial != "new" || len(w.States) != 5 || len(w.Transitions) != 4 || len(w.OnSubmit) != 2 {
		t.Fatalf("unexpected workflow: %+v", w)
	}
	if tr, ok := w.Transition("invite"); !ok || tr.Guard.RequireFields[0] != "score" || tr.Actions[0].Type != ActionWebhook {
		t.Fatalf("invite transition not decoded: %+v", tr)
	}
}

func TestParseProblems(t *testing.T) {
	cases := []struct {
		name     string
		parse    func([]byte) error
		doc      string
		wantPath string
		wantMsg  string // substring
	}{
		{"empty document", parseFormErr, "   \n", "", "empty"},
		{"syntax error", parseFormErr, "slug: [unclosed", "", "invalid YAML/JSON"},
		{"top level list", parseFormErr, "- a\n- b\n", "", ""},
		{"unknown property", parseFormErr, "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: X, colour: red }\n", "fields[0]", "colour"},
		{"wrong type", parseFormErr, "slug: a\ntitle: 5\nfields:\n  - { key: x, type: text, label: X }\n", "title", ""},
		{"semantic after schema", parseFormErr, "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: X }\n  - { key: x, type: text, label: Y }\n", "fields[1].key", "duplicate"},
		{"workflow schema", parseWorkflowErr, "slug: w\ntitle: W\ninitial: a\nstates:\n  - { key: a, label: A, color: orange }\n", "states[0].color", ""},
		{"workflow semantic", parseWorkflowErr, "slug: w\ntitle: W\ninitial: b\nstates:\n  - { key: a, label: A }\n", "initial", "unknown state"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.parse([]byte(tc.doc))
			paths := problemPaths(t, err)
			if !hasPath(paths, tc.wantPath) {
				t.Fatalf("want problem at %q, got %v (%v)", tc.wantPath, paths, err)
			}
			if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("error %q does not mention %q", err, tc.wantMsg)
			}
		})
	}
}

// Review Focus 1: YAML 1.1 turns unquoted yes/no/on/off into booleans.
func TestParseFormYAMLImplicitBoolean(t *testing.T) {
	doc := "slug: a\ntitle: A\nfields:\n  - key: ok\n    type: select\n    label: OK?\n    options:\n      - { value: yes, label: Yes }\n"
	paths := problemPaths(t, parseFormErr([]byte(doc)))
	if !hasPath(paths, "fields[0].options[0].value") {
		t.Fatalf("want problem at fields[0].options[0].value, got %v", paths)
	}
}

// Review Focus 2: duplicate keys must not silently "last one wins".
func TestParseFormRejectsDuplicateKeys(t *testing.T) {
	doc := "slug: a\ntitle: A\ntitle: B\nfields:\n  - { key: x, type: text, label: X }\n"
	err := parseFormErr([]byte(doc))
	paths := problemPaths(t, err)
	if !hasPath(paths, "") || !strings.Contains(err.Error(), "title") {
		t.Fatalf("want root problem mentioning title, got %v (%v)", paths, err)
	}
}

func parseFormErr(b []byte) error     { _, err := ParseForm(b); return err }
func parseWorkflowErr(b []byte) error { _, err := ParseWorkflow(b); return err }
```

`internal/definition/bundle_test.go`:
```go
package definition

import "testing"

func TestValidateBundleAcceptsConsistentBundle(t *testing.T) {
	if err := ValidateBundle([]Form{baseForm()}, []Workflow{baseWorkflow()}, nil); err != nil {
		t.Fatalf("bundle should be valid: %v", err)
	}
}

func TestValidateBundleResolvesExistingWorkflows(t *testing.T) {
	if err := ValidateBundle([]Form{baseForm()}, nil, []string{"hiring"}); err != nil {
		t.Fatalf("existing workflow slug should resolve: %v", err)
	}
}

func TestValidateBundleProblems(t *testing.T) {
	badWorkflow := baseWorkflow()
	badWorkflow.Initial = "draft"
	dupForm := baseForm()
	orphan := baseForm()
	orphan.Slug = "orphan"
	orphan.Workflow = "missing"

	err := ValidateBundle(
		[]Form{baseForm(), dupForm, orphan},
		[]Workflow{baseWorkflow(), baseWorkflow(), badWorkflow},
		nil,
	)
	paths := problemPaths(t, err)
	for _, want := range []string{
		"forms[1].slug",      // duplicate form slug
		"forms[2].workflow",  // unknown workflow
		"workflows[1].slug",  // duplicate workflow slug
		"workflows[2].slug",  // duplicate workflow slug
		"workflows[2].initial", // nested semantic problem, prefixed
	} {
		if !hasPath(paths, want) {
			t.Errorf("missing %q in %v", want, paths)
		}
	}
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/definition/ -run 'TestParse|TestValidateBundle' -v`
Expected: FAIL — `undefined: ParseForm`, `undefined: ParseWorkflow`, `undefined: ValidateBundle`.

- [ ] **Step 4: Write `internal/definition/parse.go`**

```go
package definition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"sigs.k8s.io/yaml"

	"github.com/openforms/openforms/schemas"
)

const (
	formSchemaID     = "https://openforms.dev/schemas/form.schema.json"
	workflowSchemaID = "https://openforms.dev/schemas/workflow.schema.json"
)

var (
	schemaOnce     sync.Once
	schemaErr      error
	formSchema     *jsonschema.Schema
	workflowSchema *jsonschema.Schema
	printer        = message.NewPrinter(language.English)
)

func loadSchemas() error {
	schemaOnce.Do(func() {
		c := jsonschema.NewCompiler()
		for id, file := range map[string]string{formSchemaID: "form.schema.json", workflowSchemaID: "workflow.schema.json"} {
			b, err := schemas.FS.ReadFile(file)
			if err != nil {
				schemaErr = err
				return
			}
			doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(b))
			if err != nil {
				schemaErr = fmt.Errorf("%s: %w", file, err)
				return
			}
			if err := c.AddResource(id, doc); err != nil {
				schemaErr = err
				return
			}
		}
		if formSchema, schemaErr = c.Compile(formSchemaID); schemaErr != nil {
			return
		}
		workflowSchema, schemaErr = c.Compile(workflowSchemaID)
	})
	return schemaErr
}

// ParseForm parses a YAML or JSON form definition, validates it against
// form.schema.json and then against the semantic rules. Invalid input yields
// *ValidationError.
func ParseForm(raw []byte) (Form, error) {
	var f Form
	if err := decodeDocument(raw, formSchemaID, &f); err != nil {
		return Form{}, err
	}
	if err := ValidateForm(f); err != nil {
		return Form{}, err
	}
	return f, nil
}

// ParseWorkflow is ParseForm for workflow definitions.
func ParseWorkflow(raw []byte) (Workflow, error) {
	var w Workflow
	if err := decodeDocument(raw, workflowSchemaID, &w); err != nil {
		return Workflow{}, err
	}
	if err := ValidateWorkflow(w); err != nil {
		return Workflow{}, err
	}
	return w, nil
}

func rootProblem(msg string) error {
	return &ValidationError{Problems: []Problem{{Path: "", Message: msg}}}
}

func decodeDocument(raw []byte, schemaID string, out any) error {
	if err := loadSchemas(); err != nil {
		return fmt.Errorf("load schemas: %w", err)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return rootProblem("document is empty")
	}
	// Strict mode rejects duplicate keys instead of letting the last one win.
	js, err := yaml.YAMLToJSONStrict(raw)
	if err != nil {
		return rootProblem("invalid YAML/JSON: " + err.Error())
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(js))
	if err != nil {
		return rootProblem("invalid JSON: " + err.Error())
	}
	sch := formSchema
	if schemaID == workflowSchemaID {
		sch = workflowSchema
	}
	if err := sch.Validate(inst); err != nil {
		var ve *jsonschema.ValidationError
		if errors.As(err, &ve) {
			return &ValidationError{Problems: schemaProblems(ve)}
		}
		return err
	}
	if err := json.Unmarshal(js, out); err != nil {
		return rootProblem("invalid document: " + err.Error())
	}
	return nil
}

// schemaProblems flattens a jsonschema error tree into one Problem per leaf.
func schemaProblems(ve *jsonschema.ValidationError) []Problem {
	var out []Problem
	seen := map[Problem]bool{}
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			p := Problem{Path: instancePath(e.InstanceLocation), Message: e.ErrorKind.LocalizedString(printer)}
			if !seen[p] {
				seen[p] = true
				out = append(out, p)
			}
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// instancePath turns ["fields","0","showIf"] into "fields[0].showIf".
// Numeric tokens are array indices: object keys can never be all-digit
// because every key in the schemas matches a letter-first pattern.
func instancePath(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		if _, err := strconv.Atoi(t); err == nil {
			b.WriteString("[" + t + "]")
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(t)
	}
	return b.String()
}
```

> If `go build` reports that `ErrorKind` has no `LocalizedString` method, run `go doc github.com/santhosh-tekuri/jsonschema/v6 ErrorKind` and use the method it lists that takes a `*message.Printer` and returns `string`; the rest of the function is unchanged.

- [ ] **Step 5: Write `internal/definition/bundle.go`**

```go
package definition

import "fmt"

// ValidateBundle validates every definition in a bundle, rejects duplicate
// slugs, and checks that each form's workflow resolves to a workflow in the
// bundle or in existingWorkflowSlugs. Problem paths are prefixed with
// "forms[i]" / "workflows[i]".
func ValidateBundle(forms []Form, workflows []Workflow, existingWorkflowSlugs []string) error {
	var ps problems
	known := make(map[string]bool, len(existingWorkflowSlugs)+len(workflows))
	for _, s := range existingWorkflowSlugs {
		known[s] = true
	}

	seenW := make(map[string]bool, len(workflows))
	for i, w := range workflows {
		p := fmt.Sprintf("workflows[%d]", i)
		ps.merge(p, ValidateWorkflow(w))
		if seenW[w.Slug] {
			ps.add(p+".slug", fmt.Sprintf("duplicate workflow slug %q", w.Slug))
		}
		seenW[w.Slug] = true
		known[w.Slug] = true
	}

	seenF := make(map[string]bool, len(forms))
	for i, f := range forms {
		p := fmt.Sprintf("forms[%d]", i)
		ps.merge(p, ValidateForm(f))
		if seenF[f.Slug] {
			ps.add(p+".slug", fmt.Sprintf("duplicate form slug %q", f.Slug))
		}
		seenF[f.Slug] = true
		if f.Workflow != "" && !known[f.Workflow] {
			ps.add(p+".workflow", fmt.Sprintf("unknown workflow %q", f.Workflow))
		}
	}
	return ps.err()
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/definition/ -run 'TestParse|TestValidateBundle' -v`
Expected: PASS — `TestParseFormYAML`, `TestParseFormYAMLAndJSONAgree`, `TestParseWorkflowYAML`, `TestParseProblems` (8 subtests), `TestParseFormYAMLImplicitBoolean`, `TestParseFormRejectsDuplicateKeys`, `TestValidateBundleAcceptsConsistentBundle`, `TestValidateBundleResolvesExistingWorkflows`, `TestValidateBundleProblems`.

- [ ] **Step 7: Commit**

```bash
go mod tidy
git add go.mod go.sum internal/definition/parse.go internal/definition/bundle.go internal/definition/parse_test.go internal/definition/bundle_test.go
git commit -m "feat(definition): parse YAML/JSON definitions and validate bundles"
```

---

### Task 8: Visibility and submission validation (with conformance fixtures)

**Parallel group B** (with Tasks 7, 9 — disjoint files). Requires Tasks 2, 4, 6.

**Files:**
- Create: `internal/definition/submission.go`
- Test: `internal/definition/submission_test.go`

**Interfaces:**
- Consumes: `schemas.Fixtures` (Task 2), `ValidateForm` (Task 4), `checkValue`, `jsonEqual`, `normalizeJSON` (Task 6), `problems` (Task 1).
- Produces: `func Visible(f Form, data map[string]any) map[string]bool` (every field key present), `func ValidateSubmission(f Form, data map[string]any) (map[string]any, error)` — cleaned data (never nil on success, never aliases `data`) or `*ValidationError` with paths `data.<key>` in field order. Used by Plan 03 (`submissions.Service.Create`).

- [ ] **Step 1: Write the failing test**

`internal/definition/submission_test.go`:
```go
package definition

import (
	"encoding/json"
	"reflect"
	"sort"
	"testing"

	"github.com/openforms/openforms/schemas"
)

type visibilityFixture struct {
	Name    string          `json:"name"`
	Form    json.RawMessage `json:"form"`
	Data    map[string]any  `json:"data"`
	Visible map[string]bool `json:"visible"`
}

type submissionFixture struct {
	Name       string          `json:"name"`
	Form       json.RawMessage `json:"form"`
	Data       map[string]any  `json:"data"`
	Clean      map[string]any  `json:"clean"` // nil when the fixture has "clean": null
	ErrorPaths []string        `json:"errorPaths"`
}

func readFixture(t *testing.T, name string, out any) {
	t.Helper()
	b, err := schemas.Fixtures.ReadFile("fixtures/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
}

func fixtureForm(t *testing.T, raw json.RawMessage) Form {
	t.Helper()
	var f Form
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("fixture form: %v", err)
	}
	if err := ValidateForm(f); err != nil {
		t.Fatalf("fixture form is not a valid definition: %v", err)
	}
	return f
}

func TestVisibilityFixtures(t *testing.T) {
	var cases []visibilityFixture
	readFixture(t, "visibility.json", &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			got := Visible(fixtureForm(t, c.Form), c.Data)
			if !reflect.DeepEqual(got, c.Visible) {
				t.Fatalf("visible = %v, want %v", got, c.Visible)
			}
		})
	}
}

func TestSubmissionFixtures(t *testing.T) {
	var cases []submissionFixture
	readFixture(t, "submission.json", &cases)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			clean, err := ValidateSubmission(fixtureForm(t, c.Form), c.Data)
			if c.Clean != nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !jsonEqual(clean, c.Clean) {
					t.Fatalf("clean = %#v, want %#v", clean, c.Clean)
				}
				return
			}
			got := problemPaths(t, err) // sorted
			want := append([]string(nil), c.ErrorPaths...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("error paths = %v, want %v (%v)", got, want, err)
			}
		})
	}
}

func TestVisibleNilData(t *testing.T) {
	f := Form{Slug: "t", Title: "T", Fields: []Field{
		{Key: "a", Type: FieldText, Label: "A"},
		{Key: "b", Type: FieldText, Label: "B", ShowIf: &Condition{Field: "a", Equals: "x"}},
	}}
	got := Visible(f, nil)
	if !got["a"] || got["b"] {
		t.Fatalf("got %v", got)
	}
}

func TestValidateSubmissionDoesNotAliasInput(t *testing.T) {
	f := Form{Slug: "t", Title: "T", Fields: []Field{
		{Key: "skills", Type: FieldMultiselect, Label: "Skills", Options: []Option{{Value: "go", Label: "Go"}}},
	}}
	in := map[string]any{"skills": []any{"go"}, "extra": 1}
	clean, err := ValidateSubmission(f, in)
	if err != nil {
		t.Fatal(err)
	}
	clean["skills"].([]any)[0] = "changed"
	if in["skills"].([]any)[0] != "go" {
		t.Fatal("clean data must not share slices with the input")
	}
	if _, ok := in["extra"]; !ok {
		t.Fatal("input map must not be mutated")
	}
}

func TestValidateSubmissionEmptyCleanIsNonNil(t *testing.T) {
	f := Form{Slug: "t", Title: "T", Fields: []Field{{Key: "a", Type: FieldText, Label: "A"}}}
	clean, err := ValidateSubmission(f, map[string]any{})
	if err != nil || clean == nil {
		t.Fatalf("clean=%v err=%v", clean, err)
	}
}

func TestValidateSubmissionProblemsInFieldOrder(t *testing.T) {
	f := Form{Slug: "t", Title: "T", Fields: []Field{
		{Key: "z", Type: FieldText, Label: "Z", Required: true},
		{Key: "a", Type: FieldText, Label: "A", Required: true},
	}}
	_, err := ValidateSubmission(f, nil)
	ve, ok := err.(*ValidationError)
	if !ok || len(ve.Problems) != 2 || ve.Problems[0].Path != "data.z" || ve.Problems[1].Path != "data.a" {
		t.Fatalf("got %v", err)
	}
	if ve.Problems[0].Message != "is required" {
		t.Fatalf("message = %q", ve.Problems[0].Message)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run 'TestVisib|TestSubmission|TestValidateSubmission' -v`
Expected: FAIL — `undefined: Visible`, `undefined: ValidateSubmission`.

- [ ] **Step 3: Write the implementation**

`internal/definition/submission.go`:
```go
package definition

// Visible reports, for every field key, whether the field is shown for data
// (spec §5.4). It shares one evaluator with ValidateSubmission so the two can
// never disagree.
func Visible(f Form, data map[string]any) map[string]bool {
	visible, _, _ := evaluate(f, data)
	return visible
}

// ValidateSubmission cleans data (drops unknown keys, hidden fields and
// not-provided values; trims strings; normalizes numbers and lists) and
// validates every visible field. It returns the cleaned data or a
// *ValidationError whose problems use paths "data.<key>" in field order.
func ValidateSubmission(f Form, data map[string]any) (map[string]any, error) {
	_, clean, ps := evaluate(f, data)
	if err := ps.err(); err != nil {
		return nil, err
	}
	return clean, nil
}

// evaluate walks the fields in declaration order. A field's condition is
// evaluated against the cleaned values of earlier fields; a hidden controller
// hides its dependants; an invalid value counts as absent.
func evaluate(f Form, data map[string]any) (map[string]bool, map[string]any, problems) {
	visible := make(map[string]bool, len(f.Fields))
	clean := make(map[string]any, len(f.Fields))
	types := make(map[string]FieldType, len(f.Fields))
	var ps problems
	for _, fld := range f.Fields {
		types[fld.Key] = fld.Type
		vis := true
		if c := fld.ShowIf; c != nil {
			if !visible[c.Field] {
				vis = false
			} else {
				val, present := clean[c.Field]
				vis = holds(*c, types[c.Field], val, present)
			}
		}
		visible[fld.Key] = vis
		if !vis {
			continue
		}
		path := "data." + fld.Key
		val, provided, msg := checkValue(fld.Type, fld.Options, fld.Validation, data[fld.Key])
		switch {
		case msg != "":
			ps.add(path, msg)
		case !provided:
			if fld.Required {
				ps.add(path, "is required")
			}
		case fld.Type == FieldCheckbox && fld.Required && val == false:
			ps.add(path, "must be checked")
		default:
			clean[fld.Key] = val
		}
	}
	return visible, clean, ps
}

// holds evaluates a condition against the controller's cleaned value.
func holds(c Condition, ctrl FieldType, val any, present bool) bool {
	multi := ctrl == FieldMultiselect
	switch {
	case c.Equals != nil:
		if !present {
			return false
		}
		if multi {
			return containsJSON(val.([]any), c.Equals)
		}
		return jsonEqual(val, c.Equals)
	case c.NotEquals != nil:
		if !present {
			return true
		}
		if multi {
			return !containsJSON(val.([]any), c.NotEquals)
		}
		return !jsonEqual(val, c.NotEquals)
	case c.In != nil:
		if !present {
			return false
		}
		if multi {
			for _, item := range val.([]any) {
				if containsJSON(c.In, item) {
					return true
				}
			}
			return false
		}
		return containsJSON(c.In, val)
	}
	return true
}

func containsJSON(list []any, v any) bool {
	for _, it := range list {
		if jsonEqual(it, v) {
			return true
		}
	}
	return false
}
```

`checkValue` builds fresh values (a new `[]any` for multiselect, new strings), so `clean` never aliases `data`.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run 'TestVisib|TestSubmission|TestValidateSubmission' -v`
Expected: PASS — `TestVisibilityFixtures` (21 subtests), `TestSubmissionFixtures` (46 subtests), `TestVisibleNilData`, `TestValidateSubmissionDoesNotAliasInput`, `TestValidateSubmissionEmptyCleanIsNonNil`, `TestValidateSubmissionProblemsInFieldOrder`.

- [ ] **Step 5: Commit**

```bash
git add internal/definition/submission.go internal/definition/submission_test.go
git commit -m "feat(definition): add visibility and submission validation passing conformance fixtures"
```

---

### Task 9: Workflow field validation

**Parallel group B** (with Tasks 7, 8 — disjoint files). Requires Task 6.

**Files:**
- Create: `internal/definition/workflow_fields.go`
- Test: `internal/definition/workflow_fields_test.go`

**Interfaces:**
- Consumes: `Workflow.Field` (Task 1), `checkValue` (Task 6), `problems` (Task 1).
- Produces: `func ValidateWorkflowFields(w Workflow, fields map[string]any) (map[string]any, error)` — validates a **patch**. Returns a new map with the same keys: normalized values, or `nil` for keys whose value is `null`/blank (meaning "unset"). Unknown keys and type errors produce `*ValidationError` with paths `fields.<key>`, sorted by key. Callers merge with `nil` → `delete`. Used by Plan 04 (`workflow.Engine.Transition`, `UpdateFields`).

- [ ] **Step 1: Write the failing test**

`internal/definition/workflow_fields_test.go`:
```go
package definition

import (
	"reflect"
	"testing"
)

func fieldsWorkflow() Workflow {
	return Workflow{
		Slug: "w", Title: "W", Initial: "new",
		States: []State{{Key: "new", Label: "New"}},
		Fields: []WorkflowField{
			{Key: "score", Type: FieldNumber, Label: "Score"},
			{Key: "rejectionReason", Type: FieldTextarea, Label: "Reason"},
			{Key: "source", Type: FieldSelect, Label: "Source", Options: []Option{{Value: "referral", Label: "Referral"}}},
			{Key: "flagged", Type: FieldCheckbox, Label: "Flagged"},
			{Key: "followUp", Type: FieldDate, Label: "Follow up"},
		},
	}
}

func TestValidateWorkflowFieldsNormalizes(t *testing.T) {
	got, err := ValidateWorkflowFields(fieldsWorkflow(), map[string]any{
		"score":           4,
		"source":          " referral ",
		"flagged":         false,
		"followUp":        "2026-10-01",
		"rejectionReason": nil,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"score":           4.0,
		"source":          "referral",
		"flagged":         false,
		"followUp":        "2026-10-01",
		"rejectionReason": nil,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestValidateWorkflowFieldsBlankMeansUnset(t *testing.T) {
	got, err := ValidateWorkflowFields(fieldsWorkflow(), map[string]any{"rejectionReason": "   "})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := got["rejectionReason"]
	if !ok || v != nil {
		t.Fatalf("blank value must map to nil (unset), got %#v present=%v", v, ok)
	}
}

func TestValidateWorkflowFieldsProblems(t *testing.T) {
	_, err := ValidateWorkflowFields(fieldsWorkflow(), map[string]any{
		"zeta":   1,
		"alpha":  "x",
		"score":  "4",
		"source": "website",
		"flagged": "yes",
	})
	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("want *ValidationError, got %v", err)
	}
	var paths []string
	for _, p := range ve.Problems {
		paths = append(paths, p.Path)
	}
	want := []string{"fields.alpha", "fields.flagged", "fields.score", "fields.source", "fields.zeta"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v (sorted by key)", paths, want)
	}
	if ve.Problems[0].Message != "unknown workflow field" {
		t.Fatalf("message = %q", ve.Problems[0].Message)
	}
}

func TestValidateWorkflowFieldsEmptyPatch(t *testing.T) {
	got, err := ValidateWorkflowFields(fieldsWorkflow(), nil)
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %v, %v", got, err)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/definition/ -run TestValidateWorkflowFields -v`
Expected: FAIL — `undefined: ValidateWorkflowFields`.

- [ ] **Step 3: Write the implementation**

`internal/definition/workflow_fields.go`:
```go
package definition

import "sort"

// ValidateWorkflowFields validates a patch of workflow-managed field values
// against w.Fields. The result has the same keys as fields: a normalized value,
// or nil when the patch sets the field to null or a blank string ("unset").
// Callers merge it into a submission's fields: nil → delete the key,
// otherwise set it. Problems use paths "fields.<key>", sorted by key.
func ValidateWorkflowFields(w Workflow, fields map[string]any) (map[string]any, error) {
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make(map[string]any, len(fields))
	var ps problems
	for _, k := range keys {
		path := "fields." + k
		wf, ok := w.Field(k)
		if !ok {
			ps.add(path, "unknown workflow field")
			continue
		}
		val, provided, msg := checkValue(wf.Type, wf.Options, nil, fields[k])
		switch {
		case msg != "":
			ps.add(path, msg)
		case !provided:
			out[k] = nil
		default:
			out[k] = val
		}
	}
	if err := ps.err(); err != nil {
		return nil, err
	}
	return out, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/definition/ -run TestValidateWorkflowFields -v`
Expected: PASS — 4 tests.

- [ ] **Step 5: Commit**

```bash
git add internal/definition/workflow_fields.go internal/definition/workflow_fields_test.go
git commit -m "feat(definition): validate workflow field patches"
```

---

## Final verification (after merging Parallel group B)

- [ ] **Step 1: Format, vet and run the whole plan's test suite**

Run:
```bash
gofmt -l schemas internal/definition
go vet ./schemas/... ./internal/definition/...
go test ./schemas/... ./internal/definition/... -count=1
```
Expected: `gofmt -l` prints nothing; `go vet` prints nothing; `ok  github.com/openforms/openforms/schemas` and `ok  github.com/openforms/openforms/internal/definition`.

- [ ] **Step 2: Confirm the exported surface matches spec §6.4**

Run: `go doc -short ./internal/definition`
Expected output includes exactly these exported functions (plus the types and constants from Task 1):
```
func Canonical(v any) ([]byte, string, error)
func ParseForm(raw []byte) (Form, error)
func ParseWorkflow(raw []byte) (Workflow, error)
func ValidateBundle(forms []Form, workflows []Workflow, existingWorkflowSlugs []string) error
func ValidateForm(f Form) error
func ValidateSubmission(f Form, data map[string]any) (map[string]any, error)
func ValidateWorkflow(w Workflow) error
func ValidateWorkflowFields(w Workflow, fields map[string]any) (map[string]any, error)
func Visible(f Form, data map[string]any) map[string]bool
```

- [ ] **Step 3: Hand-off note for the Wave 1 merge**

Lane B changed `go.mod`/`go.sum`. When merging into `main` after Plan 01 (Lane A), resolve any `go.mod` conflict by keeping both sides' requirements and running `go mod tidy`, then `go test ./...`. No commit is needed here unless the merge produced changes:

```bash
go mod tidy && go test ./... && git add go.mod go.sum && git commit -m "chore: tidy modules after wave 1 merge"
```

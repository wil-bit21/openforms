# openforms Plan 09 — Demo, Packaging, Docs & E2E Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. Tasks marked with the same **Parallel group** touch disjoint files and may be dispatched to concurrent subagents (superpowers:dispatching-parallel-agents), each in its own worktree.

**Goal:** Turn the finished platform into a shippable product: an example bundle and `openforms seed --demo`, a public `/api/v1/public/config` endpoint with demo credentials on the admin login page, a single Docker image plus compose stack and CI, a polished `/demo` page, complete user docs, and Playwright E2E tests covering the full submit → review → notify loop.

**Architecture:** The demo bundle is plain YAML under `examples/openforms/`, embedded into the Go binary by the `examples` package and applied through `definitions.Store.Apply` with `source=seed`. The `internal/seed` package owns the idempotent seeding logic; the CLI (`seed --demo`) and `app.New` (`OPENFORMS_SEED_DEMO=true`) both call it. The `/demo` page is a Vite React app in `web/apps/demo` that uses the headless SDK and `@openforms/react` against the same server and imports the example YAML with Vite `?raw` imports so what visitors read is what the server runs. E2E tests drive the `docker compose --profile app` stack from the host with Playwright. A webhook sink in the Playwright global setup records deliveries, and Mailpit's HTTP API is used to assert emails.

**Tech Stack:** Go ≥ 1.24 (cobra, pgx, chi), TypeScript 5, React 18, Vite 6, Vitest 3, @testing-library/react, MSW 2, shiki, Playwright, Docker (multi-stage, distroless), docker compose, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-09-23-openforms-design.md`. Binding sections for this plan: §3 (layout + web build contract), §5.1/§5.2 (definition formats), §7 (API), §8 (`seed --demo`), §9.6 (demo page), §10 (demo & packaging), §12 (E2E list). Also read the roadmap: `docs/superpowers/plans/2026-09-23-openforms-00-roadmap.md`.

## Global Constraints

- Go ≥ 1.24, module `github.com/openforms/openforms`; Node 22 LTS; pnpm 9; PostgreSQL 16; React 18; Vite 6.
- All JSON over the wire is camelCase; every API error uses the envelope `{"error": {"code", "message", "details"?}}`.
- Web apps set Vite `base: "/_app/<name>/"`, so the demo uses `base: "/_app/demo/"`; `pnpm -C web build` copies `apps/demo/dist` to `internal/webui/dist/demo` (via Plan 06's `scripts/copy-dist.mjs`); the Go server serves `/demo` and `/demo/*` from `dist/demo/index.html`.
- Demo users (password `demo1234`): `admin@demo.local` [admin], `reviewer@demo.local` [reviewer], `manager@demo.local` [hiring-manager].
- `/admin/login` shows demo credentials only when `GET /api/v1/public/config` returns `{"demo": true}`.
- Dockerfile: stage 1 `node:22` → `pnpm install --frozen-lockfile && pnpm -C web build`; stage 2 `golang:1.24` → `go build -o /openforms ./cmd/openforms` with dist embedded; stage 3 `gcr.io/distroless/static-debian12` running `openforms serve`.
- `docker compose --profile app up` runs postgres + mailpit + openforms (demo mode; seeds on start via `OPENFORMS_SEED_DEMO=true`).
- Makefile targets: `dev-db`, `test`, `web`, `web-test`, `build`, `e2e`, `lint`.
- Demo page copy when not seeded: "Run `openforms seed --demo` to enable the live demo".
- No placeholder copy ("Lorem ipsum") in shipped pages.
- Commit after every task with conventional prefixes (`feat:`, `fix:`, `test:`, `chore:`, `docs:`).
- Windows dev machine: run every command from Git Bash. Makefile recipes are POSIX shell and **must be indented with a tab character**.

## Review Focus

1. **Seeding twice** (container restart with `OPENFORMS_SEED_DEMO=true`, or `seed --demo` after an auto-seed) must create no new versions and no duplicate users, and must not error, even if a demo user already exists with a different password. Tests: Task 2 `TestDemoIsIdempotent` and `TestDemoKeepsExistingUser`.
2. **Demo page against a server that is unseeded or unreachable** should show the "Run `openforms seed --demo`…" hint or a "Server unreachable" notice, and never a blank page or an uncaught error. Tests: Task 5 `App.test.tsx`, which covers both cases.
3. **Reviewer panel when the server is not in demo mode** must never call `/auth/login` with demo credentials and must explain why. Test: Task 5 `ReviewerPanel.test.tsx` "does not log in outside demo mode".
4. **A transition the persona may not perform** (a reviewer trying to Hire) must be shown disabled with a reason, and must work after switching to the Hiring manager persona. Test: Task 5 `ReviewerPanel.test.tsx` "role-guarded transition".
5. **Demo mode off:** the admin login page must not show demo credentials, and a Docker build whose web bundle is missing must fail rather than ship an image that answers 503. Tests: Task 3 `DemoCredentials.test.tsx` "renders nothing when demo is false" and the Task 4 Dockerfile `test -f` guard, verified in Step 4.3.

## Execution order & parallelism

| Wave | Tasks | Notes |
|---|---|---|
| Wave 4 (parallel with Plan 08) | **Parallel group W4:** Task 1, Task 3, Task 4. Then Task 2 (needs Task 1). | Task 1: `examples/`. Task 3: `internal/httpapi/public_config*`, `routes.go` (one line), SDK, admin `DemoCredentials`. Task 4: `Dockerfile`, `.dockerignore`, `docker-compose.yml`, `Makefile`, `.github/`. Task 2: `internal/seed/`, `internal/cli/seed*.go`, `internal/cli/root.go` (one line), `internal/app/app.go` (one block). |
| Wave 5 (after Plans 01–08 and Wave 4 are merged) | **Parallel group W5:** Task 5, Task 6. Then Task 7. | Task 5: `web/apps/demo/`. Task 6: `README.md` and `docs/*.md` (not `docs/superpowers`). Task 7: `e2e/` (its demo spec needs Task 5). |

After Wave 4 merges into `main`, run the **Wave 4 merge check**:
`docker compose --profile app up -d --build && curl -fsS http://localhost:8080/api/v1/public/forms/job-application | head -c 200`
Expected: JSON starting with `{"form":{"slug":"job-application"`.

---

### Task 1: Example bundle (`examples/`)

**Parallel group:** W4

**Files:**
- Create: `examples/openforms/forms/job-application.yaml`
- Create: `examples/openforms/forms/contact.yaml`
- Create: `examples/openforms/workflows/hiring.yaml`
- Create: `examples/openforms/workflows/contact-triage.yaml`
- Create: `examples/examples.go`
- Test: `examples/examples_test.go`

**Interfaces:**
- Consumes: `definition.ParseForm(raw []byte) (definition.Form, error)`, `definition.ParseWorkflow(raw []byte) (definition.Workflow, error)`, `definition.ValidateBundle(forms, workflows, existingWorkflowSlugs []string) error` (Plan 02).
- Produces: package `github.com/openforms/openforms/examples` exposing `var FS embed.FS` (root dir `openforms/`) and `func Bundle() ([]definition.Form, []definition.Workflow, error)` (sorted by file name). Used by Task 2 (`internal/seed`), and the YAML files are imported raw by Task 5 (`web/apps/demo/src/sources.ts`).

> **Deviation from spec §5.2, applied on purpose:** the spec's illustrative `hiring` workflow has a webhook action pointing at `https://example.com/hooks/interview`. A demo stack cannot deliver to that URL, so every "Invite" would end in an `action_failed` event on the demo timeline. The seeded `hiring.yaml` therefore leaves that webhook out. Webhooks are exercised by the E2E bundle in Task 7, which points them at a local sink.

- [ ] **Step 1.1: Write the failing test**

Create `examples/examples_test.go`:

```go
package examples_test

import (
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openforms/openforms/examples"
	"github.com/openforms/openforms/internal/definition"
)

func TestBundleIsValid(t *testing.T) {
	forms, workflows, err := examples.Bundle()
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	if err := definition.ValidateBundle(forms, workflows, nil); err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}
	var formSlugs, workflowSlugs []string
	for _, f := range forms {
		formSlugs = append(formSlugs, f.Slug)
	}
	for _, w := range workflows {
		workflowSlugs = append(workflowSlugs, w.Slug)
	}
	if want := []string{"contact", "job-application"}; !reflect.DeepEqual(formSlugs, want) {
		t.Errorf("form slugs = %v, want %v", formSlugs, want)
	}
	if want := []string{"contact-triage", "hiring"}; !reflect.DeepEqual(workflowSlugs, want) {
		t.Errorf("workflow slugs = %v, want %v", workflowSlugs, want)
	}
}

func TestSlugsMatchFileNames(t *testing.T) {
	for _, dir := range []string{"openforms/forms", "openforms/workflows"} {
		entries, err := fs.ReadDir(examples.FS, dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}
		for _, e := range entries {
			raw, err := fs.ReadFile(examples.FS, path.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			base := strings.TrimSuffix(e.Name(), ".yaml")
			var slug string
			if strings.HasSuffix(dir, "forms") {
				f, err := definition.ParseForm(raw)
				if err != nil {
					t.Fatalf("%s: %v", e.Name(), err)
				}
				slug = f.Slug
			} else {
				w, err := definition.ParseWorkflow(raw)
				if err != nil {
					t.Fatalf("%s: %v", e.Name(), err)
				}
				slug = w.Slug
			}
			if slug != base {
				t.Errorf("%s/%s: slug %q does not match file name", dir, e.Name(), slug)
			}
		}
	}
}

func findForm(t *testing.T, slug string) definition.Form {
	t.Helper()
	forms, _, err := examples.Bundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range forms {
		if f.Slug == slug {
			return f
		}
	}
	t.Fatalf("form %q not found", slug)
	return definition.Form{}
}

func findWorkflow(t *testing.T, slug string) definition.Workflow {
	t.Helper()
	_, workflows, err := examples.Bundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range workflows {
		if w.Slug == slug {
			return w
		}
	}
	t.Fatalf("workflow %q not found", slug)
	return definition.Workflow{}
}

func TestJobApplicationShape(t *testing.T) {
	f := findForm(t, "job-application")
	var keys []string
	for _, fld := range f.Fields {
		keys = append(keys, fld.Key)
	}
	want := []string{"name", "email", "role", "years", "portfolio", "coverLetter", "consent"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("field keys = %v, want %v", keys, want)
	}
	if f.Workflow != "hiring" || !f.Settings.Public {
		t.Errorf("workflow=%q public=%v, want hiring/true", f.Workflow, f.Settings.Public)
	}
	portfolio := f.Fields[4]
	if portfolio.ShowIf == nil || portfolio.ShowIf.Field != "role" || portfolio.ShowIf.Equals != "designer" {
		t.Errorf("portfolio.showIf = %+v, want role == designer", portfolio.ShowIf)
	}
	if f.Fields[6].Type != definition.FieldCheckbox || !f.Fields[6].Required {
		t.Errorf("consent must be a required checkbox, got %+v", f.Fields[6])
	}
}

func TestHiringWorkflowShape(t *testing.T) {
	w := findWorkflow(t, "hiring")
	var states, transitions []string
	for _, s := range w.States {
		states = append(states, s.Key)
	}
	for _, tr := range w.Transitions {
		transitions = append(transitions, tr.Key)
	}
	if want := []string{"new", "screening", "interview", "hired", "rejected"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	if want := []string{"screen", "invite", "hire", "reject"}; !reflect.DeepEqual(transitions, want) {
		t.Errorf("transitions = %v, want %v", transitions, want)
	}
	reject, _ := w.Transition("reject")
	if !reflect.DeepEqual(reject.Guard.RequireFields, []string{"rejectionReason"}) {
		t.Errorf("reject.requireFields = %v", reject.Guard.RequireFields)
	}
	hire, _ := w.Transition("hire")
	if !reflect.DeepEqual(hire.Guard.Roles, []string{"hiring-manager"}) {
		t.Errorf("hire.roles = %v", hire.Guard.Roles)
	}
	if len(w.OnSubmit) != 2 {
		t.Errorf("onSubmit actions = %d, want 2", len(w.OnSubmit))
	}
	for _, tr := range w.Transitions {
		for _, a := range tr.Actions {
			if a.Type == definition.ActionWebhook {
				t.Errorf("transition %q has a webhook; demo bundle must not call external URLs", tr.Key)
			}
		}
	}
}

func TestContactTriageShape(t *testing.T) {
	w := findWorkflow(t, "contact-triage")
	var states []string
	for _, s := range w.States {
		states = append(states, s.Key)
	}
	sort.Strings(states)
	if want := []string{"in_progress", "open", "resolved", "spam"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	f := findForm(t, "contact")
	if f.Workflow != "contact-triage" {
		t.Errorf("contact.workflow = %q", f.Workflow)
	}
}
```

- [ ] **Step 1.2: Run the test to verify it fails**

Run: `go test ./examples/...`
Expected: FAIL — a build error such as `no non-test Go files in .../examples` or `undefined: examples.Bundle`.

- [ ] **Step 1.3: Write the YAML bundle**

Create `examples/openforms/forms/job-application.yaml`:

```yaml
slug: job-application
title: Job application
description: Apply to join the team. It takes about two minutes.
workflow: hiring
settings:
  public: true
  submitLabel: Send application
  confirmationMessage: Thanks for applying! We'll review your application and keep you posted.
fields:
  - key: name
    type: text
    label: Full name
    required: true
    placeholder: Ada Lovelace
    validation: { minLength: 2, maxLength: 100 }
  - key: email
    type: email
    label: Email
    required: true
    placeholder: ada@example.com
    help: We'll send status updates here.
  - key: role
    type: select
    label: Role
    required: true
    options:
      - { value: engineer, label: Engineer }
      - { value: designer, label: Designer }
      - { value: product, label: Product manager }
  - key: years
    type: number
    label: Years of experience
    required: true
    validation: { min: 0, max: 50 }
  - key: portfolio
    type: url
    label: Portfolio URL
    help: A link to your best work.
    required: true
    showIf: { field: role, equals: designer }
  - key: coverLetter
    type: textarea
    label: Why do you want to join?
    placeholder: A few sentences are plenty.
    validation: { maxLength: 2000 }
  - key: consent
    type: checkbox
    label: I agree to my data being stored for this application.
    required: true
```

Create `examples/openforms/workflows/hiring.yaml`:

```yaml
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
  - type: email
    to: "{{submission.data.email}}"
    subject: We got your application
    body: |
      Hi {{submission.data.name}},

      Thanks for applying to join us. Your application is now in our hiring pipeline
      and a reviewer has been assigned. We'll email you whenever its status changes.

      — The hiring team
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
      - type: email
        to: "{{submission.data.email}}"
        subject: Let's talk — interview invitation
        body: |
          Hi {{submission.data.name}},

          We enjoyed reading your application and would love to meet you.
          We'll follow up shortly with times that work.

          — The hiring team
  - key: hire
    label: Hire
    from: [interview]
    to: hired
    guard: { roles: [hiring-manager] }
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: Welcome aboard!
        body: |
          Hi {{submission.data.name}},

          We're delighted to offer you the position. Expect a formal offer in your inbox soon.

          — The hiring team
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: Your application
        body: |
          Hi {{submission.data.name}},

          Thank you for your interest. {{submission.fields.rejectionReason}}

          — The hiring team
```

Create `examples/openforms/forms/contact.yaml`:

```yaml
slug: contact
title: Contact us
description: Questions, feedback or a bug to report — we read every message.
workflow: contact-triage
settings:
  public: true
  submitLabel: Send message
  confirmationMessage: Thanks! We usually reply within one business day.
fields:
  - key: name
    type: text
    label: Your name
    required: true
    validation: { maxLength: 100 }
  - key: email
    type: email
    label: Email
    required: true
  - key: topic
    type: select
    label: Topic
    required: true
    options:
      - { value: question, label: A question }
      - { value: feedback, label: Feedback }
      - { value: bug, label: A bug }
  - key: message
    type: textarea
    label: Message
    required: true
    validation: { minLength: 10, maxLength: 5000 }
```

Create `examples/openforms/workflows/contact-triage.yaml`:

```yaml
slug: contact-triage
title: Contact triage
initial: open
states:
  - { key: open, label: Open, color: blue }
  - { key: in_progress, label: In progress, color: yellow }
  - { key: resolved, label: Resolved, color: green, terminal: true }
  - { key: spam, label: Spam, color: gray, terminal: true }
fields:
  - { key: reply, type: textarea, label: Reply }
onSubmit:
  - { type: assign, role: reviewer }
transitions:
  - key: start
    label: Start working
    from: [open]
    to: in_progress
    guard: { roles: [reviewer] }
  - key: resolve
    label: Resolve
    from: [open, in_progress]
    to: resolved
    guard: { roles: [reviewer], requireFields: [reply] }
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: "Re: your message"
        body: |
          Hi {{submission.data.name}},

          {{submission.fields.reply}}

          — The team
  - key: spam
    label: Mark as spam
    from: [open]
    to: spam
    guard: { roles: [reviewer] }
```

- [ ] **Step 1.4: Write the embedding package**

Create `examples/examples.go`:

```go
// Package examples embeds the demo bundle used by `openforms seed --demo`
// and displayed on the /demo page.
package examples

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

// FS holds the bundle rooted at "openforms/" (forms/ and workflows/ subdirectories).
//
//go:embed openforms
var FS embed.FS

// Bundle parses every embedded form and workflow, sorted by file name.
func Bundle() ([]definition.Form, []definition.Workflow, error) {
	forms, err := parseDir(FS, "openforms/forms", definition.ParseForm)
	if err != nil {
		return nil, nil, err
	}
	workflows, err := parseDir(FS, "openforms/workflows", definition.ParseWorkflow)
	if err != nil {
		return nil, nil, err
	}
	return forms, workflows, nil
}

func parseDir[T any](fsys fs.FS, dir string, parse func([]byte) (T, error)) ([]T, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]T, 0, len(names))
	for _, name := range names {
		p := path.Join(dir, name)
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		v, err := parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}
```

- [ ] **Step 1.5: Run the tests to verify they pass**

Run: `go test ./examples/... -v`
Expected: PASS for `TestBundleIsValid`, `TestSlugsMatchFileNames`, `TestJobApplicationShape`, `TestHiringWorkflowShape`, `TestContactTriageShape`. If `ParseForm` reports a problem, fix the YAML, not the test. The problem path points at the offending line.

- [ ] **Step 1.6: Validate with the CLI too (Plan 05 command)**

Run: `go run ./cmd/openforms validate --dir examples/openforms`
Expected: exit 0 and no problems printed.

- [ ] **Step 1.7: Commit**

```bash
git add examples/
git commit -m "feat(examples): add job-application and contact demo bundle"
```

---

### Task 2: `openforms seed --demo` and auto-seed on serve

**Depends on:** Task 1.

**Files:**
- Create: `internal/seed/seed.go`
- Test: `internal/seed/seed_test.go`
- Create: `internal/cli/seed.go`
- Test: `internal/cli/seed_test.go`
- Modify: `internal/cli/root.go` (register the command, one line)
- Modify: `internal/app/app.go` (auto-seed block in `New`)

**Interfaces:**
- Consumes: `examples.Bundle()` (Task 1); `auth.Service.{EnsureDefaultOrg, GetUserByEmail, CreateUser, Login}`, `auth.ErrNotFound`, `auth.ErrEmailTaken`, `auth.RoleAdmin` (Plan 01); `definitions.NewStore`, `Store.Apply`, `ApplyInput{Forms, Workflows, Source, Actor}`, `definitions.SourceSeed`, `ApplyResult{Items []ApplyItem{Kind, Slug, Version, Changed, Created}}`, `Store.GetForm`, `Store.FormVersions` (Plan 03); `db.Open`, `db.Migrate`, `dbtest.New`, `testutil.NewEnv` (Plan 01); `config.Load` (Plan 01).
- Produces:
  ```go
  package seed
  const DemoPassword = "demo1234"
  type DemoUser struct { Email, Name string; Roles []string }
  var DemoUsers []DemoUser // admin@demo.local, reviewer@demo.local, manager@demo.local
  type Result struct { Apply definitions.ApplyResult; UsersCreated []string }
  func Demo(ctx context.Context, authSvc *auth.Service, defs *definitions.Store, orgID uuid.UUID) (Result, error)
  func EnabledFromEnv() bool // OPENFORMS_SEED_DEMO is "true" or "1" (case-insensitive)
  ```
  plus the CLI command `openforms seed --demo`.

- [ ] **Step 2.1: Write the failing seed package tests**

Create `internal/seed/seed_test.go`:

```go
package seed_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/seed"
	"github.com/openforms/openforms/internal/testutil"
)

func TestDemoSeedsBundleAndUsers(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	defs := definitions.NewStore(env.Pool)

	res, err := seed.Demo(ctx, env.Auth, defs, env.OrgID)
	if err != nil {
		t.Fatalf("Demo: %v", err)
	}
	if len(res.Apply.Items) != 4 {
		t.Fatalf("apply items = %d, want 4: %+v", len(res.Apply.Items), res.Apply.Items)
	}
	for _, it := range res.Apply.Items {
		if !it.Created || it.Version != 1 {
			t.Errorf("item %s/%s: created=%v version=%d, want created v1", it.Kind, it.Slug, it.Created, it.Version)
		}
	}
	if len(res.UsersCreated) != 3 {
		t.Errorf("users created = %v, want 3", res.UsersCreated)
	}
	for _, du := range seed.DemoUsers {
		u, err := env.Auth.GetUserByEmail(ctx, env.OrgID, du.Email)
		if err != nil {
			t.Fatalf("GetUserByEmail(%s): %v", du.Email, err)
		}
		if !reflect.DeepEqual(u.Roles, du.Roles) {
			t.Errorf("%s roles = %v, want %v", du.Email, u.Roles, du.Roles)
		}
	}
	rec, err := defs.GetForm(ctx, env.OrgID, "job-application")
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}
	if rec.Current.Source != definitions.SourceSeed {
		t.Errorf("source = %q, want seed", rec.Current.Source)
	}
	if _, _, err := env.Auth.Login(ctx, "reviewer@demo.local", seed.DemoPassword); err != nil {
		t.Errorf("reviewer login: %v", err)
	}
}

func TestDemoIsIdempotent(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	defs := definitions.NewStore(env.Pool)

	if _, err := seed.Demo(ctx, env.Auth, defs, env.OrgID); err != nil {
		t.Fatalf("first Demo: %v", err)
	}
	res, err := seed.Demo(ctx, env.Auth, defs, env.OrgID)
	if err != nil {
		t.Fatalf("second Demo: %v", err)
	}
	for _, it := range res.Apply.Items {
		if it.Changed || it.Created {
			t.Errorf("second seed changed %s/%s: %+v", it.Kind, it.Slug, it)
		}
	}
	if len(res.UsersCreated) != 0 {
		t.Errorf("second seed created users %v", res.UsersCreated)
	}
	versions, err := defs.FormVersions(ctx, env.OrgID, "job-application")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Errorf("job-application versions = %d, want 1", len(versions))
	}
	users, err := env.Auth.ListUsers(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Errorf("users = %d, want 3", len(users))
	}
}

func TestDemoKeepsExistingUser(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	if _, err := env.Auth.CreateUser(ctx, env.OrgID, "reviewer@demo.local", "Existing", "my-own-password", []string{"reviewer"}); err != nil {
		t.Fatal(err)
	}
	res, err := seed.Demo(ctx, env.Auth, definitions.NewStore(env.Pool), env.OrgID)
	if err != nil {
		t.Fatalf("Demo: %v", err)
	}
	for _, e := range res.UsersCreated {
		if e == "reviewer@demo.local" {
			t.Errorf("existing user was re-created")
		}
	}
	if _, _, err := env.Auth.Login(ctx, "reviewer@demo.local", "my-own-password"); err != nil {
		t.Errorf("existing password no longer works: %v", err)
	}
}

func TestEnabledFromEnv(t *testing.T) {
	cases := map[string]bool{"": false, "false": false, "0": false, "true": true, "TRUE": true, "1": true}
	for v, want := range cases {
		t.Setenv("OPENFORMS_SEED_DEMO", v)
		if got := seed.EnabledFromEnv(); got != want {
			t.Errorf("OPENFORMS_SEED_DEMO=%q → %v, want %v", v, got, want)
		}
	}
}
```

- [ ] **Step 2.2: Run to verify it fails**

Run: `docker compose up -d postgres && go test ./internal/seed/...`
Expected: FAIL — build error `no non-test Go files` / `undefined: seed.Demo`.

- [ ] **Step 2.3: Implement the seed package**

Create `internal/seed/seed.go`:

```go
// Package seed loads the embedded demo bundle and demo users.
package seed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/openforms/openforms/examples"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definitions"
)

// DemoPassword is the password of every demo user.
const DemoPassword = "demo1234"

// DemoUser describes one account created by Demo.
type DemoUser struct {
	Email string
	Name  string
	Roles []string
}

// DemoUsers are created by Demo when absent.
var DemoUsers = []DemoUser{
	{Email: "admin@demo.local", Name: "Avery Admin", Roles: []string{auth.RoleAdmin}},
	{Email: "reviewer@demo.local", Name: "Riley Reviewer", Roles: []string{"reviewer"}},
	{Email: "manager@demo.local", Name: "Morgan Manager", Roles: []string{"hiring-manager"}},
}

// Result reports what Demo changed.
type Result struct {
	Apply        definitions.ApplyResult
	UsersCreated []string
}

// Demo applies the example bundle (source "seed") and creates missing demo users.
// It is idempotent: unchanged definitions create no versions and existing users are left untouched.
func Demo(ctx context.Context, authSvc *auth.Service, defs *definitions.Store, orgID uuid.UUID) (Result, error) {
	var res Result
	forms, workflows, err := examples.Bundle()
	if err != nil {
		return res, fmt.Errorf("load example bundle: %w", err)
	}
	res.Apply, err = defs.Apply(ctx, orgID, definitions.ApplyInput{
		Forms:     forms,
		Workflows: workflows,
		Source:    definitions.SourceSeed,
		Actor:     "seed",
	})
	if err != nil {
		return res, fmt.Errorf("apply example bundle: %w", err)
	}
	for _, du := range DemoUsers {
		_, err := authSvc.GetUserByEmail(ctx, orgID, du.Email)
		if err == nil {
			continue
		}
		if !errors.Is(err, auth.ErrNotFound) {
			return res, fmt.Errorf("look up %s: %w", du.Email, err)
		}
		if _, err := authSvc.CreateUser(ctx, orgID, du.Email, du.Name, DemoPassword, du.Roles); err != nil {
			if errors.Is(err, auth.ErrEmailTaken) {
				continue // created concurrently by another instance
			}
			return res, fmt.Errorf("create %s: %w", du.Email, err)
		}
		res.UsersCreated = append(res.UsersCreated, du.Email)
	}
	return res, nil
}

// EnabledFromEnv reports whether OPENFORMS_SEED_DEMO asks for seeding on serve.
func EnabledFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OPENFORMS_SEED_DEMO")))
	return v == "true" || v == "1"
}
```

- [ ] **Step 2.4: Run the seed tests to verify they pass**

Run: `go test ./internal/seed/... -v`
Expected: PASS for all four tests.

- [ ] **Step 2.5: Write the failing CLI tests**

Create `internal/cli/seed_test.go`:

```go
package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/db/dbtest"
)

func TestRunSeedDemoIsIdempotent(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	var first bytes.Buffer
	if err := runSeedDemo(ctx, &first, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	out := first.String()
	for _, want := range []string{"job-application", "hiring", "contact-triage", "created", "reviewer@demo.local", "demo1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("first output missing %q:\n%s", want, out)
		}
	}

	var second bytes.Buffer
	if err := runSeedDemo(ctx, &second, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}
	out = second.String()
	if strings.Contains(out, "created") || strings.Contains(out, "updated") {
		t.Errorf("second run should report only unchanged items:\n%s", out)
	}
	if !strings.Contains(out, "demo users already exist") {
		t.Errorf("second output missing users line:\n%s", out)
	}
}

func TestSeedCommandRequiresDemoFlag(t *testing.T) {
	cmd := newSeedCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--demo") {
		t.Fatalf("err = %v, want mention of --demo", err)
	}
}
```

- [ ] **Step 2.6: Run to verify it fails**

Run: `go test ./internal/cli/ -run 'Seed' -v`
Expected: FAIL — `undefined: runSeedDemo` / `undefined: newSeedCmd`.

- [ ] **Step 2.7: Implement the command**

Create `internal/cli/seed.go`:

```go
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/seed"
)

func newSeedCmd() *cobra.Command {
	var demo bool
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load the demo forms, workflows and users (idempotent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !demo {
				return errors.New("nothing to seed: pass --demo")
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.DatabaseURL == "" {
				return errors.New("OPENFORMS_DATABASE_URL is required")
			}
			pool, err := db.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := db.Migrate(ctx, pool); err != nil {
				return err
			}
			return runSeedDemo(ctx, cmd.OutOrStdout(), pool)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false, "seed the demo bundle and demo users")
	return cmd
}

func runSeedDemo(ctx context.Context, out io.Writer, pool *pgxpool.Pool) error {
	authSvc := auth.NewService(pool)
	orgID, err := authSvc.EnsureDefaultOrg(ctx)
	if err != nil {
		return err
	}
	res, err := seed.Demo(ctx, authSvc, definitions.NewStore(pool), orgID)
	if err != nil {
		return err
	}
	for _, it := range res.Apply.Items {
		status := "unchanged"
		switch {
		case it.Created:
			status = "created"
		case it.Changed:
			status = "updated"
		}
		fmt.Fprintf(out, "%-9s %-20s v%-4d %s\n", it.Kind, it.Slug, it.Version, status)
	}
	if len(res.UsersCreated) == 0 {
		fmt.Fprintln(out, "demo users already exist")
	}
	for _, email := range res.UsersCreated {
		fmt.Fprintf(out, "user      %-20s password %s\n", email, seed.DemoPassword)
	}
	return nil
}
```

In `internal/cli/root.go`, add this line next to the other `AddCommand` calls (the root command variable is named in Plan 01's `root.go`; use that name):

```go
	root.AddCommand(newSeedCmd())
```

- [ ] **Step 2.8: Run the CLI tests to verify they pass**

Run: `go test ./internal/cli/ -run 'Seed' -v`
Expected: PASS for `TestRunSeedDemoIsIdempotent` and `TestSeedCommandRequiresDemoFlag`.

- [ ] **Step 2.9: Auto-seed in `app.New`**

In `internal/app/app.go`, inside `New`, after `a.Auth`, `a.Defs` and `a.OrgID` are set and **before** the router (`a.Handler`) is built, insert:

```go
	if seed.EnabledFromEnv() {
		res, err := seed.Demo(ctx, a.Auth, a.Defs, a.OrgID)
		if err != nil {
			a.Close()
			return nil, fmt.Errorf("seed demo: %w", err)
		}
		slog.Info("demo bundle seeded", "items", len(res.Apply.Items), "usersCreated", len(res.UsersCreated))
	}
```

Add the imports `"log/slog"` (if missing), `"fmt"` (if missing) and `"github.com/openforms/openforms/internal/seed"`. If Plan 01 named the receiver variable something other than `a`, use that name.

- [ ] **Step 2.10: Verify the whole Go suite and a manual boot**

Run: `go build ./... && go test ./...`
Expected: PASS.

Run (Git Bash, two terminals):
```bash
OPENFORMS_DATABASE_URL='postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable' OPENFORMS_SEED_DEMO=true go run ./cmd/openforms serve
curl -s http://localhost:8080/api/v1/public/forms/job-application | head -c 120
```
Expected: the server logs `demo bundle seeded`, and curl prints `{"form":{"slug":"job-application",...`. Stop the server with Ctrl+C, then run `go run ./cmd/openforms seed --demo` with the same env var. Expected: every line ends `unchanged`, followed by `demo users already exist`.

- [ ] **Step 2.11: Commit**

```bash
git add internal/seed internal/cli/seed.go internal/cli/seed_test.go internal/cli/root.go internal/app/app.go
git commit -m "feat(seed): add openforms seed --demo and OPENFORMS_SEED_DEMO auto-seed"
```

---

### Task 3: `GET /api/v1/public/config`, SDK `getPublicConfig()`, demo credentials on login

**Parallel group:** W4

**Files:**
- Create: `internal/httpapi/public_config.go`
- Test: `internal/httpapi/public_config_test.go`
- Modify: `internal/httpapi/routes.go` (one line)
- Modify: `web/packages/sdk/src/api-types.ts` (add `PublicConfig`)
- Modify: `web/packages/sdk/src/client.ts` (add `getPublicConfig`)
- Test: `web/packages/sdk/src/public-config.test.ts`
- Create: `web/apps/admin/src/components/DemoCredentials.tsx`
- Create: `web/apps/admin/src/components/DemoCredentials.css`
- Test: `web/apps/admin/src/components/DemoCredentials.test.tsx`
- Modify: `web/apps/admin/src/pages/LoginPage.tsx`
- Test: `web/apps/admin/src/pages/LoginPage.demo.test.tsx`

**Interfaces:**
- Consumes: `httpapi.Deps`, `WriteJSON`, `NewRouter` (Plan 01); `testutil.NewEnv`, `testutil.Do`, `testutil.Decode` (Plan 01); `definitions.NewStore`, `submissions.NewService` (Plan 03); `OpenFormsClient` (Plan 06); admin `client` from `src/api.ts`, `renderWithProviders` from `src/test/render.tsx`, MSW `server` from `src/test/server.ts` (Plan 07).
- Produces:
  - `GET /api/v1/public/config` → `200 {"demo": <Config.DemoMode>}` (no auth), registered by `func mountPublicConfig(r chi.Router, d Deps)`.
  - TS `export interface PublicConfig { demo: boolean }` and `OpenFormsClient.getPublicConfig(): Promise<PublicConfig>`.
  - `DemoCredentials({ onPick }: { onPick: (email: string, password: string) => void })` plus exported constants `DEMO_ACCOUNTS`, `DEMO_PASSWORD`.

- [ ] **Step 3.1: Write the failing Go test**

Create `internal/httpapi/public_config_test.go`:

```go
package httpapi_test

import (
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

func TestPublicConfig(t *testing.T) {
	for _, demo := range []bool{true, false} {
		env := testutil.NewEnv(t)
		cfg := env.Config
		cfg.DemoMode = demo
		defs := definitions.NewStore(env.Pool)
		h := httpapi.NewRouter(httpapi.Deps{
			Config: cfg,
			Auth:   env.Auth,
			Defs:   defs,
			Subs:   submissions.NewService(env.Pool, defs),
			OrgID:  env.OrgID,
		})

		rec := testutil.Do(t, h, http.MethodGet, "/api/v1/public/config", "", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("demo=%v: status %d body %s", demo, rec.Code, rec.Body.String())
		}
		got := testutil.Decode[map[string]bool](t, rec)
		if got["demo"] != demo {
			t.Errorf("demo=%v: body %v", demo, got)
		}
	}
}
```

- [ ] **Step 3.2: Run to verify it fails**

Run: `go test ./internal/httpapi/ -run TestPublicConfig -v`
Expected: FAIL with `status 404` (route not registered).

- [ ] **Step 3.3: Implement the handler and register it**

Create `internal/httpapi/public_config.go`:

```go
package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

type publicConfigResponse struct {
	Demo bool `json:"demo"`
}

// mountPublicConfig exposes non-sensitive server settings to anonymous clients
// (the admin login page and the /demo page use it to detect demo mode).
func mountPublicConfig(r chi.Router, d Deps) {
	r.Get("/public/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		WriteJSON(w, http.StatusOK, publicConfigResponse{Demo: d.Config.DemoMode})
	})
}
```

In `internal/httpapi/routes.go`, add `mountPublicConfig(r, d)` on the line immediately after the existing `mountPublic(r, d)` call, which is in the `/api/v1` router outside the `RequireAuth` group. If Plan 03's `mountPublic` registers its routes through `r.Route("/public", …)` and chi reports a routing conflict (the test gets a 404 or panics), register the handler inside that sub-router as `r.Get("/config", …)` instead: move the body of `mountPublicConfig` into a helper `publicConfigHandler(d Deps) http.HandlerFunc` and call it from both places.

- [ ] **Step 3.4: Run to verify it passes**

Run: `go test ./internal/httpapi/ -v`
Expected: PASS (including every existing httpapi test).

- [ ] **Step 3.5: Write the failing SDK test**

Create `web/packages/sdk/src/public-config.test.ts`:

```ts
import { describe, expect, it, vi } from "vitest";
import { OpenFormsClient } from "./client";

describe("getPublicConfig", () => {
  it("GETs /api/v1/public/config and returns the body", async () => {
    const fetchMock = vi.fn<typeof fetch>(
      async () =>
        new Response(JSON.stringify({ demo: true }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
    );
    const client = new OpenFormsClient({ baseUrl: "http://api.test", fetch: fetchMock });

    await expect(client.getPublicConfig()).resolves.toEqual({ demo: true });

    expect(fetchMock).toHaveBeenCalledTimes(1);
    const [url, init] = fetchMock.mock.calls[0];
    expect(String(url)).toBe("http://api.test/api/v1/public/config");
    expect(init?.method ?? "GET").toBe("GET");
  });
});
```

- [ ] **Step 3.6: Run to verify it fails**

Run: `pnpm -C web/packages/sdk test -- public-config`
Expected: FAIL with `client.getPublicConfig is not a function` (or a TS error saying the property does not exist).

- [ ] **Step 3.7: Implement in the SDK**

Append to `web/packages/sdk/src/api-types.ts`:

```ts
/** GET /api/v1/public/config */
export interface PublicConfig {
  demo: boolean;
}
```

In `web/packages/sdk/src/client.ts`, import `PublicConfig` together with the other api-types imports. Then add this method to `OpenFormsClient` directly after `getPublicForm`, using the same private request helper that `getPublicForm` uses (shown here as `this.request`; if Plan 06 named it differently, use that name, since the method body follows the same pattern as `getPublicForm`):

```ts
  /** GET /api/v1/public/config (unauthenticated). */
  getPublicConfig(): Promise<PublicConfig> {
    return this.request<PublicConfig>("GET", "/api/v1/public/config");
  }
```

If `src/index.ts` re-exports types explicitly, not with `export *`, add `PublicConfig` to that export list.

- [ ] **Step 3.8: Run to verify it passes**

Run: `pnpm -C web/packages/sdk test && pnpm -C web/packages/sdk typecheck && pnpm -C web/packages/sdk build`
Expected: all tests PASS, no type errors, and the build succeeds. The build step matters because the admin app consumes the built package.

- [ ] **Step 3.9: Write the failing admin tests**

Create `web/apps/admin/src/components/DemoCredentials.test.tsx`:

```tsx
import { describe, expect, it, vi } from "vitest";
import { http, HttpResponse } from "msw";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "../test/server";
import { renderWithProviders } from "../test/render";
import { DemoCredentials, DEMO_PASSWORD } from "./DemoCredentials";

describe("DemoCredentials", () => {
  it("lists the demo accounts and picks one", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })));
    const onPick = vi.fn();
    renderWithProviders(<DemoCredentials onPick={onPick} />);

    expect(await screen.findByRole("heading", { name: "Demo accounts" })).toBeInTheDocument();
    expect(screen.getByText("admin@demo.local")).toBeInTheDocument();
    expect(screen.getByText("reviewer@demo.local")).toBeInTheDocument();
    expect(screen.getByText("manager@demo.local")).toBeInTheDocument();
    expect(screen.getByText(DEMO_PASSWORD)).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Use Reviewer" }));
    expect(onPick).toHaveBeenCalledWith("reviewer@demo.local", "demo1234");
  });

  it("renders nothing when demo is false", async () => {
    let requested = false;
    server.use(
      http.get("*/api/v1/public/config", () => {
        requested = true;
        return HttpResponse.json({ demo: false });
      }),
    );
    const { container } = renderWithProviders(<DemoCredentials onPick={() => {}} />);
    await waitFor(() => expect(requested).toBe(true));
    expect(container).toBeEmptyDOMElement();
  });

  it("renders nothing when the config request fails", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.error()));
    const { container } = renderWithProviders(<DemoCredentials onPick={() => {}} />);
    await new Promise((r) => setTimeout(r, 50));
    expect(container).toBeEmptyDOMElement();
  });
});
```

Create `web/apps/admin/src/pages/LoginPage.demo.test.tsx`:

```tsx
import { describe, expect, it } from "vitest";
import { http, HttpResponse } from "msw";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { server } from "../test/server";
import { renderWithProviders } from "../test/render";
import { LoginPage } from "./LoginPage";

describe("LoginPage in demo mode", () => {
  it("fills the form from a demo account", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })));
    renderWithProviders(<LoginPage />, { route: "/login", path: "/login" });

    await userEvent.click(await screen.findByRole("button", { name: "Use Hiring manager" }));

    expect(screen.getByLabelText(/email/i)).toHaveValue("manager@demo.local");
    expect(screen.getByLabelText(/password/i)).toHaveValue("demo1234");
  });

  it("does not show demo accounts outside demo mode", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: false })));
    renderWithProviders(<LoginPage />, { route: "/login", path: "/login" });
    await screen.findByLabelText(/email/i);
    await new Promise((r) => setTimeout(r, 50));
    expect(screen.queryByRole("heading", { name: "Demo accounts" })).not.toBeInTheDocument();
  });
});
```

If Plan 07 exports `LoginPage` as a default export, change the import to `import LoginPage from "./LoginPage";`.

- [ ] **Step 3.10: Run to verify they fail**

Run: `pnpm -C web/apps/admin test -- DemoCredentials LoginPage.demo`
Expected: FAIL. `DemoCredentials` cannot be resolved (`Failed to resolve import "./DemoCredentials"`), and the LoginPage test cannot find the "Use Hiring manager" button.

- [ ] **Step 3.11: Implement the component and wire it into LoginPage**

Create `web/apps/admin/src/components/DemoCredentials.tsx`:

```tsx
import { useQuery } from "@tanstack/react-query";
import { client } from "../api";
import "./DemoCredentials.css";

export const DEMO_PASSWORD = "demo1234";

export const DEMO_ACCOUNTS: ReadonlyArray<{ email: string; role: string }> = [
  { email: "admin@demo.local", role: "Admin" },
  { email: "reviewer@demo.local", role: "Reviewer" },
  { email: "manager@demo.local", role: "Hiring manager" },
];

export function DemoCredentials({ onPick }: { onPick: (email: string, password: string) => void }) {
  const { data } = useQuery({
    queryKey: ["public-config"],
    queryFn: () => client.getPublicConfig(),
    staleTime: Infinity,
    retry: false,
  });
  if (!data?.demo) return null;

  return (
    <section className="demo-credentials" aria-labelledby="demo-credentials-title">
      <h2 id="demo-credentials-title">Demo accounts</h2>
      <p>
        Every account uses the password <code>{DEMO_PASSWORD}</code>.
      </p>
      <ul>
        {DEMO_ACCOUNTS.map((account) => (
          <li key={account.email}>
            <code>{account.email}</code>
            <button type="button" onClick={() => onPick(account.email, DEMO_PASSWORD)}>
              Use {account.role}
            </button>
          </li>
        ))}
      </ul>
    </section>
  );
}
```

Create `web/apps/admin/src/components/DemoCredentials.css`:

```css
.demo-credentials {
  margin-top: 1.5rem;
  padding: 1rem 1.25rem;
  border: 1px dashed var(--border, #d0d5dd);
  border-radius: var(--radius, 8px);
  background: var(--surface-muted, rgba(127, 127, 127, 0.06));
  font-size: 0.875rem;
}
.demo-credentials h2 {
  margin: 0 0 0.25rem;
  font-size: 0.9375rem;
}
.demo-credentials p {
  margin: 0 0 0.75rem;
  color: var(--text-muted, inherit);
}
.demo-credentials ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 0.5rem;
}
.demo-credentials li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
}
.demo-credentials button {
  font: inherit;
  padding: 0.25rem 0.625rem;
  border-radius: 6px;
  border: 1px solid var(--border, #d0d5dd);
  background: var(--surface, transparent);
  color: inherit;
  cursor: pointer;
}
.demo-credentials button:hover {
  border-color: var(--accent, #4f46e5);
}
```

In `web/apps/admin/src/pages/LoginPage.tsx`:
1. Add `import { DemoCredentials } from "../components/DemoCredentials";`.
2. Directly after the closing `</form>` tag, render:

```tsx
      <DemoCredentials
        onPick={(email, password) => {
          setEmail(email);
          setPassword(password);
        }}
      />
```

`setEmail`/`setPassword` are the state setters behind the email and password inputs. If Plan 07 used different names, such as a single `setValues` object, write `onPick` so that both controlled inputs receive the picked values. If the inputs are uncontrolled, convert them to controlled inputs with `useState` in this step.

- [ ] **Step 3.12: Run to verify they pass**

Run: `pnpm -C web/apps/admin test && pnpm -C web/apps/admin typecheck`
Expected: all admin tests PASS (both new and existing) with no type errors.

- [ ] **Step 3.13: Commit**

```bash
git add internal/httpapi/public_config.go internal/httpapi/public_config_test.go internal/httpapi/routes.go \
  web/packages/sdk/src web/apps/admin/src/components/DemoCredentials.* web/apps/admin/src/pages/LoginPage.tsx \
  web/apps/admin/src/pages/LoginPage.demo.test.tsx
git commit -m "feat: add public config endpoint and demo credentials on admin login"
```

---

### Task 4: Dockerfile, compose `app` profile, Makefile, CI

**Parallel group:** W4

**Files:**
- Create: `Dockerfile`
- Create: `.dockerignore`
- Modify (replace): `docker-compose.yml`
- Modify: `Makefile` (add/replace targets `web`, `web-test`, `build`, `up`, `down`, `e2e`)
- Create: `.github/workflows/ci.yml`

**Interfaces:**
- Consumes: web build contract (§3): `pnpm -C web build` writes `internal/webui/dist/{admin,hosted,demo}/index.html` and `dist/embed/embed.js`; `openforms serve`; env vars from spec §6.1; `OPENFORMS_SEED_DEMO` (Task 2).
- Produces: image `openforms` (entrypoint `/openforms`, default cmd `serve`, port 8080); compose service `openforms` in profile `app`, reachable at `http://localhost:8080`, with `host.docker.internal` resolvable from the container (needed by the Task 7 webhook sink); Makefile targets used by the roadmap's definition of done; CI jobs `go`, `web`, `e2e`.

This task is configuration, so the "test" is a set of executable checks. Step 4.1 records the failing state first.

- [ ] **Step 4.1: Record the failing state**

Run: `docker build -t openforms:dev . ; echo "exit=$?"`
Expected: `exit=1` with `failed to read dockerfile` (no Dockerfile yet).

- [ ] **Step 4.2: Write `.dockerignore` and `Dockerfile`**

Create `.dockerignore`:

```
.git
.github
**/node_modules
web/**/dist
internal/webui/dist
e2e
bin
docs
*.log
.env
```

Create `Dockerfile`:

```dockerfile
# syntax=docker/dockerfile:1.7

# ---- Stage 1: build the web apps --------------------------------------------
FROM node:22-bookworm-slim AS web
WORKDIR /src
RUN corepack enable && corepack prepare pnpm@9 --activate
# The SDK's generated types are committed; schemas/ is only needed by tests.
# The demo app imports YAML from examples/ via Vite ?raw imports.
COPY schemas/ schemas/
COPY examples/ examples/
COPY web/ web/
RUN mkdir -p internal/webui/dist && touch internal/webui/dist/.gitkeep
RUN cd web && pnpm install --frozen-lockfile && pnpm build
# Refuse to produce an image without the UIs (the server would answer 503).
RUN test -f internal/webui/dist/admin/index.html \
 && test -f internal/webui/dist/hosted/index.html \
 && test -f internal/webui/dist/embed/embed.js

# ---- Stage 2: build the Go binary with the web apps embedded -----------------
FROM golang:1.24-bookworm AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/webui/dist ./internal/webui/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/openforms ./cmd/openforms

# ---- Stage 3: minimal runtime -------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=go /out/openforms /openforms
ENV OPENFORMS_HTTP_ADDR=:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/openforms"]
CMD ["serve"]
```

- [ ] **Step 4.3: Build the image and verify the missing-UI guard**

Run: `docker build -t openforms:dev . && docker run --rm openforms:dev --help | head -n 5`
Expected: build succeeds. The help output lists `serve`, `migrate`, `seed` and the other commands.

Check the guard: temporarily rename `web/apps/hosted/index.html` to `index.html.bak`, run `docker build -t openforms:guard .`, and restore the file.
Expected: the build FAILS at the `test -f internal/webui/dist/hosted/index.html` step (non-zero exit). Restore the file before continuing.

- [ ] **Step 4.4: Replace `docker-compose.yml`**

```yaml
# Development services: `docker compose up -d postgres mailpit`
# Full stack (demo mode):  `docker compose --profile app up -d --build`
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: openforms
      POSTGRES_PASSWORD: openforms
      POSTGRES_DB: openforms
    ports:
      - "54329:5432"
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openforms -d openforms"]
      interval: 2s
      timeout: 5s
      retries: 30

  mailpit:
    image: axllent/mailpit:v1.21
    ports:
      - "1025:1025"
      - "8025:8025"

  openforms:
    profiles: ["app"]
    build: .
    image: openforms:dev
    ports:
      - "8080:8080"
    environment:
      OPENFORMS_DATABASE_URL: postgres://openforms:openforms@postgres:5432/openforms?sslmode=disable
      OPENFORMS_BASE_URL: http://localhost:8080
      OPENFORMS_SMTP_HOST: mailpit
      OPENFORMS_SMTP_PORT: "1025"
      OPENFORMS_SMTP_FROM: openforms@demo.local
      OPENFORMS_WEBHOOK_SECRET: e2e-secret
      OPENFORMS_DEMO: "true"
      OPENFORMS_SEED_DEMO: "true"
    extra_hosts:
      - "host.docker.internal:host-gateway"
    depends_on:
      postgres:
        condition: service_healthy
      mailpit:
        condition: service_started

volumes:
  pgdata: {}
```

- [ ] **Step 4.5: Verify compose**

Run: `docker compose --profile app config -q && echo ok`
Expected: `ok`.

Run: `docker compose --profile app up -d --build && sleep 5 && curl -fsS http://localhost:8080/healthz`
Expected: `{"status":"ok"}`. Also check `curl -fsS http://localhost:8080/api/v1/public/config`. Expected: `{"demo":true}` once Task 3 is merged into this lane, or a 404 before that. The seeded-form check runs in the Wave 4 merge check at the top of this plan.

- [ ] **Step 4.6: Makefile targets**

Add these targets to `Makefile`. Replace any target with the same name from Plan 01 (keep `dev-db`, `test` and `lint` as Plan 01 wrote them). Recipe lines must start with a **tab**.

```make
.PHONY: web web-test build up down e2e

web:
	cd web && pnpm install --frozen-lockfile && pnpm build

web-test:
	cd web && pnpm install --frozen-lockfile && pnpm typecheck && pnpm test

build: web
	go build -o bin/openforms ./cmd/openforms

up:
	docker compose --profile app up -d --build

down:
	docker compose --profile app down -v

e2e: up
	cd e2e && pnpm install --frozen-lockfile && pnpm exec playwright install chromium && pnpm test
```

Run: `make -n web web-test build up e2e`
Expected: the commands print without `missing separator` errors, which would mean spaces were used instead of tabs.

- [ ] **Step 4.7: CI workflow**

Create `.github/workflows/ci.yml`:

```yaml
name: ci

on:
  push:
    branches: [main]
  pull_request:

concurrency:
  group: ci-${{ github.ref }}
  cancel-in-progress: true

jobs:
  go:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:16
        env:
          POSTGRES_USER: openforms
          POSTGRES_PASSWORD: openforms
          POSTGRES_DB: openforms
        ports:
          - 54329:5432
        options: >-
          --health-cmd "pg_isready -U openforms -d openforms"
          --health-interval 5s --health-timeout 5s --health-retries 20
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod
      - run: go vet ./...
      - run: go test -race ./...

  web:
    runs-on: ubuntu-latest
    defaults:
      run:
        working-directory: web
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: pnpm
          cache-dependency-path: web/pnpm-lock.yaml
      - run: pnpm install --frozen-lockfile
      - run: pnpm typecheck
      - run: pnpm test
      - run: pnpm build

  e2e:
    needs: [go, web]
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: pnpm/action-setup@v4
        with:
          version: 9
      - uses: actions/setup-node@v4
        with:
          node-version: 22
          cache: pnpm
          cache-dependency-path: e2e/pnpm-lock.yaml
      - name: Start stack
        run: docker compose --profile app up -d --build
      - name: Install Playwright
        working-directory: e2e
        run: pnpm install --frozen-lockfile && pnpm exec playwright install --with-deps chromium
      - name: Run E2E
        working-directory: e2e
        run: pnpm test
      - name: Stack logs
        if: failure()
        run: docker compose --profile app logs --no-color
      - uses: actions/upload-artifact@v4
        if: always()
        with:
          name: playwright-report
          path: e2e/playwright-report
          retention-days: 7
```

If `web/package.json` declares a `packageManager` field, remove the `with: version: 9` lines from both `pnpm/action-setup` steps, because the action refuses conflicting versions.

- [ ] **Step 4.8: Lint the workflow**

Run: `docker run --rm -v "$PWD:/repo" -w /repo rhysd/actionlint:latest -color`
Expected: no output and exit 0. Git Bash on Windows may rewrite `/repo`; if the volume path is mangled, prefix the command with `MSYS_NO_PATHCONV=1`.

- [ ] **Step 4.9: Commit**

```bash
docker compose --profile app down
git add Dockerfile .dockerignore docker-compose.yml Makefile .github/workflows/ci.yml
git commit -m "chore: add Docker image, compose app profile, Makefile targets and CI"
```

---

### Task 5: `/demo` page (`web/apps/demo`)

**Parallel group:** W5

**Files:**
- Create: `web/apps/demo/package.json`
- Create: `web/apps/demo/index.html`
- Create: `web/apps/demo/vite.config.ts`
- Create: `web/apps/demo/tsconfig.json`
- Create: `web/apps/demo/src/main.tsx`
- Create: `web/apps/demo/src/App.tsx`
- Create: `web/apps/demo/src/client.ts`
- Create: `web/apps/demo/src/sources.ts`
- Create: `web/apps/demo/src/highlight.ts`
- Create: `web/apps/demo/src/useDemoStatus.ts`
- Create: `web/apps/demo/src/components/CodeBlock.tsx`
- Create: `web/apps/demo/src/components/Notice.tsx`
- Create: `web/apps/demo/src/sections/Hero.tsx`
- Create: `web/apps/demo/src/sections/DefineSection.tsx`
- Create: `web/apps/demo/src/sections/CollectSection.tsx`
- Create: `web/apps/demo/src/sections/TrackSection.tsx`
- Create: `web/apps/demo/src/sections/ReviewerPanel.tsx`
- Create: `web/apps/demo/src/sections/EmbedSection.tsx`
- Create: `web/apps/demo/src/sections/CliSection.tsx`
- Create: `web/apps/demo/src/styles.css`
- Create: `web/apps/demo/src/test/setup.ts`
- Create: `web/apps/demo/src/test/server.ts`
- Test: `web/apps/demo/src/App.test.tsx`
- Test: `web/apps/demo/src/sections/ReviewerPanel.test.tsx`
- Test: `web/apps/demo/src/components/CodeBlock.test.tsx`

**Interfaces:**
- Consumes (Plan 06): `OpenFormsClient` with `getPublicConfig()` (Task 3), `getPublicForm(slug)`, `submitPublic(...)` (used internally by `OpenForm`), `getPublicStatus`, `login(email, password)`, `getSubmission(id): Promise<SubmissionDetail>`, `transition(id, { transition, fields?, comment?, expectedState? })`; `OpenFormsError { status; code; message; details }`; types `SubmissionDetail`, `AvailableTransition` from `@openforms/sdk`; `<OpenForm client slug onSubmitted />`, `<StatusTracker client submissionId token pollMs />` and `@openforms/react/styles.css` from `@openforms/react`. Example YAML from Task 1. `/embed.js` served by the Go server (Plans 01/06).
- Produces: the built app in `internal/webui/dist/demo/` (served at `/demo`); exported `PERSONAS`, `DEMO_PASSWORD` from `ReviewerPanel.tsx`; test-only `resetDemoApi()` and `demoApi` from `src/test/server.ts`.

- [ ] **Step 5.1: Scaffold the package (no app code yet)**

Create `web/apps/demo/package.json`:

```json
{
  "name": "@openforms/demo-app",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "scripts": {
    "dev": "vite",
    "build": "tsc --noEmit && vite build",
    "test": "vitest run",
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "@openforms/react": "workspace:*",
    "@openforms/sdk": "workspace:*",
    "react": "^18.3.1",
    "react-dom": "^18.3.1",
    "shiki": "^3.2.0"
  },
  "devDependencies": {
    "@testing-library/jest-dom": "^6.6.0",
    "@testing-library/react": "^16.1.0",
    "@testing-library/user-event": "^14.5.2",
    "@types/react": "^18.3.12",
    "@types/node": "^22.10.0",
    "@types/react-dom": "^18.3.1",
    "@vitejs/plugin-react": "^4.3.4",
    "jsdom": "^25.0.1",
    "msw": "^2.7.0",
    "typescript": "^5.7.2",
    "vite": "^6.0.0",
    "vitest": "^3.0.0"
  }
}
```

Create `web/apps/demo/vite.config.ts`:

```ts
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import { fileURLToPath } from "node:url";

// Repo root: the demo imports ../../../examples/openforms/*.yaml via ?raw.
const repoRoot = fileURLToPath(new URL("../../..", import.meta.url));
const api = "http://localhost:8080";

export default defineConfig({
  base: "/_app/demo/",
  plugins: [react()],
  server: {
    port: 5175,
    fs: { allow: [repoRoot] },
    proxy: {
      "/api": api,
      "/healthz": api,
      "/embed.js": api,
      "/f/": api,
      "/s/": api,
      "/admin": api,
      "/_app/hosted": api,
      "/_app/admin": api,
    },
  },
  test: {
    environment: "jsdom",
    environmentOptions: { jsdom: { url: "http://localhost:3000/demo" } },
    setupFiles: ["./src/test/setup.ts"],
    css: false,
  },
});
```

Create `web/apps/demo/tsconfig.json`:

```json
{
  "extends": "../../tsconfig.base.json",
  "compilerOptions": {
    "jsx": "react-jsx",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "types": ["node", "vite/client", "@testing-library/jest-dom"],
    "noEmit": true
  },
  "include": ["src", "vite.config.ts"]
}
```

Create `web/apps/demo/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta name="description" content="Try openforms: forms as code with built-in submission workflows." />
    <meta name="color-scheme" content="light dark" />
    <title>openforms · live demo</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

Run: `cd web && pnpm install`
Expected: the lockfile updates and `@openforms/demo-app` is linked into the workspace (Plan 06's `pnpm-workspace.yaml` covers `apps/*`).

- [ ] **Step 5.2: Write the test infrastructure (MSW mock API)**

Create `web/apps/demo/src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
import { afterAll, afterEach, beforeAll, beforeEach } from "vitest";
import { cleanup } from "@testing-library/react";
import { server, resetDemoApi } from "./server";

class ResizeObserverStub {
  observe() {}
  unobserve() {}
  disconnect() {}
}
(globalThis as unknown as { ResizeObserver: unknown }).ResizeObserver ??= ResizeObserverStub;

beforeAll(() => server.listen({ onUnhandledRequest: "error" }));
beforeEach(() => resetDemoApi());
afterEach(() => {
  cleanup();
  server.resetHandlers();
});
afterAll(() => server.close());
```

Create `web/apps/demo/src/test/server.ts`:

```ts
import { http, HttpResponse } from "msw";
import { setupServer } from "msw/node";

type StateKey = "new" | "screening" | "interview" | "hired" | "rejected";

const STATES = [
  { key: "new", label: "New", color: "gray" },
  { key: "screening", label: "Screening", color: "blue" },
  { key: "interview", label: "Interview", color: "purple" },
  { key: "hired", label: "Hired", color: "green", terminal: true },
  { key: "rejected", label: "Rejected", color: "red", terminal: true },
] as const;

const WORKFLOW = {
  slug: "hiring",
  title: "Hiring pipeline",
  initial: "new",
  states: STATES,
  fields: [
    { key: "score", type: "number", label: "Score" },
    { key: "rejectionReason", type: "textarea", label: "Rejection reason" },
  ],
  transitions: [
    { key: "screen", label: "Start screening", from: ["new"], to: "screening", guard: { roles: ["reviewer"] } },
    { key: "invite", label: "Invite to interview", from: ["screening"], to: "interview", guard: { roles: ["reviewer"], requireFields: ["score"] } },
    { key: "hire", label: "Hire", from: ["interview"], to: "hired", guard: { roles: ["hiring-manager"] } },
    { key: "reject", label: "Reject", from: ["new", "screening", "interview"], to: "rejected", guard: { roles: ["reviewer", "hiring-manager"], requireFields: ["rejectionReason"] } },
  ],
};

export const FORM = {
  slug: "job-application",
  title: "Job application",
  settings: { public: true, submitLabel: "Send application", confirmationMessage: "Thanks for applying!" },
  fields: [{ key: "name", type: "text", label: "Full name", required: true }],
};

export const demoApi = {
  state: "new" as StateKey,
  fields: {} as Record<string, unknown>,
  persona: "" as string,
  loginCalls: [] as string[],
  transitionBodies: [] as Array<Record<string, unknown>>,
};

export function resetDemoApi() {
  demoApi.state = "new";
  demoApi.fields = {};
  demoApi.persona = "";
  demoApi.loginCalls = [];
  demoApi.transitionBodies = [];
}

const label = (k: string) => STATES.find((s) => s.key === k)!.label;
const terminal = (k: string) => Boolean(STATES.find((s) => s.key === k && "terminal" in s));

function available() {
  const roles = demoApi.persona === "manager@demo.local" ? ["hiring-manager"] : ["reviewer"];
  return WORKFLOW.transitions
    .filter((t) => t.from.includes(demoApi.state))
    .map((t) => {
      const allowed = t.guard.roles.some((r) => roles.includes(r));
      return {
        key: t.key,
        label: t.label,
        to: t.to,
        toLabel: label(t.to),
        requireFields: t.guard.requireFields ?? [],
        allowed,
        reason: allowed ? "" : "role",
      };
    });
}

function submission() {
  return {
    id: "sub-1",
    form: "job-application",
    formVersion: 1,
    state: demoApi.state,
    stateLabel: label(demoApi.state),
    terminal: terminal(demoApi.state),
    data: { name: "Ada" },
    fields: demoApi.fields,
    assignee: null,
    createdAt: "2026-09-23T10:00:00Z",
    updatedAt: "2026-09-23T10:00:00Z",
  };
}

const err = (status: number, code: string, message: string) =>
  HttpResponse.json({ error: { code, message } }, { status });

export const handlers = [
  http.get("*/api/v1/public/config", () => HttpResponse.json({ demo: true })),
  http.get("*/api/v1/public/forms/job-application", () => HttpResponse.json({ form: FORM })),
  http.post("*/api/v1/public/forms/job-application/submissions", () =>
    HttpResponse.json(
      { id: "sub-1", state: "new", stateLabel: "New", receiptToken: "tok-1", confirmationMessage: "Thanks for applying!" },
      { status: 201 },
    ),
  ),
  http.get("*/api/v1/public/submissions/sub-1", () =>
    HttpResponse.json({
      id: "sub-1",
      formTitle: "Job application",
      state: demoApi.state,
      stateLabel: label(demoApi.state),
      terminal: terminal(demoApi.state),
      states: STATES,
      history: [{ state: "new", label: "New", at: "2026-09-23T10:00:00Z" }],
      createdAt: "2026-09-23T10:00:00Z",
    }),
  ),
  http.post("*/api/v1/auth/login", async ({ request }) => {
    const body = (await request.json()) as { email: string; password: string };
    demoApi.loginCalls.push(body.email);
    if (body.password !== "demo1234") return err(401, "invalid_credentials", "Invalid email or password");
    demoApi.persona = body.email;
    return HttpResponse.json({
      user: { id: "u-1", email: body.email, name: body.email, roles: [], createdAt: "2026-09-23T10:00:00Z" },
    });
  }),
  http.get("*/api/v1/submissions/sub-1", () =>
    HttpResponse.json({ submission: submission(), form: FORM, workflow: WORKFLOW, events: [], transitions: available() }),
  ),
  http.post("*/api/v1/submissions/sub-1/transitions", async ({ request }) => {
    const body = (await request.json()) as { transition: string; fields?: Record<string, unknown> };
    demoApi.transitionBodies.push(body);
    const t = available().find((x) => x.key === body.transition);
    if (!t) return err(409, "invalid_state", "Transition not available from this state");
    if (!t.allowed) return err(403, "forbidden", "You are not allowed to perform this transition");
    demoApi.fields = { ...demoApi.fields, ...(body.fields ?? {}) };
    demoApi.state = t.to as StateKey;
    return HttpResponse.json({ submission: submission() });
  }),
  http.get("*/embed.js", () => new HttpResponse("/* embed stub */", { headers: { "Content-Type": "text/javascript" } })),
];

export const server = setupServer(...handlers);
```

- [ ] **Step 5.3: Write the failing tests**

Create `web/apps/demo/src/components/CodeBlock.test.tsx`:

```tsx
import { describe, expect, it, vi } from "vitest";
import { render, screen } from "@testing-library/react";

vi.mock("../highlight", () => ({
  highlight: vi.fn(async (code: string) => `<pre class="shiki"><code>HL:${code}</code></pre>`),
}));

import { CodeBlock } from "./CodeBlock";

describe("CodeBlock", () => {
  it("shows plain code immediately, then the highlighted version", async () => {
    render(<CodeBlock code="slug: demo" lang="yaml" label="forms/demo.yaml" />);
    expect(screen.getByText("slug: demo")).toBeInTheDocument();
    expect(screen.getByText("forms/demo.yaml")).toBeInTheDocument();
    expect(await screen.findByText("HL:slug: demo")).toBeInTheDocument();
  });
});
```

Create `web/apps/demo/src/App.test.tsx`:

```tsx
import { describe, expect, it, vi } from "vitest";
import { http, HttpResponse } from "msw";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { render } from "@testing-library/react";
import { server } from "./test/server";

vi.mock("./highlight", () => ({ highlight: async (code: string) => `<pre class="shiki"><code>${code}</code></pre>` }));

import { App } from "./App";

describe("demo App", () => {
  it("renders every section", async () => {
    render(<App />);
    for (const name of ["Define", "Collect", "Track & review", "Embed", "Automate from the CLI"]) {
      expect(screen.getByRole("heading", { level: 2, name: new RegExp(name) })).toBeInTheDocument();
    }
    expect(screen.getByRole("tab", { name: "forms/job-application.yaml" })).toHaveAttribute("aria-selected", "true");
    expect(await screen.findByLabelText(/Full name/)).toBeInTheDocument();
  });

  it("shows the seed hint when the demo form is missing", async () => {
    server.use(
      http.get("*/api/v1/public/forms/job-application", () =>
        HttpResponse.json({ error: { code: "not_found", message: "not found" } }, { status: 404 }),
      ),
    );
    render(<App />);
    const collect = screen.getByRole("region", { name: /Collect/ });
    expect(await within(collect).findByText(/to enable the live demo/)).toBeInTheDocument();
    expect(within(collect).getByText("openforms seed --demo")).toBeInTheDocument();
  });

  it("shows an offline notice when the server is unreachable", async () => {
    server.use(http.get("*/api/v1/public/config", () => HttpResponse.error()));
    render(<App />);
    expect(await screen.findByText("Server unreachable")).toBeInTheDocument();
  });

  it("submits the application and starts tracking it", async () => {
    render(<App />);
    await userEvent.type(await screen.findByLabelText(/Full name/), "Ada Lovelace");
    await userEvent.click(screen.getByRole("button", { name: "Send application" }));

    const track = screen.getByRole("region", { name: /Track & review/ });
    expect(await within(track).findByRole("link", { name: "Open in the admin inbox" })).toHaveAttribute(
      "href",
      "/admin/submissions/sub-1",
    );
    expect((await within(track).findAllByText("New")).length).toBeGreaterThan(0);
    expect(await within(track).findByRole("button", { name: "Start screening" })).toBeEnabled();
  });
});
```

Create `web/apps/demo/src/sections/ReviewerPanel.test.tsx`:

```tsx
import { describe, expect, it, vi } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { demoApi } from "../test/server";
import { ReviewerPanel } from "./ReviewerPanel";

describe("ReviewerPanel", () => {
  it("does not log in outside demo mode", async () => {
    render(<ReviewerPanel submissionId="sub-1" demoMode={false} onTransitioned={() => {}} />);
    expect(screen.getByText(/only available when the server runs in demo mode/)).toBeInTheDocument();
    await new Promise((r) => setTimeout(r, 50));
    expect(demoApi.loginCalls).toEqual([]);
  });

  it("performs transitions as the reviewer, collecting required fields", async () => {
    const onTransitioned = vi.fn();
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={onTransitioned} />);

    await userEvent.click(await screen.findByRole("button", { name: "Start screening" }));
    await waitFor(() => expect(onTransitioned).toHaveBeenCalledTimes(1));
    expect(demoApi.loginCalls[0]).toBe("reviewer@demo.local");

    const invite = await screen.findByRole("button", { name: "Invite to interview" });
    expect(invite).toBeDisabled();
    await userEvent.type(screen.getByLabelText("Score"), "4");
    expect(invite).toBeEnabled();
    await userEvent.click(invite);

    await waitFor(() => expect(onTransitioned).toHaveBeenCalledTimes(2));
    expect(demoApi.transitionBodies[1]).toMatchObject({ transition: "invite", fields: { score: 4 }, expectedState: "screening" });
  });

  it("role-guarded transition is disabled until switching persona", async () => {
    demoApi.state = "interview";
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={() => {}} />);

    const hire = await screen.findByRole("button", { name: "Hire" });
    expect(hire).toBeDisabled();
    expect(screen.getByText("Not allowed for Reviewer")).toBeInTheDocument();

    await userEvent.click(screen.getByRole("button", { name: "Hiring manager" }));
    await waitFor(() => expect(demoApi.loginCalls).toContain("manager@demo.local"));
    await waitFor(() => expect(screen.getByRole("button", { name: "Hire" })).toBeEnabled());
  });

  it("shows a final-state message for terminal submissions", async () => {
    demoApi.state = "hired";
    render(<ReviewerPanel submissionId="sub-1" demoMode onTransitioned={() => {}} />);
    expect(await screen.findByText(/reached a final state: Hired/)).toBeInTheDocument();
  });
});
```

- [ ] **Step 5.4: Run to verify they fail**

Run: `pnpm -C web/apps/demo test`
Expected: FAIL. Imports such as `./CodeBlock`, `./App` and `./ReviewerPanel` cannot be resolved (`Failed to resolve import`).

- [ ] **Step 5.5: Implement the plumbing**

Create `web/apps/demo/src/client.ts`:

```ts
import { OpenFormsClient } from "@openforms/sdk";

/** Same-origin client: the demo is served by the openforms server at /demo. */
export const client = new OpenFormsClient({ baseUrl: window.location.origin });
```

Create `web/apps/demo/src/sources.ts`:

```ts
import jobApplicationYaml from "../../../../examples/openforms/forms/job-application.yaml?raw";
import hiringYaml from "../../../../examples/openforms/workflows/hiring.yaml?raw";

export interface SourceFile {
  file: string;
  code: string;
}

/** The exact files `openforms seed --demo` loads into the server. */
export const SOURCES: SourceFile[] = [
  { file: "forms/job-application.yaml", code: jobApplicationYaml },
  { file: "workflows/hiring.yaml", code: hiringYaml },
];
```

Create `web/apps/demo/src/highlight.ts`:

```ts
import { codeToHtml } from "shiki";

export type Lang = "yaml" | "bash" | "html";

/** Dual-theme highlighting; colours come from --shiki-light / --shiki-dark CSS variables. */
export function highlight(code: string, lang: Lang): Promise<string> {
  return codeToHtml(code, {
    lang,
    themes: { light: "github-light", dark: "github-dark" },
    defaultColor: false,
  });
}
```

Create `web/apps/demo/src/useDemoStatus.ts`:

```ts
import { useEffect, useState } from "react";
import { OpenFormsError } from "@openforms/sdk";
import { client } from "./client";

export type DemoStatus =
  | { kind: "loading" }
  | { kind: "offline" }
  | { kind: "not-seeded"; demoMode: boolean }
  | { kind: "ready"; demoMode: boolean };

export const DEMO_FORM_SLUG = "job-application";

export function useDemoStatus(): DemoStatus {
  const [status, setStatus] = useState<DemoStatus>({ kind: "loading" });

  useEffect(() => {
    let cancelled = false;
    const set = (s: DemoStatus) => {
      if (!cancelled) setStatus(s);
    };
    (async () => {
      let demoMode: boolean;
      try {
        demoMode = (await client.getPublicConfig()).demo;
      } catch {
        set({ kind: "offline" });
        return;
      }
      try {
        await client.getPublicForm(DEMO_FORM_SLUG);
        set({ kind: "ready", demoMode });
      } catch (e) {
        set(e instanceof OpenFormsError && e.status === 404 ? { kind: "not-seeded", demoMode } : { kind: "offline" });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  return status;
}
```

Create `web/apps/demo/src/components/Notice.tsx`:

```tsx
import type { ReactNode } from "react";

export function Notice({ title, tone = "info", children }: { title: string; tone?: "info" | "warn"; children: ReactNode }) {
  return (
    <div className={`notice notice-${tone}`} role="status">
      <strong>{title}</strong>
      <div>{children}</div>
    </div>
  );
}
```

Create `web/apps/demo/src/components/CodeBlock.tsx`:

```tsx
import { useEffect, useState } from "react";
import { highlight, type Lang } from "../highlight";

export function CodeBlock({ code, lang, label }: { code: string; lang: Lang; label?: string }) {
  const [html, setHtml] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    setHtml(null);
    highlight(code, lang)
      .then((h) => {
        if (live) setHtml(h);
      })
      .catch(() => {
        /* keep the plain fallback */
      });
    return () => {
      live = false;
    };
  }, [code, lang]);

  return (
    <figure className="code">
      {label && <figcaption>{label}</figcaption>}
      {html ? (
        <div className="code-body" dangerouslySetInnerHTML={{ __html: html }} />
      ) : (
        <pre className="code-body">
          <code>{code}</code>
        </pre>
      )}
    </figure>
  );
}
```

- [ ] **Step 5.6: Implement the sections**

Create `web/apps/demo/src/sections/Hero.tsx`:

```tsx
export function Hero() {
  return (
    <header className="hero">
      <div className="wrap">
        <p className="eyebrow">openforms · live demo</p>
        <h1>
          Forms as code.
          <br />
          Workflows built in.
        </h1>
        <p className="lede">
          openforms is an open-source, self-hosted form platform for developers. Define forms and their review
          workflows in YAML, push them with a CLI, and move every submission through guarded states, with
          webhooks, emails and a full audit trail.
        </p>
        <div className="hero-actions">
          <a className="button primary" href="#collect">
            Try it below
          </a>
          <a className="button" href="https://github.com/openforms/openforms">
            GitHub
          </a>
          <a className="button" href="https://github.com/openforms/openforms/tree/main/docs">
            Docs
          </a>
        </div>
      </div>
    </header>
  );
}
```

Create `web/apps/demo/src/sections/DefineSection.tsx`:

```tsx
import { useState } from "react";
import { SOURCES } from "../sources";
import { CodeBlock } from "../components/CodeBlock";

export function DefineSection() {
  const [active, setActive] = useState(0);
  return (
    <section className="step" id="define" aria-labelledby="define-title">
      <div className="step-intro">
        <span className="step-number">1</span>
        <h2 id="define-title">Define</h2>
        <p>
          A form and its workflow are two small YAML files in your repo. These are the files running behind
          this page. The workflow gives each submission its states, who may move it and what happens when it
          moves.
        </p>
      </div>
      <div className="card">
        <div role="tablist" aria-label="Definition files" className="tabs">
          {SOURCES.map((s, i) => (
            <button
              key={s.file}
              role="tab"
              id={`tab-${i}`}
              aria-selected={i === active}
              aria-controls={`panel-${i}`}
              tabIndex={i === active ? 0 : -1}
              className="tab"
              onClick={() => setActive(i)}
            >
              {s.file}
            </button>
          ))}
        </div>
        <div role="tabpanel" id={`panel-${active}`} aria-labelledby={`tab-${active}`}>
          <CodeBlock code={SOURCES[active].code} lang="yaml" />
        </div>
      </div>
    </section>
  );
}
```

Create `web/apps/demo/src/sections/CollectSection.tsx`:

```tsx
import { OpenForm } from "@openforms/react";
import { client } from "../client";
import { Notice } from "../components/Notice";
import { DEMO_FORM_SLUG, type DemoStatus } from "../useDemoStatus";

export interface Submitted {
  id: string;
  receiptToken: string;
}

export function CollectSection({ status, onSubmitted }: { status: DemoStatus; onSubmitted: (s: Submitted) => void }) {
  return (
    <section className="step" id="collect" aria-labelledby="collect-title">
      <div className="step-intro">
        <span className="step-number">2</span>
        <h2 id="collect-title">Collect</h2>
        <p>
          This form is rendered headlessly with <code>@openforms/react</code>, straight from the definition
          above. Conditional fields, validation and the submit label all come from the YAML. Pick “Designer” to
          see the portfolio field appear.
        </p>
      </div>
      <div className="card form-card">
        {status.kind === "loading" && <p className="muted">Connecting to the openforms server…</p>}
        {status.kind === "offline" && (
          <Notice title="Server unreachable" tone="warn">
            The demo could not reach the openforms API. Start the server with{" "}
            <code>docker compose --profile app up</code> and reload this page.
          </Notice>
        )}
        {status.kind === "not-seeded" && (
          <Notice title="Live demo not enabled">
            Run <code>openforms seed --demo</code> to enable the live demo.
          </Notice>
        )}
        {status.kind === "ready" && (
          <OpenForm
            client={client}
            slug={DEMO_FORM_SLUG}
            onSubmitted={(result) => onSubmitted({ id: result.id, receiptToken: result.receiptToken })}
          />
        )}
      </div>
    </section>
  );
}
```

Create `web/apps/demo/src/sections/ReviewerPanel.tsx`:

```tsx
import { useCallback, useEffect, useState } from "react";
import { OpenFormsError, type AvailableTransition, type SubmissionDetail } from "@openforms/sdk";
import { client } from "../client";

export const DEMO_PASSWORD = "demo1234";

export const PERSONAS = [
  { email: "reviewer@demo.local", label: "Reviewer" },
  { email: "manager@demo.local", label: "Hiring manager" },
] as const;

type Persona = (typeof PERSONAS)[number];

function messageOf(e: unknown): string {
  return e instanceof OpenFormsError ? e.message : "Something went wrong. Please try again.";
}

function isEmpty(v: unknown): boolean {
  return v === undefined || v === null || v === "";
}

export function ReviewerPanel({
  submissionId,
  demoMode,
  onTransitioned,
}: {
  submissionId: string;
  demoMode: boolean;
  onTransitioned: () => void;
}) {
  const [persona, setPersona] = useState<Persona>(PERSONAS[0]);
  const [detail, setDetail] = useState<SubmissionDetail | null>(null);
  const [values, setValues] = useState<Record<string, string>>({});
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      await client.login(persona.email, DEMO_PASSWORD);
      setDetail(await client.getSubmission(submissionId));
    } catch (e) {
      setError(messageOf(e));
    }
  }, [persona, submissionId]);

  useEffect(() => {
    if (demoMode) void load();
  }, [demoMode, load]);

  if (!demoMode) {
    return (
      <aside className="panel" aria-labelledby="reviewer-title">
        <h3 id="reviewer-title">Play the reviewer</h3>
        <p className="muted">
          Reviewer simulation is only available when the server runs in demo mode (<code>OPENFORMS_DEMO=true</code>).
          Sign in to the admin inbox to review this submission.
        </p>
      </aside>
    );
  }

  const wfField = (key: string) => detail?.workflow?.fields?.find((f) => f.key === key);

  const missing = (t: AvailableTransition) =>
    t.requireFields.filter((k) => isEmpty(detail?.submission.fields[k]) && isEmpty(values[k]));

  const coerce = (key: string, raw: string): unknown => {
    const type = wfField(key)?.type;
    if (type === "number") return raw === "" ? null : Number(raw);
    if (type === "checkbox") return raw === "true";
    return raw;
  };

  async function run(t: AvailableTransition) {
    if (!detail) return;
    setBusy(t.key);
    setError(null);
    try {
      const fields: Record<string, unknown> = {};
      for (const k of t.requireFields) {
        if (!isEmpty(values[k])) fields[k] = coerce(k, values[k]);
      }
      await client.transition(submissionId, {
        transition: t.key,
        fields,
        expectedState: detail.submission.state,
      });
      setValues({});
      await load();
      onTransitioned();
    } catch (e) {
      setError(messageOf(e));
      await load();
    } finally {
      setBusy(null);
    }
  }

  return (
    <aside className="panel" aria-labelledby="reviewer-title">
      <h3 id="reviewer-title">Play the reviewer</h3>
      <div className="segmented" role="group" aria-label="Act as">
        {PERSONAS.map((p) => (
          <button
            key={p.email}
            type="button"
            aria-pressed={p.email === persona.email}
            onClick={() => setPersona(p)}
          >
            {p.label}
          </button>
        ))}
      </div>
      <p className="muted small">
        Signed in as <code>{persona.email}</code>. Guards in the workflow decide what this role may do.
      </p>

      {error && (
        <p className="error" role="alert">
          {error}
        </p>
      )}
      {!detail && !error && <p className="muted">Loading submission…</p>}

      {detail && detail.submission.terminal && (
        <p className="final">This submission reached a final state: {detail.submission.stateLabel}.</p>
      )}

      {detail && !detail.submission.terminal && detail.transitions.length === 0 && (
        <p className="muted">No transitions are available from {detail.submission.stateLabel}.</p>
      )}

      {detail && !detail.submission.terminal && (
        <ul className="transitions">
          {detail.transitions.map((t) => {
            const need = missing(t);
            return (
              <li key={t.key} className="transition">
                {t.allowed &&
                  t.requireFields
                    .filter((k) => isEmpty(detail.submission.fields[k]))
                    .map((k) => {
                      const f = wfField(k);
                      const id = `field-${t.key}-${k}`;
                      return (
                        <label key={k} htmlFor={id} className="field">
                          <span>{f?.label ?? k}</span>
                          {f?.type === "textarea" ? (
                            <textarea
                              id={id}
                              rows={2}
                              value={values[k] ?? ""}
                              onChange={(e) => setValues((v) => ({ ...v, [k]: e.target.value }))}
                            />
                          ) : (
                            <input
                              id={id}
                              type={f?.type === "number" ? "number" : "text"}
                              value={values[k] ?? ""}
                              onChange={(e) => setValues((v) => ({ ...v, [k]: e.target.value }))}
                            />
                          )}
                        </label>
                      );
                    })}
                <button
                  type="button"
                  className="button primary"
                  disabled={!t.allowed || need.length > 0 || busy !== null}
                  onClick={() => void run(t)}
                >
                  {t.label}
                </button>
                <span className="muted small">
                  {t.allowed ? `→ ${t.toLabel}` : `Not allowed for ${persona.label}`}
                </span>
              </li>
            );
          })}
        </ul>
      )}
    </aside>
  );
}
```

Create `web/apps/demo/src/sections/TrackSection.tsx`:

```tsx
import { useState } from "react";
import { StatusTracker } from "@openforms/react";
import { client } from "../client";
import type { Submitted } from "./CollectSection";
import { ReviewerPanel } from "./ReviewerPanel";

export function TrackSection({ submitted, demoMode }: { submitted: Submitted | null; demoMode: boolean }) {
  const [refreshKey, setRefreshKey] = useState(0);
  return (
    <section className="step" id="track" aria-labelledby="track-title">
      <div className="step-intro">
        <span className="step-number">3</span>
        <h2 id="track-title">Track &amp; review</h2>
        <p>
          Every submission gets a private status link. Act as a reviewer or hiring manager on the right and
          watch the respondent’s tracker update. Each move is guarded, audited, and can send email or fire
          webhooks.
        </p>
      </div>
      {!submitted ? (
        <div className="card empty">
          <p className="muted">Submit the application above to watch it move through the hiring workflow.</p>
        </div>
      ) : (
        <div className="track-grid">
          <div className="card">
            <h3>What the applicant sees</h3>
            <StatusTracker
              key={refreshKey}
              client={client}
              submissionId={submitted.id}
              token={submitted.receiptToken}
              pollMs={3000}
            />
          </div>
          <div className="card">
            <ReviewerPanel
              submissionId={submitted.id}
              demoMode={demoMode}
              onTransitioned={() => setRefreshKey((k) => k + 1)}
            />
            <a className="inbox-link" href={`/admin/submissions/${submitted.id}`}>
              Open in the admin inbox
            </a>
          </div>
        </div>
      )}
    </section>
  );
}
```

Create `web/apps/demo/src/sections/EmbedSection.tsx`:

```tsx
import { useEffect, useRef, useState } from "react";
import { CodeBlock } from "../components/CodeBlock";

export function EmbedSection({ enabled }: { enabled: boolean }) {
  const host = useRef<HTMLDivElement>(null);
  const injected = useRef(false);
  const [copied, setCopied] = useState(false);
  const snippet = `<div data-openforms="contact"></div>\n<script src="${window.location.origin}/embed.js" async></script>`;

  useEffect(() => {
    if (!enabled || injected.current || !host.current) return;
    injected.current = true;
    const script = document.createElement("script");
    script.src = "/embed.js";
    script.async = true;
    host.current.appendChild(script);
  }, [enabled]);

  async function copy() {
    try {
      await navigator.clipboard.writeText(snippet);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      setCopied(false);
    }
  }

  return (
    <section className="step" id="embed" aria-labelledby="embed-title">
      <div className="step-intro">
        <span className="step-number">4</span>
        <h2 id="embed-title">Embed</h2>
        <p>
          Not rendering forms yourself? Drop two lines into any page. The hosted form loads in an auto-resizing
          iframe and emits an <code>openforms:submitted</code> event.
        </p>
      </div>
      <div className="embed-grid">
        <div className="card">
          <CodeBlock code={snippet} lang="html" label="index.html" />
          <button type="button" className="button" onClick={() => void copy()}>
            {copied ? "Copied" : "Copy snippet"}
          </button>
        </div>
        <div className="card" ref={host}>
          {enabled ? (
            <div data-openforms="contact" />
          ) : (
            <p className="muted">The live embed appears once the demo bundle is seeded.</p>
          )}
        </div>
      </div>
    </section>
  );
}
```

Create `web/apps/demo/src/sections/CliSection.tsx`:

```tsx
import { CodeBlock } from "../components/CodeBlock";

const CLI = `# scaffold openforms.yaml, a form and a workflow
openforms init

# check every definition offline (great in CI)
openforms validate

# push to your server: unchanged files create no new versions
export OPENFORMS_API_KEY=ofk_...
openforms push

# fail CI when someone edited a form in the UI without pulling
openforms pull --check`;

export function CliSection() {
  return (
    <section className="step" id="cli" aria-labelledby="cli-title">
      <div className="step-intro">
        <span className="step-number">5</span>
        <h2 id="cli-title">Automate from the CLI</h2>
        <p>
          The same binary is the server and the CLI. Keep definitions in git, review them in pull requests, and
          deploy them like code.
        </p>
      </div>
      <div className="card">
        <CodeBlock code={CLI} lang="bash" label="terminal" />
      </div>
    </section>
  );
}
```

Create `web/apps/demo/src/App.tsx`:

```tsx
import { useState } from "react";
import { useDemoStatus } from "./useDemoStatus";
import { Hero } from "./sections/Hero";
import { DefineSection } from "./sections/DefineSection";
import { CollectSection, type Submitted } from "./sections/CollectSection";
import { TrackSection } from "./sections/TrackSection";
import { EmbedSection } from "./sections/EmbedSection";
import { CliSection } from "./sections/CliSection";

export function App() {
  const status = useDemoStatus();
  const [submitted, setSubmitted] = useState<Submitted | null>(null);
  const ready = status.kind === "ready";

  return (
    <>
      <Hero />
      <main className="wrap">
        <DefineSection />
        <CollectSection status={status} onSubmitted={setSubmitted} />
        <TrackSection submitted={submitted} demoMode={ready && status.demoMode} />
        <EmbedSection enabled={ready} />
        <CliSection />
      </main>
      <footer className="footer wrap">
        <p>
          openforms is open source.{" "}
          <a href="https://github.com/openforms/openforms">Star it on GitHub</a> ·{" "}
          <a href="/admin">Admin</a>
        </p>
      </footer>
    </>
  );
}
```

Create `web/apps/demo/src/main.tsx`:

```tsx
import { createRoot } from "react-dom/client";
import "@openforms/react/styles.css";
import "./styles.css";
import { App } from "./App";

createRoot(document.getElementById("root")!).render(<App />);
```

> `main.tsx` does not wrap the app in `StrictMode`: the embed section injects a global script exactly once. Idempotency is guarded by a ref, but StrictMode's double effects would still re-run the reviewer login.

- [ ] **Step 5.7: Styles**

Create `web/apps/demo/src/styles.css`:

```css
:root {
  --bg: #fbfbfd;
  --surface: #ffffff;
  --surface-2: #f3f4f8;
  --text: #0f172a;
  --muted: #5b6475;
  --border: #e3e6ee;
  --accent: #4f46e5;
  --accent-contrast: #ffffff;
  --accent-soft: #eef0ff;
  --warn: #b45309;
  --warn-soft: #fff7e6;
  --danger: #b91c1c;
  --radius: 14px;
  --shadow: 0 1px 2px rgba(15, 23, 42, 0.04), 0 8px 24px rgba(15, 23, 42, 0.06);
  --font: ui-sans-serif, system-ui, -apple-system, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif;
  --mono: ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, monospace;
  --of-accent: var(--accent);
  --of-radius: 10px;
  --of-font: var(--font);
  color-scheme: light dark;
}

@media (prefers-color-scheme: dark) {
  :root {
    --bg: #0b0d12;
    --surface: #12151c;
    --surface-2: #181c25;
    --text: #e7e9ee;
    --muted: #9aa3b5;
    --border: #252a36;
    --accent: #8b87ff;
    --accent-contrast: #0b0d12;
    --accent-soft: #1d1c3a;
    --warn: #fbbf24;
    --warn-soft: #2a2110;
    --danger: #f87171;
    --shadow: 0 1px 2px rgba(0, 0, 0, 0.4), 0 8px 24px rgba(0, 0, 0, 0.35);
  }
}

* { box-sizing: border-box; }
html { scroll-behavior: smooth; }
body {
  margin: 0;
  background: var(--bg);
  color: var(--text);
  font: 16px/1.6 var(--font);
  -webkit-font-smoothing: antialiased;
}
a { color: var(--accent); }
code { font-family: var(--mono); font-size: 0.9em; background: var(--surface-2); padding: 0.1em 0.35em; border-radius: 5px; }
.wrap { width: min(1120px, 100% - 32px); margin-inline: auto; }
.muted { color: var(--muted); }
.small { font-size: 0.85rem; }

/* Hero */
.hero {
  padding: 96px 0 72px;
  background:
    radial-gradient(900px 400px at 15% -10%, var(--accent-soft), transparent 70%),
    radial-gradient(700px 360px at 95% 0%, color-mix(in srgb, var(--accent) 12%, transparent), transparent 70%);
  border-bottom: 1px solid var(--border);
}
.eyebrow { margin: 0 0 12px; font-size: 0.8rem; letter-spacing: 0.08em; text-transform: uppercase; color: var(--accent); font-weight: 600; }
.hero h1 { margin: 0; font-size: clamp(2.4rem, 6vw, 4rem); line-height: 1.05; letter-spacing: -0.03em; }
.lede { max-width: 640px; margin: 20px 0 28px; font-size: 1.125rem; color: var(--muted); }
.hero-actions { display: flex; flex-wrap: wrap; gap: 12px; }

/* Buttons */
.button {
  display: inline-flex; align-items: center; justify-content: center; gap: 8px;
  padding: 10px 16px; border-radius: 10px; border: 1px solid var(--border);
  background: var(--surface); color: var(--text); font: 600 0.95rem/1 var(--font);
  text-decoration: none; cursor: pointer; transition: border-color 120ms, transform 120ms;
}
.button:hover { border-color: var(--accent); }
.button:active { transform: translateY(1px); }
.button.primary { background: var(--accent); border-color: var(--accent); color: var(--accent-contrast); }
.button:disabled { opacity: 0.45; cursor: not-allowed; }

/* Steps */
main.wrap { padding: 32px 0 64px; }
.step { display: grid; grid-template-columns: minmax(0, 320px) minmax(0, 1fr); gap: 40px; padding: 56px 0; border-bottom: 1px solid var(--border); }
.step:last-child { border-bottom: 0; }
.step-intro h2 { margin: 8px 0 12px; font-size: 1.75rem; letter-spacing: -0.02em; }
.step-intro p { margin: 0; color: var(--muted); }
.step-number {
  display: inline-grid; place-items: center; width: 32px; height: 32px; border-radius: 999px;
  background: var(--accent-soft); color: var(--accent); font-weight: 700;
}
@media (max-width: 880px) { .step { grid-template-columns: 1fr; gap: 20px; } }

.card { background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); box-shadow: var(--shadow); padding: 20px; min-width: 0; }
.card h3 { margin: 0 0 12px; font-size: 1rem; }
.card.empty { display: grid; place-items: center; min-height: 160px; text-align: center; }
.form-card { padding: 28px; }

/* Tabs + code */
.tabs { display: flex; gap: 4px; margin-bottom: 12px; border-bottom: 1px solid var(--border); }
.tab { border: 0; background: none; padding: 8px 12px; font: 500 0.875rem var(--mono); color: var(--muted); cursor: pointer; border-bottom: 2px solid transparent; margin-bottom: -1px; }
.tab[aria-selected="true"] { color: var(--text); border-bottom-color: var(--accent); }
.code { margin: 0; }
.code figcaption { font: 500 0.8rem var(--mono); color: var(--muted); margin-bottom: 8px; }
.code-body, .code-body pre { margin: 0; overflow: auto; max-height: 520px; border-radius: 10px; font: 0.85rem/1.55 var(--mono); }
pre.code-body, .code-body pre { padding: 16px; background: var(--surface-2); }
.shiki, .shiki span { color: var(--shiki-light); background-color: var(--shiki-light-bg); }
@media (prefers-color-scheme: dark) { .shiki, .shiki span { color: var(--shiki-dark); background-color: var(--shiki-dark-bg); } }

/* Notices */
.notice { display: grid; gap: 4px; padding: 16px; border-radius: 10px; background: var(--accent-soft); border: 1px solid color-mix(in srgb, var(--accent) 30%, transparent); }
.notice-warn { background: var(--warn-soft); border-color: color-mix(in srgb, var(--warn) 35%, transparent); }
.notice strong { font-size: 0.95rem; }

/* Track & review */
.track-grid, .embed-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 20px; align-items: start; }
@media (max-width: 880px) { .track-grid, .embed-grid { grid-template-columns: 1fr; } }
.panel h3 { margin: 0 0 12px; }
.segmented { display: inline-flex; padding: 3px; border-radius: 10px; background: var(--surface-2); border: 1px solid var(--border); margin-bottom: 8px; }
.segmented button { border: 0; background: none; padding: 6px 12px; border-radius: 8px; font: 600 0.85rem var(--font); color: var(--muted); cursor: pointer; }
.segmented button[aria-pressed="true"] { background: var(--surface); color: var(--text); box-shadow: var(--shadow); }
.transitions { list-style: none; margin: 16px 0 0; padding: 0; display: grid; gap: 14px; }
.transition { display: grid; gap: 8px; padding-top: 14px; border-top: 1px dashed var(--border); }
.transition:first-child { border-top: 0; padding-top: 0; }
.field { display: grid; gap: 4px; font-size: 0.875rem; font-weight: 600; }
.field input, .field textarea { font: 400 0.95rem var(--font); padding: 8px 10px; border-radius: 8px; border: 1px solid var(--border); background: var(--surface); color: var(--text); }
.field input:focus, .field textarea:focus { outline: 2px solid var(--accent); outline-offset: 1px; }
.error { color: var(--danger); font-weight: 600; }
.final { font-weight: 600; }
.inbox-link { display: inline-block; margin-top: 16px; font-weight: 600; }

.footer { padding: 32px 0 48px; color: var(--muted); font-size: 0.9rem; border-top: 1px solid var(--border); }
.embed-grid .card > .button { margin-top: 12px; }
.embed-grid iframe { border: 0; width: 100%; }
```

- [ ] **Step 5.8: Run the tests to verify they pass**

Run: `pnpm -C web/apps/demo test`
Expected: PASS for `CodeBlock.test.tsx` (1), `App.test.tsx` (4) and `ReviewerPanel.test.tsx` (4).

If a test cannot find `Full name`, check how Plan 06's `OpenForm` labels required fields. It may append " *", which the `/Full name/` regex already tolerates. If `onSubmitted`'s parameter type lacks `receiptToken`, fix the type in `@openforms/react`, which must expose the §7.2 public submit response, not the demo.

- [ ] **Step 5.9: Typecheck, build, and check the built output**

Run: `pnpm -C web/apps/demo typecheck && pnpm -C web build && ls internal/webui/dist/demo`
Expected: no type errors. `internal/webui/dist/demo` contains `index.html` and `assets/`.

Run: `grep -c "/_app/demo/assets/" internal/webui/dist/demo/index.html`
Expected: a number ≥ 1, which confirms the base path.

- [ ] **Step 5.10: Manual smoke test against the real server**

```bash
docker compose up -d postgres mailpit
go build -o bin/openforms ./cmd/openforms
OPENFORMS_DATABASE_URL='postgres://openforms:openforms@localhost:54329/openforms?sslmode=disable' \
OPENFORMS_DEMO=true OPENFORMS_SEED_DEMO=true OPENFORMS_SMTP_HOST=localhost ./bin/openforms serve
```

Open `http://localhost:8080/demo` and check each item:
1. Both YAML tabs render highlighted.
2. Choosing "Designer" reveals Portfolio URL.
3. Submitting shows the tracker at "New".
4. "Start screening" moves the tracker to "Screening".
5. "Invite to interview" stays disabled until Score is filled.
6. As Reviewer, "Hire" is disabled with "Not allowed for Reviewer"; switching to Hiring manager enables it.
7. The contact embed renders and resizes.
8. Mailpit (`http://localhost:8025`) shows the "We got your application" email.

Toggle dark mode in the OS and confirm the page stays legible.

- [ ] **Step 5.11: Commit**

```bash
git add web/apps/demo web/pnpm-lock.yaml
git commit -m "feat(demo): add live /demo page with reviewer simulation and embed"
```

---

### Task 6: User documentation

**Parallel group:** W5

**Files:**
- Create or replace: `README.md`
- Create: `docs/getting-started.md`
- Create: `docs/definitions.md`
- Create: `docs/workflows.md`
- Create: `docs/api.md`
- Create: `docs/cli.md`
- Create: `docs/self-hosting.md`

**Interfaces:**
- Consumes: the spec (§5, §6.9 template vars and webhook format, §7, §8, §10), Task 1's example files, Task 4's compose/Makefile.
- Produces: user-facing documentation. The demo page's "Docs" link and the README point here.

Docs have no unit tests. Their "test" is a link and command check (Step 6.8) plus a read-through against the running stack (Step 6.9).

- [ ] **Step 6.1: README**

Create `README.md`:

````markdown
# openforms

**Forms as code. Workflows built in.**

openforms is an open-source, self-hosted form platform for developers, and an alternative to Typeform and Tally for teams that want their forms in git.

- **Forms as code.** Forms and workflows are YAML or JSON files. `openforms push` deploys them; `openforms pull --check` catches drift in CI.
- **Submission workflows.** Every form can have a state machine with custom states, role and required-field guards, and actions (webhook, email, auto-assign) that run reliably in the background with retries.
- **Headless first, hosted too.** Use the REST API, the TypeScript SDK or the React renderer, or share a hosted page (`/f/<slug>`) and embed it anywhere with two lines of HTML.
- **A real admin UI.** A reviewer inbox, submission timelines, and visual editors for forms and workflows.
- **One container.** A single Go binary with the web UI embedded, plus PostgreSQL.

## Try it in two minutes

```bash
git clone https://github.com/openforms/openforms && cd openforms
docker compose --profile app up -d --build
```

Then open:

| URL | What |
|---|---|
| http://localhost:8080/demo | Live demo: define → collect → track & review → embed |
| http://localhost:8080/admin | Admin inbox (demo accounts are listed on the login page, password `demo1234`) |
| http://localhost:8080/f/job-application | Hosted form |
| http://localhost:8025 | Mailpit, which catches every email the demo sends |

## A form and its workflow

```yaml
# openforms/forms/contact.yaml
slug: contact
title: Contact us
workflow: contact-triage
settings: { public: true }
fields:
  - { key: email, type: email, label: Email, required: true }
  - { key: message, type: textarea, label: Message, required: true }
```

```yaml
# openforms/workflows/contact-triage.yaml
slug: contact-triage
title: Contact triage
initial: open
states:
  - { key: open, label: Open }
  - { key: resolved, label: Resolved, terminal: true }
fields:
  - { key: reply, type: textarea, label: Reply }
transitions:
  - key: resolve
    label: Resolve
    from: [open]
    to: resolved
    guard: { roles: [support], requireFields: [reply] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Re: your message", body: "{{submission.fields.reply}}" }
```

```bash
openforms validate && openforms push
```

## Documentation

- [Getting started](docs/getting-started.md)
- [Form definitions](docs/definitions.md)
- [Workflows](docs/workflows.md)
- [HTTP API](docs/api.md)
- [CLI](docs/cli.md)
- [Self-hosting](docs/self-hosting.md)

## Architecture

```
            ┌───────────────────── openforms (one Go binary) ─────────────────────┐
 browser ──▶│ /f, /s (hosted)  /admin (inbox + editors)  /demo  /embed.js         │
 your app ─▶│ /api/v1  ── definitions store ── submissions ── workflow engine    │
 CLI ──────▶│                                               └─▶ job queue ──▶ webhook / email / assign
            └───────────────────────────────────┬──────────────────────────────────┘
                                                ▼
                                           PostgreSQL
```

Actions are written to the job queue in the same database transaction as the state change, so a failed transition never fires actions and a committed one always does (at least once).

## Repository layout

| Path | Contents |
|---|---|
| `cmd/openforms` | Binary entry point (server and CLI) |
| `internal/` | Go packages: config, db, auth, definition, definitions, submissions, workflow, jobs, actions, httpapi, webui, cli |
| `schemas/` | JSON Schemas and conformance fixtures shared by Go and TypeScript |
| `web/` | pnpm workspace: `@openforms/sdk`, `@openforms/react`, embed script, hosted/admin/demo apps |
| `examples/openforms` | The demo bundle loaded by `openforms seed --demo` |
| `e2e/` | Playwright end-to-end tests |

## Development

Requirements: Go ≥ 1.24, Node 22, pnpm 9, Docker.

```bash
make dev-db      # start Postgres (port 54329) and Mailpit
make test        # Go tests
make web-test    # TypeScript typecheck + tests
make build       # build web apps and bin/openforms
make e2e         # full stack + Playwright
```
````

- [ ] **Step 6.2: Getting started**

Create `docs/getting-started.md`:

````markdown
# Getting started

This guide takes you from nothing to a working form with a review workflow in about ten minutes.

## 1. Run openforms

You need Docker. From a clone of the repository:

```bash
docker compose --profile app up -d --build
curl http://localhost:8080/healthz   # {"status":"ok"}
```

The compose stack starts PostgreSQL, Mailpit (a local email catcher at http://localhost:8025) and openforms in demo mode. Demo mode seeds two example forms and three accounts. Their password is `demo1234`:

| Email | Role |
|---|---|
| `admin@demo.local` | admin |
| `reviewer@demo.local` | reviewer |
| `manager@demo.local` | hiring-manager |

Open http://localhost:8080/demo for a guided tour, or sign in at http://localhost:8080/admin.

> Running without Docker? Build with `make build`, start Postgres, then run
> `OPENFORMS_DATABASE_URL=postgres://… ./bin/openforms serve`. See [self-hosting](self-hosting.md).

## 2. Create an API key

The CLI talks to the server with an API key. Create one with the `admin` role:

```bash
docker compose exec openforms /openforms admin create-api-key --name my-laptop --roles admin
# ofk_2V9s…   ← shown once; copy it
export OPENFORMS_URL=http://localhost:8080
export OPENFORMS_API_KEY=ofk_2V9s…
```

You can also create keys in the admin UI under **API keys**.

## 3. Scaffold definitions

Install the CLI (`make build` produces `bin/openforms`, or use the Docker image) and, in your own project:

```bash
openforms init
```

This writes:

```
openforms.yaml                           # server URL and definitions directory
openforms/forms/contact.yaml             # a contact form
openforms/workflows/contact-triage.yaml  # its review workflow
```

## 4. Validate and push

```bash
openforms validate      # offline; prints file:path: message for every problem
openforms push --dry-run
openforms push
```

`push` prints one line per definition, with a status of `created`, `updated` or `unchanged`. Pushing the same files again changes nothing: definitions are versioned by content hash.

## 5. Collect a submission

Open http://localhost:8080/f/contact, fill it in and submit. The confirmation page links to a private status page (`/s/<id>?token=…`) that the respondent can bookmark.

Other ways to collect submissions:

- **Embed** the form on any site:
  ```html
  <div data-openforms="contact"></div>
  <script src="http://localhost:8080/embed.js" async></script>
  ```
- **Render it yourself** with React:
  ```tsx
  import { OpenFormsClient } from "@openforms/sdk";
  import { OpenForm } from "@openforms/react";
  import "@openforms/react/styles.css";

  const client = new OpenFormsClient({ baseUrl: "https://forms.example.com" });
  export const Contact = () => <OpenForm client={client} slug="contact" />;
  ```
- **Call the API** directly: `POST /api/v1/public/forms/contact/submissions` with `{"data": {...}}`.

## 6. Review it

Sign in at http://localhost:8080/admin. The inbox lists every submission. Open one to see its answers and timeline, and the transitions your role allows. A transition that needs fields (such as a reply) asks for them before it runs. Its actions, such as sending an email, run in the background. Check Mailpit to see the email arrive.

## 7. Keep definitions in git

Commit the `openforms/` directory. In CI, fail the build when someone edited a form in the UI without pulling:

```bash
openforms validate
openforms pull --check   # exits 1 if the server differs from your files
```

## Next steps

- [Form definitions](definitions.md): every field type, validation rule and condition.
- [Workflows](workflows.md): states, guards, actions, templates and webhooks.
- [HTTP API](api.md) and [CLI](cli.md) reference.
- [Self-hosting](self-hosting.md) for production.
````

- [ ] **Step 6.3: Definitions reference**

Create `docs/definitions.md`:

````markdown
# Form definitions

A form is a YAML (or JSON) document. The server stores every version and the CLI keeps them in files named `<dir>/forms/<slug>.yaml`, where the file name must equal the slug.

```yaml
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
```

## Top-level keys

| Key | Required | Description |
|---|---|---|
| `slug` | yes | `^[a-z0-9][a-z0-9-]{0,62}$`. Unique per server; used in URLs (`/f/<slug>`). |
| `title` | yes | Shown above the form and in the admin UI. |
| `description` | no | Shown under the title. |
| `workflow` | no | Slug of a [workflow](workflows.md). Without one, submissions stay in the terminal state `submitted`. |
| `settings.public` | yes | `true` lets anyone load and submit the form through the public API, hosted page and embed. `false` hides it (404) from anonymous users; authenticated API clients can still create submissions. |
| `settings.submitLabel` | no | Submit button text. Default `Submit`. |
| `settings.confirmationMessage` | no | Shown after a successful submission. |
| `fields` | yes | Ordered list of fields (see below). |

## Fields

| Key | Required | Description |
|---|---|---|
| `key` | yes | `^[a-zA-Z][a-zA-Z0-9_]{0,63}$`, unique within the form. The key in submission data. |
| `type` | yes | One of the types below. |
| `label` | yes | Visible label. |
| `help` | no | Hint text under the input. |
| `placeholder` | no | Placeholder text. |
| `required` | no | Must be provided when the field is visible. A required `checkbox` must be checked. |
| `options` | select/multiselect | `[{value, label}]`, at least one, values unique. Not allowed on other types. |
| `validation` | no | See below. |
| `showIf` | no | Condition that controls visibility (see below). |

### Field types and value types

| Type | Value in submission data | Notes |
|---|---|---|
| `text` | string | |
| `textarea` | string | Multi-line. |
| `email` | string | Must be a bare address (`ada@example.com`). |
| `url` | string | Absolute `http://` or `https://` URL with a host. |
| `number` | number | JSON number. |
| `date` | string | `YYYY-MM-DD`. |
| `select` | string | Must be one of the option values. |
| `multiselect` | array of strings | Every value must be an option; no duplicates. |
| `checkbox` | boolean | |

### Validation

| Rule | Applies to | Meaning |
|---|---|---|
| `minLength`, `maxLength` | text, textarea, email, url | Length limits in characters. |
| `pattern` | text, textarea, email, url | A regular expression (RE2 syntax) the whole value must match. |
| `min`, `max` | number | Inclusive bounds; `min` ≤ `max`. |

### Conditional fields: `showIf`

```yaml
showIf: { field: role, equals: designer }       # visible when role == "designer"
showIf: { field: role, notEquals: engineer }    # visible when role != "engineer"
showIf: { field: role, in: [designer, product] } # visible when role is one of these
```

Rules:

- `field` must refer to a field declared **earlier** in the list.
- Use exactly one of `equals`, `notEquals`, `in`.
- When the controlling field is a `multiselect`, `equals: x` means "contains x".
- If the controlling field is itself hidden, the dependent field is hidden too.
- Hidden fields are never required, and their values are dropped from the stored submission.

## What gets stored

When a submission arrives the server:

1. drops keys that are not fields of the form, and values of hidden fields;
2. treats `""`, `null` and `[]` as "not provided";
3. checks types, options, validation rules and required fields.

If anything fails, the API answers `422 validation_failed` with one problem per field, using paths like `data.email`. The TypeScript SDK runs the same rules in the browser for instant feedback, but the server's answer is authoritative.

## Versions

Every change to a definition creates a new immutable version. Applying identical content, compared by a canonical-JSON SHA-256 hash, creates nothing. Each submission remembers the form version and workflow version it was created with, so editing a form never changes how past submissions are displayed or processed.

The admin UI shows each version's **source** (`cli`, `ui`, `api` or `seed`). If a form was last pushed from the CLI, the editor warns that UI edits will be overwritten by the next `openforms push` unless you `openforms pull` first.

## JSON Schema

`schemas/form.schema.json` and `schemas/workflow.schema.json` describe the structure and work with editors that support YAML schemas. For example, with the VS Code YAML extension:

```yaml
# yaml-language-server: $schema=https://raw.githubusercontent.com/openforms/openforms/main/schemas/form.schema.json
```
````

- [ ] **Step 6.4: Workflows reference**

Create `docs/workflows.md`:

````markdown
# Workflows

A workflow is a state machine attached to a form. It decides what states a submission can be in, who can move it between them, what data reviewers must provide, and what happens automatically.

```yaml
slug: hiring
title: Hiring pipeline
initial: new
states:
  - { key: new, label: New, color: gray }
  - { key: screening, label: Screening, color: blue }
  - { key: hired, label: Hired, color: green, terminal: true }
  - { key: rejected, label: Rejected, color: red, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
onSubmit:
  - { type: assign, role: reviewer }
transitions:
  - key: screen
    label: Start screening
    from: [new]
    to: screening
    guard: { roles: [reviewer] }
  - key: hire
    label: Hire
    from: [screening]
    to: hired
    guard: { roles: [hiring-manager], requireFields: [score] }
    actions:
      - { type: webhook, url: "https://hr.example.com/hooks/hired" }
  - key: reject
    label: Reject
    from: [new, screening]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - type: email
        to: "{{submission.data.email}}"
        subject: Your application
        body: "{{submission.fields.rejectionReason}}"
```

Attach it to a form with `workflow: hiring` in the form definition.

## States

| Key | Required | Description |
|---|---|---|
| `key` | yes | `^[a-zA-Z][a-zA-Z0-9_]{0,63}$`, unique. |
| `label` | yes | Shown to reviewers and on the respondent's status page. |
| `color` | no | `gray`, `blue`, `green`, `yellow`, `red` or `purple`, used for badges and the diagram. |
| `terminal` | no | A final state. Transitions may not leave it. |

`initial` names the state every new submission starts in.

## Workflow fields

`fields` declares data that reviewers add to a submission over time, separate from the respondent's answers. Allowed types are `text`, `textarea`, `number`, `select` (with `options`), `checkbox` and `date`. Reviewers can edit them in the admin UI (`PATCH /submissions/{id}/fields`) or provide them while performing a transition. Setting a field to `null` clears it.

## Transitions

| Key | Required | Description |
|---|---|---|
| `key` | yes | Unique; used in API calls. |
| `label` | yes | Button text. |
| `from` | yes | Non-empty list of states the transition is available from. |
| `to` | yes | Target state. |
| `guard.roles` | no | Who may perform it. Empty means any authenticated user. Users and API keys with the `admin` role always pass. |
| `guard.requireFields` | no | Workflow fields that must have a value **after** the transition's own `fields` are merged in. |
| `actions` | no | Side effects run after the transition commits. |

### How a transition runs

`POST /api/v1/submissions/{id}/transitions` with `{"transition": "reject", "fields": {"rejectionReason": "…"}, "comment": "…", "expectedState": "screening"}`:

1. The submission row is locked, so two reviewers can't move it at once.
2. Unknown transition → `422 unknown_transition`.
3. `expectedState` given and different from the current state → `409 state_conflict`. Use it to avoid acting on stale screens.
4. Current state not in `from` → `409 invalid_state`.
5. Caller lacks every role in `guard.roles` → `403 forbidden`.
6. `fields` are type-checked and merged; any missing `requireFields` → `422 validation_failed` with paths `fields.<key>`.
7. The state and fields are saved, a `transition` event with the comment is added to the timeline, and every action is queued, all in one database transaction.

## Actions

Actions run in a background worker. They are queued in the same transaction as the state change, so they only run when the change commits, and they are retried when they fail.

### `email`

```yaml
- type: email
  to: "{{submission.data.email}}"
  subject: "Update on your application"
  body: |
    Hi {{submission.data.name}},
    Your application is now {{submission.stateLabel}}.
```

Plain-text email over SMTP (see [self-hosting](self-hosting.md)). Without SMTP settings, emails are written to the server log.

### `webhook`

```yaml
- { type: webhook, url: "https://example.com/hooks/openforms" }
```

The server sends `POST <url>` with a JSON body:

```json
{
  "event": "submission.transitioned",
  "submission": { "id": "…", "form": "hiring", "formVersion": 3, "state": "rejected", "stateLabel": "Rejected",
                  "terminal": true, "data": { … }, "fields": { … }, "assignee": null,
                  "createdAt": "…", "updatedAt": "…" },
  "transition": { "key": "reject", "label": "Reject", "from": "screening", "to": "rejected" },
  "form": { "slug": "job-application", "title": "Job application" }
}
```

`event` is `submission.created` for `onSubmit` actions, and `transition` is then `null`.

Headers:

| Header | Value |
|---|---|
| `X-OpenForms-Event` | `submission.created` or `submission.transitioned` |
| `X-OpenForms-Delivery` | Unique delivery id. Use it to deduplicate, because delivery is at least once. |
| `X-OpenForms-Signature` | `sha256=<hex HMAC-SHA256 of the raw body>` when `OPENFORMS_WEBHOOK_SECRET` is set |

Verify signatures against the **raw** request body:

```ts
// Node.js
import { createHmac, timingSafeEqual } from "node:crypto";

export function verify(rawBody: Buffer, header: string | undefined, secret: string): boolean {
  if (!header?.startsWith("sha256=")) return false;
  const expected = Buffer.from("sha256=" + createHmac("sha256", secret).update(rawBody).digest("hex"));
  const given = Buffer.from(header);
  return expected.length === given.length && timingSafeEqual(expected, given);
}
```

```go
// Go
func Verify(body []byte, header, secret string) bool {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(header))
}
```

Responses: any `2xx` is success. `408`, `429`, `5xx` and network errors are retried. Other `4xx` responses fail immediately. Requests time out after 10 seconds.

### `assign`

```yaml
- { type: assign, role: reviewer }          # least-loaded user with the role
- { type: assign, user: ada@example.com }   # a specific user
```

With `role`, openforms picks the user with that role who has the fewest assigned non-terminal submissions (ties go to the earliest-created user). If nobody has the role, it picks from admins. An unknown `user` email fails the action without retries.

### `onSubmit`

`onSubmit` actions run once when a submission is created. They are commonly used to auto-assign and send an acknowledgement email.

### Retries and failures

A failing action is retried with exponential backoff (5 s, 10 s, 20 s, … capped at 1 hour), up to 8 attempts. Every successful action adds an `action_succeeded` event to the submission's timeline. When an action finally fails, an `action_failed` event with the error is added and the job appears under **Jobs** in the admin UI, where an admin can retry it.

## Template variables

`email` actions render `{{…}}` placeholders in `to`, `subject` and `body`. Unknown variables render as an empty string.

| Variable | Value |
|---|---|
| `submission.id` | Submission UUID |
| `submission.state` / `submission.stateLabel` | Current state key / label (after the transition) |
| `submission.data.<key>` | The respondent's answer |
| `submission.fields.<key>` | A workflow field value |
| `submission.url` | Admin link: `<BASE_URL>/admin/submissions/<id>` |
| `form.slug`, `form.title` | The form |
| `transition.key`, `transition.label` | The transition being performed (empty for `onSubmit`) |
| `baseUrl` | `OPENFORMS_BASE_URL` |

## Versions and in-flight submissions

A submission keeps using the workflow version it was created with. If you rename or remove a state, existing submissions still follow the old rules, and new submissions use the new version. Applying a new workflow version automatically creates a new version of every form that references it.
````

- [ ] **Step 6.5: API reference**

Create `docs/api.md`:

````markdown
# HTTP API

Base path: `/api/v1`. All bodies are JSON with camelCase keys, and timestamps are RFC 3339 UTC.

## Authentication

| Method | How |
|---|---|
| API key | `Authorization: Bearer ofk_…`. Create keys with `openforms admin create-api-key` or in the admin UI. Keys carry roles, like users. |
| Session | `POST /auth/login` sets the `of_session` cookie (HttpOnly, 30 days). Used by the admin UI. |

Endpoints under `/public/*`, plus `/auth/login` and `/healthz`, need no authentication.

## Errors

```json
{ "error": { "code": "validation_failed", "message": "…", "details": [ { "path": "data.email", "message": "must be a valid email address" } ] } }
```

| Status | `code` | When |
|---|---|---|
| 400 | `bad_request` | Malformed JSON or body larger than 1 MiB |
| 401 | `unauthenticated` / `invalid_credentials` | Missing/invalid credentials, wrong password |
| 403 | `forbidden` | Not an admin, or a transition guard rejected your roles |
| 404 | `not_found` | Unknown resource, or a non-public form on a public endpoint |
| 409 | `email_taken`, `invalid_state`, `state_conflict`, `no_workflow` | Conflicts |
| 422 | `validation_failed`, `unknown_transition` | Invalid definitions, submission data or transition input |
| 429 | `rate_limited` | More than 20 public submissions per minute from one IP |
| 500 | `internal` | Server error (details are logged) |

## Pagination

List endpoints accept `?limit=` (default 50, max 200) and `?cursor=`. Responses have the shape `{"items": [...], "nextCursor": "…" | null}`.

## Public endpoints

| Method | Path | Description |
|---|---|---|
| GET | `/public/config` | `{"demo": bool}` |
| GET | `/public/forms/{slug}` | `{"form": <definition>}` for public forms |
| POST | `/public/forms/{slug}/submissions` | Body `{"data": {...}}` → `201 {"id","state","stateLabel","receiptToken","confirmationMessage"}` |
| GET | `/public/submissions/{id}?token=` | Status for the respondent: `{id, formTitle, state, stateLabel, terminal, states, history, createdAt}` |

```bash
curl -X POST http://localhost:8080/api/v1/public/forms/contact/submissions \
  -H 'Content-Type: application/json' \
  -d '{"data":{"name":"Ada","email":"ada@example.com","topic":"question","message":"How do workflows work?"}}'
```

Keep the `receiptToken`. It is the only way to read the public status, and it is never shown again.

## Auth, users and API keys

| Method | Path | Auth | Description |
|---|---|---|---|
| POST | `/auth/login` | none | `{email, password}` → `{user}` + cookie |
| POST | `/auth/logout` | session | 204 |
| GET | `/auth/me` | any | `{"principal": {kind, id, name, email, roles}}` |
| GET / POST | `/users` | admin | List / create `{email, name, password, roles}` |
| PATCH / DELETE | `/users/{id}` | admin | Update `{name?, password?, roles?}` / delete |
| GET / POST | `/api-keys` | admin | List / create `{name, roles}` → `{apiKey, key}` (plaintext `key` shown once) |
| DELETE | `/api-keys/{id}` | admin | Revoke |

## Definitions

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/forms` | any | `{"items": [{slug, title, workflow, public, version, source, updatedAt, submissionCount}]}` |
| GET | `/forms/{slug}` | any | `{"form": {slug, version, source, updatedAt, workflowVersion, definition}}` |
| PUT | `/forms/{slug}` | admin | Body = definition; `?source=ui` optional → `{"item": {kind, slug, version, changed, created}}` |
| GET | `/forms/{slug}/versions` | any | `{"items": [{version, hash, source, createdBy, createdAt}]}` newest first |
| GET | `/forms/{slug}/versions/{n}` | any | One version |
| GET | `/forms/{slug}/submissions.csv` | any | CSV export |
| GET, PUT | `/workflows`, `/workflows/{slug}`, `/workflows/{slug}/versions[/{n}]` | as forms | Workflow equivalents |
| POST | `/definitions/validate` | admin | Body `{forms: [...], workflows: [...]}` → `{"valid": true}` or 422 |
| POST | `/definitions/apply` | admin | Same body; `?dryRun=true`, `?source=cli` → `{"items": [ApplyItem]}`; atomic |
| GET | `/definitions` | admin | Export everything as `{forms, workflows}` |

## Submissions

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/submissions` | any | Filters: `form`, `state`, `assignee` (`<uuid>`, `me`, `none`) |
| POST | `/submissions` | any | Create for any form (public or not): `{"form": slug, "data": {...}}` → `201 {submission, receiptToken}` |
| GET | `/submissions/{id}` | any | `{submission, form, workflow, events, transitions}`; `transitions` shows availability for **you** |
| POST | `/submissions/{id}/transitions` | any | `{transition, fields?, comment?, expectedState?}` → `{submission}` |
| PATCH | `/submissions/{id}/fields` | any | `{"fields": {...}}` (null clears) → `{submission}` |
| POST | `/submissions/{id}/comments` | any | `{"body"}` → `201 {event}` |
| PUT | `/submissions/{id}/assignee` | any | `{"userId": uuid | null}` → `{submission}` |

### Shapes

```jsonc
// Submission
{ "id": "…", "form": "job-application", "formVersion": 2, "state": "screening", "stateLabel": "Screening",
  "terminal": false, "data": { "name": "Ada" }, "fields": { "score": 4 },
  "assignee": { "id": "…", "name": "Riley Reviewer", "email": "reviewer@demo.local" },
  "createdAt": "…", "updatedAt": "…" }

// Event (timeline entry); type: created | transition | fields_updated | assigned | comment | action_succeeded | action_failed
{ "id": 42, "type": "transition", "fromState": "new", "toState": "screening", "transition": "screen",
  "actor": { "type": "user", "id": "…", "name": "Riley Reviewer" }, "payload": { "comment": "Looks strong" },
  "createdAt": "…" }

// AvailableTransition
{ "key": "invite", "label": "Invite to interview", "to": "interview", "toLabel": "Interview",
  "requireFields": ["score"], "allowed": true, "reason": "" }
```

## Jobs

| Method | Path | Auth | Description |
|---|---|---|---|
| GET | `/jobs?status=failed` | admin | `{"items": [{id, kind, status, attempts, maxAttempts, runAt, lastError, payload, createdAt, updatedAt}]}` |
| POST | `/jobs/{id}/retry` | admin | Re-queue a failed job → 204 |

## Health

`GET /healthz` (outside `/api/v1`) → `{"status":"ok"}` when the database is reachable.
````

- [ ] **Step 6.6: CLI reference**

Create `docs/cli.md`:

````markdown
# CLI reference

`openforms` is one binary: it runs the server and it manages definitions.

```bash
make build            # → bin/openforms
# or, with Docker:
docker run --rm openforms:dev --help
```

## Connecting to a server

Remote commands (`push`, `pull`, `diff`) resolve settings in this order:

1. flags `--server` and `--api-key`
2. environment `OPENFORMS_URL` and `OPENFORMS_API_KEY`
3. `openforms.yaml` in the current directory:

```yaml
server: http://localhost:8080
dir: openforms
```

Server commands (`serve`, `migrate`, `seed`, `admin …`) connect to the database directly using `OPENFORMS_DATABASE_URL` and the other variables in [self-hosting](self-hosting.md).

## Definitions layout

```
openforms/
  forms/<slug>.yaml
  workflows/<slug>.yaml
```

`.yaml`, `.yml` and `.json` files are read. A file's `slug` must equal its file name.

## Commands

### `openforms init [dir]`

Creates `openforms.yaml`, `openforms/forms/contact.yaml` and `openforms/workflows/contact-triage.yaml` in `dir` (default `.`). It never overwrites existing files.

### `openforms validate [--dir openforms]`

Checks every definition offline, including references between forms and workflows. Problems are printed as `file:path: message`, for example:

```
openforms/forms/contact.yaml:fields[2].showIf.field: must reference an earlier field
```

Exit code 0 when valid, 1 otherwise.

### `openforms push [--dir openforms] [--dry-run]`

Validates locally, then applies all definitions atomically (`POST /api/v1/definitions/apply?source=cli`):

```
KIND      SLUG             VERSION  STATUS
workflow  contact-triage   3        updated
form      contact          4        updated
form      newsletter       1        created
```

Unchanged files report `unchanged` and create no versions. `--dry-run` shows the same table without saving anything.

### `openforms pull [--dir openforms] [--check]`

Downloads every definition and writes canonical YAML files. `--check` writes nothing and exits 1 if any file would change, which makes it the CI drift check.

### `openforms diff [--dir openforms]`

Compares local files with the server:

```
~ form contact            differs
+ form newsletter         local only
- workflow old-triage     remote only
```

### `openforms serve`

Runs migrations and starts the HTTP server and background worker. With `OPENFORMS_SEED_DEMO=true` it also seeds the demo bundle.

### `openforms migrate`

Runs database migrations and exits.

### `openforms seed --demo`

Loads the example forms and workflows (source `seed`) and creates the demo users `admin@demo.local`, `reviewer@demo.local` and `manager@demo.local` (password `demo1234`) if they don't exist. Safe to run repeatedly.

### `openforms admin create-user --email E --name N --password P --roles r1,r2`

Creates a user. Use this to create your first admin (`--roles admin`).

### `openforms admin create-api-key --name N --roles r1,r2`

Creates an API key and prints it once.

## CI recipe (GitHub Actions)

```yaml
- run: openforms validate
- run: openforms pull --check
  env:
    OPENFORMS_URL: ${{ secrets.OPENFORMS_URL }}
    OPENFORMS_API_KEY: ${{ secrets.OPENFORMS_API_KEY }}
- if: github.ref == 'refs/heads/main'
  run: openforms push
  env:
    OPENFORMS_URL: ${{ secrets.OPENFORMS_URL }}
    OPENFORMS_API_KEY: ${{ secrets.OPENFORMS_API_KEY }}
```
````

- [ ] **Step 6.7: Self-hosting guide**

Create `docs/self-hosting.md`:

````markdown
# Self-hosting

openforms is one container plus PostgreSQL 16. The web UI is compiled into the binary.

## Docker Compose (production)

```yaml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: openforms
      POSTGRES_PASSWORD: change-me
      POSTGRES_DB: openforms
    volumes:
      - pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U openforms -d openforms"]
      interval: 5s
      retries: 20
    restart: unless-stopped

  openforms:
    image: ghcr.io/openforms/openforms:latest   # or build: .
    environment:
      OPENFORMS_DATABASE_URL: postgres://openforms:change-me@postgres:5432/openforms?sslmode=disable
      OPENFORMS_BASE_URL: https://forms.example.com
      OPENFORMS_SMTP_HOST: smtp.example.com
      OPENFORMS_SMTP_PORT: "587"
      OPENFORMS_SMTP_USERNAME: forms@example.com
      OPENFORMS_SMTP_PASSWORD: change-me
      OPENFORMS_SMTP_FROM: forms@example.com
      OPENFORMS_WEBHOOK_SECRET: a-long-random-string
    ports:
      - "127.0.0.1:8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    restart: unless-stopped

volumes:
  pgdata: {}
```

Create the first admin:

```bash
docker compose exec openforms /openforms admin create-user \
  --email you@example.com --name "Your Name" --password 'a-strong-password' --roles admin
```

## Configuration

| Variable | Default | Description |
|---|---|---|
| `OPENFORMS_DATABASE_URL` | none (required) | PostgreSQL connection string |
| `OPENFORMS_HTTP_ADDR` | `:8080` | Listen address |
| `OPENFORMS_BASE_URL` | `http://localhost:8080` | Public URL; used in links in emails and webhooks |
| `OPENFORMS_COOKIE_SECURE` | `true` when `BASE_URL` is https | Mark the session cookie `Secure` |
| `OPENFORMS_SMTP_HOST` | none (empty) | SMTP server; when empty, emails are logged instead of sent |
| `OPENFORMS_SMTP_PORT` | `1025` | SMTP port |
| `OPENFORMS_SMTP_USERNAME` / `OPENFORMS_SMTP_PASSWORD` | none (empty) | SMTP credentials |
| `OPENFORMS_SMTP_FROM` | `openforms@localhost` | Sender address |
| `OPENFORMS_WEBHOOK_SECRET` | none (empty) | HMAC secret for `X-OpenForms-Signature`; when empty, webhooks are unsigned |
| `OPENFORMS_WORKER_CONCURRENCY` | `4` | Parallel background jobs per instance |
| `OPENFORMS_DEMO` | `false` | Demo mode: shows demo accounts on the login page and enables the /demo reviewer simulation. **Never enable in production.** |
| `OPENFORMS_SEED_DEMO` | `false` | Seed the demo bundle and demo users on start |

## Reverse proxy and TLS

Put openforms behind a TLS-terminating proxy and set `OPENFORMS_BASE_URL` to the public https URL. With Caddy:

```
forms.example.com {
  reverse_proxy 127.0.0.1:8080
}
```

The proxy must pass the client IP (`X-Forwarded-For`) for per-IP rate limiting of public submissions.

## Upgrades

Pull the new image and restart. `openforms serve` runs pending database migrations on start. To migrate separately, for example in a release job, run `openforms migrate` first.

## Backups

All state lives in PostgreSQL:

```bash
docker compose exec -T postgres pg_dump -U openforms -Fc openforms > openforms-$(date +%F).dump
# restore
docker compose exec -T postgres pg_restore -U openforms -d openforms --clean < openforms-2026-09-23.dump
```

## Running more than one instance

- Background jobs are claimed with `SELECT … FOR UPDATE SKIP LOCKED`, so several instances can share one database safely. A job whose worker dies is picked up again after 5 minutes.
- The public submission rate limit (20 per minute per IP) is kept in memory per instance.
- Webhook delivery is at least once. Receivers should deduplicate on `X-OpenForms-Delivery`.

## Security checklist

- Use a strong database password and keep Postgres off the public network.
- Keep `OPENFORMS_DEMO` and `OPENFORMS_SEED_DEMO` off, since the demo accounts have a published password.
- Set `OPENFORMS_WEBHOOK_SECRET` and verify signatures in your receivers ([workflows](workflows.md#webhook)).
- Give API keys only the roles they need, and revoke unused keys in **Admin → API keys**.
````

- [ ] **Step 6.8: Check links and commands**

Run: `grep -o '](\([a-z-]*\.md\)[^)]*)' README.md docs/*.md | sort -u`
Expected: every target listed (`getting-started.md`, `definitions.md`, `workflows.md`, `api.md`, `cli.md`, `self-hosting.md`) exists under `docs/`. Check with `ls docs/`.

Run: `go run ./cmd/openforms --help`
Expected: every command documented in `docs/cli.md` (`serve`, `migrate`, `seed`, `admin`, `init`, `validate`, `push`, `pull`, `diff`) is listed. If Plan 05 used a different flag name, fix the doc, not the code.

- [ ] **Step 6.9: Walk through Getting started**

With the compose stack running (`make up`), follow `docs/getting-started.md` steps 1–6 literally in a scratch directory (`mktemp -d`). Every command must succeed as written. Correct the doc wherever reality differs.

- [ ] **Step 6.10: Commit**

```bash
git add README.md docs/getting-started.md docs/definitions.md docs/workflows.md docs/api.md docs/cli.md docs/self-hosting.md
git commit -m "docs: add README and user documentation"
```

---

### Task 7: Playwright end-to-end tests (`e2e/`)

**Depends on:** Tasks 1–5 and Plans 01–08, all merged.

**Files:**
- Create: `e2e/package.json`
- Create: `e2e/tsconfig.json`
- Create: `e2e/.gitignore`
- Create: `e2e/playwright.config.ts`
- Create: `e2e/global-setup.ts`
- Create: `e2e/lib/env.ts`
- Create: `e2e/lib/api.ts`
- Create: `e2e/lib/mailpit.ts`
- Create: `e2e/lib/webhook-sink.ts`
- Create: `e2e/lib/admin.ts`
- Create: `e2e/fixtures/bundle.ts`
- Test: `e2e/tests/hosted-submit.spec.ts`
- Test: `e2e/tests/review-workflow.spec.ts`
- Test: `e2e/tests/demo.spec.ts`
- Test: `e2e/tests/form-editor.spec.ts`

**Interfaces:**
- Consumes: the compose `app` stack (Task 4): openforms on `:8080` with `OPENFORMS_WEBHOOK_SECRET=e2e-secret` and `host.docker.internal` mapped to the host; Mailpit API on `:8025` (`GET /api/v1/messages`, `DELETE /api/v1/messages`); `openforms admin create-api-key` output containing an `ofk_…` key (Plan 05); API endpoints §7; the hosted page link text "Track your submission" (spec §9.3); the demo page (Task 5); the admin login, detail page and form editor (Plans 07/08).
- Produces: `make e2e` / CI `e2e` job; `e2e/.state/state.json` (`{ apiKey }`) shared between global setup and specs.

**Selector policy:** use roles, labels and visible text. The labels come from definitions, which this plan controls. For admin pages, the tests use the accessible names Plans 07/08 are required to provide: labelled inputs, buttons named by their text, and `role="dialog"` for the transition dialog. If an admin selector does not match, fix the admin page's accessibility, not the assertion.

- [ ] **Step 7.1: Scaffold the package**

Create `e2e/package.json`:

```json
{
  "name": "openforms-e2e",
  "private": true,
  "type": "module",
  "scripts": {
    "test": "playwright test",
    "typecheck": "tsc --noEmit"
  },
  "devDependencies": {
    "@playwright/test": "^1.49.1",
    "@types/node": "^22.10.0",
    "typescript": "^5.7.2"
  }
}
```

Create `e2e/tsconfig.json`:

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "module": "ESNext",
    "moduleResolution": "Bundler",
    "strict": true,
    "types": ["node"],
    "noEmit": true,
    "skipLibCheck": true
  },
  "include": ["**/*.ts"]
}
```

Create `e2e/.gitignore`:

```
node_modules/
.state/
playwright-report/
test-results/
```

Run: `cd e2e && pnpm install && pnpm exec playwright install chromium`
Expected: `pnpm-lock.yaml` is created and Chromium is downloaded.

- [ ] **Step 7.2: Shared helpers**

Create `e2e/lib/env.ts`:

```ts
import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

export const E2E_DIR = join(dirname(fileURLToPath(import.meta.url)), "..");
export const REPO_ROOT = join(E2E_DIR, "..");
export const BASE_URL = process.env.E2E_BASE_URL ?? "http://localhost:8080";
export const MAILPIT_URL = process.env.E2E_MAILPIT_URL ?? "http://localhost:8025";
export const WEBHOOK_SECRET = process.env.E2E_WEBHOOK_SECRET ?? "e2e-secret";
export const SINK_PORT = 9911;

export const E2E_REVIEWER = {
  email: "e2e-reviewer@example.com",
  name: "E2E Reviewer",
  password: "password123",
};

export interface E2EState {
  apiKey: string;
}

const STATE_FILE = join(E2E_DIR, ".state", "state.json");

export function writeState(state: E2EState): void {
  mkdirSync(dirname(STATE_FILE), { recursive: true });
  writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));
}

export function readState(): E2EState {
  return JSON.parse(readFileSync(STATE_FILE, "utf8")) as E2EState;
}
```

Create `e2e/lib/api.ts`:

```ts
import { BASE_URL, readState } from "./env";

export interface ApiResponse<T> {
  status: number;
  body: T;
}

export async function api<T = unknown>(
  method: string,
  path: string,
  body?: unknown,
  apiKey: string | null = readState().apiKey,
): Promise<ApiResponse<T>> {
  const headers: Record<string, string> = {};
  if (body !== undefined) headers["Content-Type"] = "application/json";
  if (apiKey) headers.Authorization = `Bearer ${apiKey}`;
  const res = await fetch(BASE_URL + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const text = await res.text();
  return { status: res.status, body: (text ? JSON.parse(text) : null) as T };
}

export async function createPublicSubmission(
  slug: string,
  data: Record<string, unknown>,
): Promise<{ id: string; receiptToken: string }> {
  const res = await api<{ id: string; receiptToken: string }>(
    "POST",
    `/api/v1/public/forms/${slug}/submissions`,
    { data },
    null,
  );
  if (res.status !== 201) throw new Error(`submit ${slug}: ${res.status} ${JSON.stringify(res.body)}`);
  return res.body;
}
```

Create `e2e/lib/mailpit.ts`:

```ts
import { MAILPIT_URL } from "./env";

export interface MailpitMessage {
  ID: string;
  Subject: string;
  To: Array<{ Address: string; Name: string }>;
}

export async function listMessages(): Promise<MailpitMessage[]> {
  const res = await fetch(`${MAILPIT_URL}/api/v1/messages?limit=500`);
  if (!res.ok) throw new Error(`mailpit: ${res.status}`);
  const body = (await res.json()) as { messages?: MailpitMessage[] };
  return body.messages ?? [];
}

export async function clearMessages(): Promise<void> {
  await fetch(`${MAILPIT_URL}/api/v1/messages`, { method: "DELETE" });
}
```

Create `e2e/lib/webhook-sink.ts`:

```ts
import { createServer } from "node:http";
import { SINK_PORT } from "./env";

export interface Delivery {
  path: string;
  headers: Record<string, string>;
  body: string;
  receivedAt: string;
}

/** Records every POST; GET /received returns the recorded deliveries (tests run in other processes). */
export function startWebhookSink(port = SINK_PORT): Promise<{ close(): Promise<void> }> {
  const deliveries: Delivery[] = [];
  const server = createServer((req, res) => {
    if (req.method === "GET" && req.url === "/received") {
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(deliveries));
      return;
    }
    if (req.method === "POST") {
      const chunks: Buffer[] = [];
      req.on("data", (c: Buffer) => chunks.push(c));
      req.on("end", () => {
        const headers: Record<string, string> = {};
        for (const [k, v] of Object.entries(req.headers)) headers[k] = Array.isArray(v) ? v.join(",") : (v ?? "");
        deliveries.push({
          path: req.url ?? "",
          headers,
          body: Buffer.concat(chunks).toString("utf8"),
          receivedAt: new Date().toISOString(),
        });
        res.writeHead(204);
        res.end();
      });
      return;
    }
    res.writeHead(404);
    res.end();
  });
  return new Promise((resolve, reject) => {
    server.once("error", reject);
    server.listen(port, "0.0.0.0", () =>
      resolve({ close: () => new Promise<void>((r) => server.close(() => r())) }),
    );
  });
}

export async function receivedDeliveries(): Promise<Delivery[]> {
  const res = await fetch(`http://localhost:${SINK_PORT}/received`);
  return (await res.json()) as Delivery[];
}
```

Create `e2e/lib/admin.ts`:

```ts
import { expect, type Page } from "@playwright/test";

export async function loginAdmin(page: Page, email: string, password: string): Promise<void> {
  await page.goto("/admin/login");
  await page.getByLabel(/email/i).fill(email);
  await page.getByLabel(/password/i).fill(password);
  await page.getByRole("button", { name: /sign in|log in/i }).click();
  await expect(page).not.toHaveURL(/\/admin\/login/);
}
```

Create `e2e/fixtures/bundle.ts`:

```ts
/** E2E-only definitions: the webhook points at the sink on the host (see docker-compose extra_hosts). */
export const E2E_WORKFLOW = {
  slug: "e2e-flow",
  title: "E2E flow",
  initial: "new",
  states: [
    { key: "new", label: "New", color: "gray" },
    { key: "approved", label: "Approved", color: "green", terminal: true },
    { key: "rejected", label: "Rejected", color: "red", terminal: true },
  ],
  fields: [{ key: "note", type: "textarea", label: "Note" }],
  transitions: [
    {
      key: "approve",
      label: "Approve",
      from: ["new"],
      to: "approved",
      guard: { roles: ["reviewer"], requireFields: ["note"] },
      actions: [
        { type: "webhook", url: "http://host.docker.internal:9911/hook" },
        {
          type: "email",
          to: "{{submission.data.email}}",
          subject: "E2E approved {{submission.id}}",
          body: "Note: {{submission.fields.note}}",
        },
      ],
    },
    { key: "reject", label: "Reject", from: ["new"], to: "rejected", guard: { roles: ["reviewer"] } },
  ],
};

export const E2E_FORM = {
  slug: "e2e-apply",
  title: "E2E application",
  workflow: "e2e-flow",
  settings: { public: true, submitLabel: "Send", confirmationMessage: "Thanks, e2e!" },
  fields: [
    { key: "name", type: "text", label: "Name", required: true },
    { key: "email", type: "email", label: "Email", required: true },
  ],
};
```

- [ ] **Step 7.3: Global setup and config**

Create `e2e/global-setup.ts`:

```ts
import { execFileSync } from "node:child_process";
import { api } from "./lib/api";
import { BASE_URL, E2E_REVIEWER, REPO_ROOT, writeState } from "./lib/env";
import { clearMessages } from "./lib/mailpit";
import { startWebhookSink } from "./lib/webhook-sink";
import { E2E_FORM, E2E_WORKFLOW } from "./fixtures/bundle";

async function waitForHealthy(url: string, timeoutMs: number): Promise<void> {
  const deadline = Date.now() + timeoutMs;
  let last = "";
  while (Date.now() < deadline) {
    try {
      const res = await fetch(url);
      if (res.ok) return;
      last = `HTTP ${res.status}`;
    } catch (e) {
      last = String(e);
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  throw new Error(`openforms not healthy at ${url} after ${timeoutMs}ms (${last}). Run: docker compose --profile app up -d --build`);
}

function createApiKey(): string {
  const out = execFileSync(
    "docker",
    ["compose", "--profile", "app", "exec", "-T", "openforms", "/openforms", "admin", "create-api-key", "--name", `e2e-${Date.now()}`, "--roles", "admin"],
    { cwd: REPO_ROOT, encoding: "utf8" },
  );
  const match = out.match(/ofk_[A-Za-z0-9]+/);
  if (!match) throw new Error(`could not find an API key in output:\n${out}`);
  return match[0];
}

export default async function globalSetup() {
  await waitForHealthy(`${BASE_URL}/healthz`, 180_000);
  const sink = await startWebhookSink();

  const apiKey = process.env.OPENFORMS_E2E_API_KEY ?? createApiKey();
  writeState({ apiKey });

  const applied = await api("POST", "/api/v1/definitions/apply?source=api", {
    forms: [E2E_FORM],
    workflows: [E2E_WORKFLOW],
  });
  if (applied.status !== 200) throw new Error(`apply e2e bundle: ${applied.status} ${JSON.stringify(applied.body)}`);

  const user = await api("POST", "/api/v1/users", { ...E2E_REVIEWER, roles: ["reviewer"] });
  if (![200, 201, 409].includes(user.status)) throw new Error(`create reviewer: ${user.status} ${JSON.stringify(user.body)}`);

  await clearMessages();

  return async () => {
    await sink.close();
  };
}
```

Create `e2e/playwright.config.ts`:

```ts
import { defineConfig, devices } from "@playwright/test";
import { BASE_URL } from "./lib/env";

export default defineConfig({
  testDir: "./tests",
  globalSetup: "./global-setup.ts",
  fullyParallel: false,
  workers: 1,
  retries: process.env.CI ? 1 : 0,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [["list"], ["html", { open: "never" }]],
  use: {
    baseURL: BASE_URL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  projects: [{ name: "chromium", use: { ...devices["Desktop Chrome"] } }],
});
```

- [ ] **Step 7.4: Write the specs**

Create `e2e/tests/hosted-submit.spec.ts`:

```ts
import { expect, test } from "@playwright/test";

test("respondent submits the hosted form and tracks its status", async ({ page }) => {
  await page.goto("/f/e2e-apply");
  await expect(page.getByRole("heading", { name: "E2E application" })).toBeVisible();

  await page.getByLabel(/^Name/).fill("Hosted Tester");
  await page.getByLabel(/^Email/).fill(`hosted+${Date.now()}@example.com`);
  await page.getByRole("button", { name: "Send" }).click();

  await expect(page.getByText("Thanks, e2e!")).toBeVisible();
  await page.getByRole("link", { name: "Track your submission" }).click();

  await expect(page).toHaveURL(/\/s\/[0-9a-f-]{36}\?token=/);
  await expect(page.getByText("New").first()).toBeVisible();
});

test("validation errors keep the respondent on the form", async ({ page }) => {
  await page.goto("/f/e2e-apply");
  await page.getByLabel(/^Name/).fill("No Email");
  await page.getByLabel(/^Email/).fill("not-an-email");
  await page.getByRole("button", { name: "Send" }).click();
  await expect(page.getByLabel(/^Email/)).toHaveAttribute("aria-invalid", "true");
  await expect(page.getByText("Thanks, e2e!")).toHaveCount(0);
});

test("unknown forms show a friendly page", async ({ page }) => {
  await page.goto("/f/does-not-exist");
  await expect(page.getByText("This form isn't available")).toBeVisible();
});
```

Create `e2e/tests/review-workflow.spec.ts`:

```ts
import { createHmac } from "node:crypto";
import { expect, test } from "@playwright/test";
import { api, createPublicSubmission } from "../lib/api";
import { loginAdmin } from "../lib/admin";
import { E2E_REVIEWER, WEBHOOK_SECRET } from "../lib/env";
import { listMessages } from "../lib/mailpit";
import { receivedDeliveries, type Delivery } from "../lib/webhook-sink";

test.describe.serial("review workflow", () => {
  const email = `e2e+${Date.now()}@example.com`;
  let sub: { id: string; receiptToken: string };

  test.beforeAll(async () => {
    sub = await createPublicSubmission("e2e-apply", { name: "Workflow Tester", email });
  });

  test("guard rejects approve without the required note", async () => {
    const res = await api<{ error: { code: string; details?: Array<{ path: string }> } }>(
      "POST",
      `/api/v1/submissions/${sub.id}/transitions`,
      { transition: "approve" },
    );
    expect(res.status).toBe(422);
    expect(res.body.error.code).toBe("validation_failed");
    expect(res.body.error.details).toEqual(expect.arrayContaining([expect.objectContaining({ path: "fields.note" })]));
  });

  test("reviewer approves in the admin UI, providing the note", async ({ page }) => {
    await loginAdmin(page, E2E_REVIEWER.email, E2E_REVIEWER.password);
    await page.goto(`/admin/submissions/${sub.id}`);

    await page.getByRole("button", { name: "Approve" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByLabel("Note").fill("Great candidate");
    await dialog.getByRole("button", { name: /^(Approve|Confirm)$/ }).click();

    await expect(dialog).toBeHidden();
    await expect(page.getByText("Approved").first()).toBeVisible();
    await expect(page.getByRole("button", { name: "Approve" })).toHaveCount(0);
  });

  test("respondent status page reflects the transition", async ({ page }) => {
    await page.goto(`/s/${sub.id}?token=${encodeURIComponent(sub.receiptToken)}`);
    await expect(page.getByText("Approved").first()).toBeVisible();
    const status = await api<{ state: string; terminal: boolean }>(
      "GET",
      `/api/v1/public/submissions/${sub.id}?token=${encodeURIComponent(sub.receiptToken)}`,
      undefined,
      null,
    );
    expect(status.body).toMatchObject({ state: "approved", terminal: true });
  });

  test("approval email is delivered to Mailpit", async () => {
    await expect
      .poll(
        async () => (await listMessages()).find((m) => m.Subject === `E2E approved ${sub.id}`)?.To[0]?.Address,
        { timeout: 30_000 },
      )
      .toBe(email);
  });

  test("webhook is delivered once, signed with the shared secret", async () => {
    let delivery: Delivery | undefined;
    await expect
      .poll(
        async () => {
          delivery = (await receivedDeliveries()).find(
            (d) =>
              d.headers["x-openforms-event"] === "submission.transitioned" &&
              JSON.parse(d.body).submission?.id === sub.id,
          );
          return Boolean(delivery);
        },
        { timeout: 30_000 },
      )
      .toBe(true);

    const body = JSON.parse(delivery!.body);
    expect(body.transition).toMatchObject({ key: "approve", from: "new", to: "approved" });
    expect(body.form).toMatchObject({ slug: "e2e-apply" });
    const expected = "sha256=" + createHmac("sha256", WEBHOOK_SECRET).update(delivery!.body).digest("hex");
    expect(delivery!.headers["x-openforms-signature"]).toBe(expected);
    expect(delivery!.headers["x-openforms-delivery"]).toMatch(/^\d+$/);
  });

  test("timeline records the transition and successful actions", async () => {
    await expect
      .poll(
        async () => {
          const res = await api<{ events: Array<{ type: string }> }>("GET", `/api/v1/submissions/${sub.id}`);
          return res.body.events.filter((e) => e.type === "action_succeeded").length;
        },
        { timeout: 30_000 },
      )
      .toBe(2);
    const res = await api<{ events: Array<{ type: string; transition: string | null }> }>(
      "GET",
      `/api/v1/submissions/${sub.id}`,
    );
    expect(res.body.events.map((e) => e.type)).toEqual(expect.arrayContaining(["created", "transition"]));
  });
});
```

Create `e2e/tests/demo.spec.ts`:

```ts
import { expect, test } from "@playwright/test";
import { listMessages } from "../lib/mailpit";

test("visitor submits on /demo and plays the reviewer", async ({ page }) => {
  const email = `demo+${Date.now()}@example.com`;
  await page.goto("/demo");

  const collect = page.getByRole("region", { name: /Collect/ });
  await collect.getByLabel(/Full name/).fill("Demo Visitor");
  await collect.getByLabel(/^Email/).fill(email);
  await collect.getByLabel(/^Role/).selectOption("engineer");
  await collect.getByLabel(/Years of experience/).fill("5");
  await collect.getByLabel(/I agree/).check();
  await collect.getByRole("button", { name: "Send application" }).click();

  const track = page.getByRole("region", { name: /Track & review/ });
  await expect(track.getByRole("link", { name: "Open in the admin inbox" })).toHaveAttribute(
    "href",
    /\/admin\/submissions\/[0-9a-f-]{36}$/,
  );

  await track.getByRole("button", { name: "Start screening" }).click();
  // Now in "screening": the reviewer panel offers the next transition, which needs a score.
  const invite = track.getByRole("button", { name: "Invite to interview" });
  await expect(invite).toBeDisabled();
  await track.getByLabel("Score").fill("4");
  await invite.click();

  // In "interview", only the hiring manager may hire.
  await expect(track.getByText("Not allowed for Reviewer")).toBeVisible();
  await track.getByRole("button", { name: "Hiring manager" }).click();
  await track.getByRole("button", { name: "Hire" }).click();
  await expect(track.getByText(/reached a final state: Hired/)).toBeVisible();

  await expect
    .poll(async () => (await listMessages()).filter((m) => m.To[0]?.Address === email).map((m) => m.Subject).sort(), {
      timeout: 30_000,
    })
    .toEqual(["Let's talk — interview invitation", "We got your application", "Welcome aboard!"]);
});

test("demo embed renders the contact form", async ({ page }) => {
  await page.goto("/demo");
  const frame = page.frameLocator('iframe[src*="/f/contact"]');
  await expect(frame.getByLabel(/Your name/)).toBeVisible();
});
```

Create `e2e/tests/form-editor.spec.ts`:

```ts
import { expect, test } from "@playwright/test";
import { api } from "../lib/api";
import { loginAdmin } from "../lib/admin";

type Versions = { items: Array<{ version: number; source: string }> };

test("saving the visual form editor creates a new version", async ({ page }) => {
  const before = await api<Versions>("GET", "/api/v1/forms/contact/versions");
  expect(before.status).toBe(200);

  await loginAdmin(page, "admin@demo.local", "demo1234");
  await page.goto("/admin/forms/contact/edit");

  await page.getByText(/^Your name/).first().click();
  const label = `Your name (${Date.now()})`;
  await page.getByLabel("Label", { exact: true }).fill(label);
  await page.getByRole("button", { name: "Save" }).click();

  await expect
    .poll(async () => (await api<Versions>("GET", "/api/v1/forms/contact/versions")).body.items.length)
    .toBe(before.body.items.length + 1);
  const after = await api<Versions>("GET", "/api/v1/forms/contact/versions");
  expect(after.body.items[0].source).toBe("ui");

  const form = await api<{ form: { definition: { fields: Array<{ key: string; label: string }> } } }>(
    "GET",
    "/api/v1/forms/contact",
  );
  expect(form.body.form.definition.fields.find((f) => f.key === "name")?.label).toBe(label);
});
```

- [ ] **Step 7.5: Prove the suite can fail**

Tests written after the features exist must still be able to fail. Run one spec against a broken precondition:

```bash
docker compose --profile app up -d --build
cd e2e && E2E_WEBHOOK_SECRET=wrong-secret pnpm exec playwright test tests/review-workflow.spec.ts
```

Expected: FAIL in "webhook is delivered once, signed with the shared secret", with a signature mismatch (`Expected: "sha256=…"` / `Received: "sha256=…"`). Every other test in the file passes.

- [ ] **Step 7.6: Run the full suite**

Run: `cd e2e && pnpm typecheck && pnpm test`
Expected: all specs pass. The counts are 3 in hosted-submit, 6 in review-workflow, 2 in demo and 1 in form-editor, for 12 in total.

If `demo.spec.ts` fails because the Mailpit subjects include earlier runs, check that `clearMessages()` ran in global setup. The email filter is per-run anyway, since the address is unique.

Run from the repo root: `make down && make e2e`
Expected: the stack builds from scratch, is seeded, and all 12 tests pass.

- [ ] **Step 7.7: Commit**

```bash
git add e2e/
git commit -m "test(e2e): add Playwright suite for hosted, review, demo and editor flows"
```

---

## Self-review checklist (run after Task 7)

- [ ] Spec §9.6 sections: Hero, Define, Collect, Track & review (tracker + "Play the reviewer" + inbox link), Embed (live + snippet), CLI. All six are present, and `App.test.tsx` "renders every section" pins them.
- [ ] Spec §10: demo users and password (Task 2 `DemoUsers`), `/public/config` plus login credentials (Task 3), Dockerfile stages (Task 4), compose profile with `OPENFORMS_SEED_DEMO` (Task 4), Makefile targets (Task 4), CI (Task 4), and the six docs (Task 6).
- [ ] Spec §12 E2E list: hosted submit → status (hosted-submit); admin transition with a required field → status updates (review-workflow); Mailpit email (review-workflow and demo); webhook to a local sink (review-workflow); demo reviewer loop (demo); editor save creates a version (form-editor).
- [ ] `go test ./... && make web-test && make e2e` pass on a clean checkout.

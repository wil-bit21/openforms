// Package fixture sets up a database with a hiring workflow, a form that uses
// it ("apply") and a form without a workflow ("feedback"). It is shared by the
// actions, workflow, httpapi and app tests of Plan 04.
package fixture

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

const WorkflowYAML = `
slug: hiring
title: Hiring
initial: new
states:
  - { key: new, label: New }
  - { key: screening, label: Screening }
  - { key: interview, label: Interview }
  - { key: hired, label: Hired, terminal: true }
  - { key: rejected, label: Rejected, terminal: true }
fields:
  - { key: score, type: number, label: Score }
  - { key: rejectionReason, type: textarea, label: Rejection reason }
  - key: priority
    type: select
    label: Priority
    options:
      - { value: low, label: Low }
      - { value: high, label: High }
onSubmit:
  - { type: email, to: "{{submission.data.email}}", subject: "Thanks {{submission.data.name}}", body: "We received your application." }
transitions:
  - { key: screen, label: Start screening, from: [new], to: screening, guard: { roles: [reviewer] } }
  - key: invite
    label: Invite to interview
    from: [screening]
    to: interview
    guard: { roles: [reviewer], requireFields: [score] }
    actions:
      - { type: webhook, url: "http://127.0.0.1:9/hook" }
  - { key: hire, label: Hire, from: [interview], to: hired, guard: { roles: [hiring-manager] } }
  - key: reject
    label: Reject
    from: [new, screening, interview]
    to: rejected
    guard: { roles: [reviewer, hiring-manager], requireFields: [rejectionReason] }
    actions:
      - { type: email, to: "{{submission.data.email}}", subject: "Your application", body: "{{submission.fields.rejectionReason}}" }
      - { type: assign, role: hiring-manager }
`

const FormYAML = `
slug: apply
title: Apply
workflow: hiring
settings: { public: true }
fields:
  - { key: name, type: text, label: Name, required: true }
  - { key: email, type: email, label: Email, required: true }
`

const PlainFormYAML = `
slug: feedback
title: Feedback
settings: { public: true }
fields:
  - { key: message, type: textarea, label: Message, required: true }
`

type Fixture struct {
	*testutil.Env
	Defs  *definitions.Store
	Subs  *submissions.Service
	Queue *jobs.Queue
}

func Setup(t testing.TB) *Fixture {
	t.Helper()
	env := testutil.NewEnv(t)
	wf, err := definition.ParseWorkflow([]byte(WorkflowYAML))
	if err != nil {
		t.Fatalf("fixture workflow: %v", err)
	}
	form, err := definition.ParseForm([]byte(FormYAML))
	if err != nil {
		t.Fatalf("fixture form: %v", err)
	}
	plain, err := definition.ParseForm([]byte(PlainFormYAML))
	if err != nil {
		t.Fatalf("fixture plain form: %v", err)
	}
	defs := definitions.NewStore(env.Pool)
	if _, err := defs.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms:     []definition.Form{form, plain},
		Workflows: []definition.Workflow{wf},
		Source:    definitions.SourceAPI,
		Actor:     "fixture",
	}); err != nil {
		t.Fatalf("fixture apply: %v", err)
	}
	return &Fixture{
		Env:   env,
		Defs:  defs,
		Subs:  submissions.NewService(env.Pool, defs),
		Queue: jobs.NewQueue(env.Pool),
	}
}

func (f *Fixture) Submit(t testing.TB, formSlug string, data map[string]any) submissions.Submission {
	t.Helper()
	sub, _, err := f.Subs.Create(context.Background(), f.OrgID, formSlug, data, false)
	if err != nil {
		t.Fatalf("submit %s: %v", formSlug, err)
	}
	return sub
}

// Applicant creates a submission on the "apply" form in state "new".
func (f *Fixture) Applicant(t testing.TB) submissions.Submission {
	return f.Submit(t, "apply", map[string]any{"name": "Ada", "email": "ada@example.com"})
}

func (f *Fixture) Principal(u auth.User) auth.Principal {
	return auth.Principal{OrgID: f.OrgID, Kind: auth.PrincipalUser, ID: u.ID, Name: u.Name, Email: u.Email, Roles: u.Roles}
}

// PrincipalWithRoles creates a new user with a unique email and returns its principal.
func (f *Fixture) PrincipalWithRoles(t testing.TB, roles ...string) auth.Principal {
	t.Helper()
	return f.Principal(f.User(t, uuid.NewString()+"@example.com", roles...))
}

// Jobs returns every job of the org, oldest first.
func (f *Fixture) Jobs(t testing.TB) []jobs.Job {
	t.Helper()
	all, err := f.Queue.List(context.Background(), f.OrgID, "", 1000)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all
}

func (f *Fixture) Events(t testing.TB, id uuid.UUID) []submissions.Event {
	t.Helper()
	evs, err := f.Subs.Events(context.Background(), f.OrgID, id)
	if err != nil {
		t.Fatalf("events: %v", err)
	}
	return evs
}

func (f *Fixture) Reload(t testing.TB, id uuid.UUID) submissions.Submission {
	t.Helper()
	sub, err := f.Subs.Get(context.Background(), f.OrgID, id)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	return sub
}

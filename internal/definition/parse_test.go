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
		{"semantic after schema", parseFormErr, "slug: a\ntitle: A\nfields:\n  - { key: x, type: text, label: X }\n  - { key: x, type: text, label: Second }\n", "fields[1].key", "duplicate"},
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

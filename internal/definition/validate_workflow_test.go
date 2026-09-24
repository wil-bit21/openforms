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

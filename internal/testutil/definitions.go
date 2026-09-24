package testutil

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
)

// SampleWorkflow returns a small valid workflow: new → review → done.
// "start" requires the reviewer role; "finish" requires the "note" field.
func SampleWorkflow(slug string) definition.Workflow {
	return definition.Workflow{
		Slug:    slug,
		Title:   "Review",
		Initial: "new",
		States: []definition.State{
			{Key: "new", Label: "New", Color: "gray"},
			{Key: "review", Label: "In review", Color: "blue"},
			{Key: "done", Label: "Done", Color: "green", Terminal: true},
		},
		Fields: []definition.WorkflowField{
			{Key: "note", Type: definition.FieldTextarea, Label: "Note"},
		},
		Transitions: []definition.Transition{
			{Key: "start", Label: "Start review", From: []string{"new"}, To: "review", Guard: definition.Guard{Roles: []string{"reviewer"}}},
			{Key: "finish", Label: "Finish", From: []string{"review"}, To: "done", Guard: definition.Guard{RequireFields: []string{"note"}}},
		},
	}
}

// SampleForm returns a valid public contact form. workflow may be "".
func SampleForm(slug, workflow string) definition.Form {
	return definition.Form{
		Slug:     slug,
		Title:    "Contact",
		Workflow: workflow,
		Settings: definition.FormSettings{Public: true},
		Fields: []definition.Field{
			{Key: "name", Type: definition.FieldText, Label: "Name", Required: true},
			{Key: "email", Type: definition.FieldEmail, Label: "Email", Required: true},
			{Key: "topic", Type: definition.FieldSelect, Label: "Topic", Options: []definition.Option{
				{Value: "sales", Label: "Sales"},
				{Value: "support", Label: "Support"},
			}},
		},
	}
}

// OtherOrg inserts a second organisation, for cross-org isolation tests.
func (e *Env) OtherOrg(t testing.TB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	if _, err := e.Pool.Exec(context.Background(), `INSERT INTO orgs (id, name) VALUES ($1, 'Other')`, id); err != nil {
		t.Fatalf("insert org: %v", err)
	}
	return id
}

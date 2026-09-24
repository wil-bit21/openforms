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
		Fields:   []definition.WorkflowField{{Key: "reason", Type: definition.FieldTextarea, Label: "Reason"}},
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

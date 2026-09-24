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

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

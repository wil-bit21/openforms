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

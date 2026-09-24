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
		"zeta":    1,
		"alpha":   "x",
		"score":   "4",
		"source":  "website",
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

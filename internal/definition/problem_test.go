package definition_test

import (
	"errors"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

func TestValidationErrorMessage(t *testing.T) {
	var err error = &definition.ValidationError{Problems: []definition.Problem{
		{Path: "email", Message: "must be a valid email address"},
		{Path: "password", Message: "must be at least 8 characters"},
	}}
	want := "validation failed: email: must be a valid email address; password: must be at least 8 characters"
	if err.Error() != want {
		t.Fatalf("Error() = %q\nwant %q", err.Error(), want)
	}
	var verr *definition.ValidationError
	if !errors.As(err, &verr) || len(verr.Problems) != 2 {
		t.Fatal("errors.As failed")
	}
	if (&definition.ValidationError{}).Error() != "validation failed" {
		t.Fatal("empty problems message")
	}
}

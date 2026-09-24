package testutil_test

import (
	"context"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/testutil"
)

func TestSampleDefinitionsAreValid(t *testing.T) {
	wf := testutil.SampleWorkflow("review")
	if err := definition.ValidateWorkflow(wf); err != nil {
		t.Fatalf("sample workflow invalid: %v", err)
	}
	f := testutil.SampleForm("contact", "review")
	if err := definition.ValidateForm(f); err != nil {
		t.Fatalf("sample form invalid: %v", err)
	}
	if err := definition.ValidateBundle([]definition.Form{f, testutil.SampleForm("plain", "")}, []definition.Workflow{wf}, nil); err != nil {
		t.Fatalf("sample bundle invalid: %v", err)
	}
}

func TestOtherOrgCreatesDistinctOrg(t *testing.T) {
	env := testutil.NewEnv(t)
	other := env.OtherOrg(t)
	if other == env.OrgID {
		t.Fatal("OtherOrg returned the default org id")
	}
	var n int
	if err := env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM orgs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("want 2 orgs, got %d", n)
	}
}

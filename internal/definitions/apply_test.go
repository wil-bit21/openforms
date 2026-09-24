package definitions_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func validationErr(t *testing.T, err error) *definition.ValidationError {
	t.Helper()
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v (%T), want *definition.ValidationError", err, err)
	}
	return ve
}

func TestApplyRepinsDependentForms(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", "")},
	})

	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	res := applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{wf}, Source: definitions.SourceUI})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 2, Changed: true},
		{Kind: "form", Slug: "contact", Version: 2, Changed: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}

	f, err := s.GetForm(ctx, env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	w, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatal(err)
	}
	if f.WorkflowVersionID == nil || *f.WorkflowVersionID != w.Current.ID {
		t.Fatalf("contact pinned to %v, want %v", f.WorkflowVersionID, w.Current.ID)
	}
	if f.Current.Source != definitions.SourceUI {
		t.Fatalf("re-pinned version source = %q, want ui", f.Current.Source)
	}
	p, err := s.GetForm(ctx, env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if p.Current.Version != 1 {
		t.Fatalf("unrelated form re-versioned to %d", p.Current.Version)
	}
}

func TestApplyRepinsFormInBundleWithUnchangedDefinition(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{wf},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 2, Changed: true},
		{Kind: "form", Slug: "contact", Version: 2, Changed: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
}

func TestApplyFormAgainstExistingWorkflow(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	res := applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("contact", "review")}})
	if len(res.Items) != 1 || !res.Items[0].Created {
		t.Fatalf("items = %+v", res.Items)
	}
}

func TestApplyDryRunPersistsNothing(t *testing.T) {
	env, s := newStore(t)
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
		DryRun:    true,
	})
	if len(res.Items) != 2 || !res.Items[0].Created || !res.Items[1].Created {
		t.Fatalf("dry-run items = %+v", res.Items)
	}
	if _, err := s.GetForm(context.Background(), env.OrgID, "contact"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("dry-run persisted form: err = %v", err)
	}
	if _, err := s.GetWorkflow(context.Background(), env.OrgID, "review"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("dry-run persisted workflow: err = %v", err)
	}
}

func TestApplyRejectsMissingWorkflow(t *testing.T) {
	env, s := newStore(t)
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{testutil.SampleForm("contact", "nope")},
	})
	validationErr(t, err)
	if _, err := s.GetForm(context.Background(), env.OrgID, "contact"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("invalid bundle persisted form: %v", err)
	}
}

func TestApplyRejectsDuplicateSlugs(t *testing.T) {
	env, s := newStore(t)
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Forms: []definition.Form{testutil.SampleForm("plain", ""), testutil.SampleForm("plain", "")},
	})
	ve := validationErr(t, err)
	if ve.Problems[0].Path != "forms[1].slug" {
		t.Fatalf("problem path = %q, want forms[1].slug", ve.Problems[0].Path)
	}
}

func TestApplyPrefixesDocumentProblems(t *testing.T) {
	env, s := newStore(t)
	wf := testutil.SampleWorkflow("review")
	wf.Initial = "missing"
	_, err := s.Apply(context.Background(), env.OrgID, definitions.ApplyInput{Workflows: []definition.Workflow{wf}})
	ve := validationErr(t, err)
	for _, p := range ve.Problems {
		if !strings.HasPrefix(p.Path, "workflows[0]") {
			t.Fatalf("problem path %q not prefixed with workflows[0]", p.Path)
		}
	}
}

func TestApplyConcurrentSameNewSlug(t *testing.T) {
	env, s := newStore(t)
	in := definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}}
	var wg sync.WaitGroup
	errs := make([]error, 4)
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, errs[i] = s.Apply(context.Background(), env.OrgID, in)
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("apply %d: %v", i, err)
		}
	}
	vs, err := s.FormVersions(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("want exactly 1 version, got %d", len(vs))
	}
}

func TestPrefixProblems(t *testing.T) {
	err := &definition.ValidationError{Problems: []definition.Problem{
		{Path: "fields[0].key", Message: "bad key"},
		{Path: "", Message: "whole document"},
		{Path: "[2]", Message: "indexed"},
	}}
	got := definitions.PrefixProblems("forms[3]", err)
	want := []definition.Problem{
		{Path: "forms[3].fields[0].key", Message: "bad key"},
		{Path: "forms[3]", Message: "whole document"},
		{Path: "forms[3][2]", Message: "indexed"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	if got := definitions.PrefixProblems("forms[0]", errors.New("boom")); len(got) != 1 || got[0].Path != "forms[0]" || got[0].Message != "boom" {
		t.Fatalf("plain error: %+v", got)
	}
	if got := definitions.PrefixProblems("forms[0]", nil); got != nil {
		t.Fatalf("nil error: %+v", got)
	}
}

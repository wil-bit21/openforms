package definitions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func newStore(t *testing.T) (*testutil.Env, *definitions.Store) {
	t.Helper()
	env := testutil.NewEnv(t)
	return env, definitions.NewStore(env.Pool)
}

func applyOK(t *testing.T, s *definitions.Store, env *testutil.Env, in definitions.ApplyInput) definitions.ApplyResult {
	t.Helper()
	res, err := s.Apply(context.Background(), env.OrgID, in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return res
}

func TestApplyCreatesVersionOne(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	res := applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
		Source:    definitions.SourceCLI,
		Actor:     "dev@example.com",
	})
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 1, Changed: true, Created: true},
		{Kind: "form", Slug: "contact", Version: 1, Changed: true, Created: true},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}

	f, err := s.GetForm(ctx, env.OrgID, "contact")
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}
	if f.Current.Version != 1 || f.Current.Source != definitions.SourceCLI || f.Current.CreatedBy != "dev@example.com" {
		t.Fatalf("unexpected version info %+v", f.Current)
	}
	if f.Definition.Title != "Contact" || len(f.Definition.Fields) != 3 {
		t.Fatalf("definition not round-tripped: %+v", f.Definition)
	}
	if len(f.Current.Hash) != 64 {
		t.Fatalf("hash should be sha256 hex, got %q", f.Current.Hash)
	}

	wf, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if f.WorkflowVersionID == nil || *f.WorkflowVersionID != wf.Current.ID {
		t.Fatalf("form pin = %v, want workflow version %v", f.WorkflowVersionID, wf.Current.ID)
	}
	if wf.Definition.Initial != "new" || len(wf.Definition.States) != 3 {
		t.Fatalf("workflow not round-tripped: %+v", wf.Definition)
	}
}

func TestApplyUnchangedIsNoop(t *testing.T) {
	env, s := newStore(t)
	in := definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	}
	applyOK(t, s, env, in)
	res := applyOK(t, s, env, in)
	want := []definitions.ApplyItem{
		{Kind: "workflow", Slug: "review", Version: 1},
		{Kind: "form", Slug: "contact", Version: 1},
	}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
}

func TestApplyChangedFormCreatesNewVersion(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	changed := testutil.SampleForm("plain", "")
	changed.Title = "Contact us"
	res := applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{changed}, Source: definitions.SourceUI})
	want := []definitions.ApplyItem{{Kind: "form", Slug: "plain", Version: 2, Changed: true}}
	if !reflect.DeepEqual(res.Items, want) {
		t.Fatalf("items = %+v, want %+v", res.Items, want)
	}
	f, err := s.GetForm(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Version != 2 || f.Definition.Title != "Contact us" || f.Current.Source != definitions.SourceUI {
		t.Fatalf("unexpected record %+v", f)
	}
	if f.WorkflowVersionID != nil {
		t.Fatalf("form without workflow must not be pinned, got %v", f.WorkflowVersionID)
	}
}

func TestApplyDefaultsSourceToAPI(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	f, err := s.GetForm(context.Background(), env.OrgID, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if f.Current.Source != definitions.SourceAPI {
		t.Fatalf("source = %q, want api", f.Current.Source)
	}
}

func TestGetUnknownReturnsNotFound(t *testing.T) {
	env, s := newStore(t)
	if _, err := s.GetForm(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("GetForm err = %v, want ErrNotFound", err)
	}
	if _, err := s.GetWorkflow(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("GetWorkflow err = %v, want ErrNotFound", err)
	}
}

func TestParseSource(t *testing.T) {
	for _, s := range []string{"cli", "ui", "api", "seed"} {
		if got, ok := definitions.ParseSource(s); !ok || string(got) != s {
			t.Fatalf("ParseSource(%q) = %q, %v", s, got, ok)
		}
	}
	if _, ok := definitions.ParseSource("git"); ok {
		t.Fatal("ParseSource(git) should fail")
	}
}

package definitions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/testutil"
)

func seedTwoVersions(t *testing.T) (*testutil.Env, *definitions.Store) {
	t.Helper()
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review")},
	})
	f := testutil.SampleForm("contact", "review")
	f.Title = "Contact v2"
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{f}, Source: definitions.SourceUI, Actor: "ui@example.com"})
	return env, s
}

func TestFormVersionsNewestFirst(t *testing.T) {
	env, s := seedTwoVersions(t)
	vs, err := s.FormVersions(context.Background(), env.OrgID, "contact")
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0].Version != 2 || vs[1].Version != 1 {
		t.Fatalf("versions = %+v", vs)
	}
	if vs[0].Source != definitions.SourceUI || vs[0].CreatedBy != "ui@example.com" {
		t.Fatalf("newest version info = %+v", vs[0])
	}
	if _, err := s.FormVersions(context.Background(), env.OrgID, "missing"); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("missing form err = %v", err)
	}
}

func TestFormVersionReturnsHistoricDefinition(t *testing.T) {
	env, s := seedTwoVersions(t)
	ctx := context.Background()
	v1, err := s.FormVersion(ctx, env.OrgID, "contact", 1)
	if err != nil {
		t.Fatal(err)
	}
	if v1.Definition.Title != "Contact" || v1.Current.Version != 1 {
		t.Fatalf("v1 = %+v", v1)
	}
	if _, err := s.FormVersion(ctx, env.OrgID, "contact", 9); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("v9 err = %v", err)
	}
	byID, err := s.GetFormVersion(ctx, v1.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if byID.Definition.Title != "Contact" || byID.Slug != "contact" {
		t.Fatalf("GetFormVersion = %+v", byID)
	}
}

func TestWorkflowVersionsAndLookups(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	wf := testutil.SampleWorkflow("review")
	wf.Title = "Review v2"
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{wf}})

	vs, err := s.WorkflowVersions(ctx, env.OrgID, "review")
	if err != nil || len(vs) != 2 || vs[0].Version != 2 {
		t.Fatalf("versions = %+v, err = %v", vs, err)
	}
	v1, err := s.WorkflowVersion(ctx, env.OrgID, "review", 1)
	if err != nil || v1.Definition.Title != "Review" {
		t.Fatalf("v1 = %+v, err = %v", v1, err)
	}
	if _, err := s.WorkflowVersion(ctx, env.OrgID, "review", 3); !errors.Is(err, definitions.ErrNotFound) {
		t.Fatalf("v3 err = %v", err)
	}
}

func TestGetWorkflowVersionIsCached(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{Workflows: []definition.Workflow{testutil.SampleWorkflow("review")}})
	cur, err := s.GetWorkflow(ctx, env.OrgID, "review")
	if err != nil {
		t.Fatal(err)
	}
	first, err := s.GetWorkflowVersion(ctx, cur.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Versions are immutable; tamper with the row to prove the second read is served from cache.
	if _, err := env.Pool.Exec(ctx, `UPDATE workflow_versions SET definition = jsonb_set(definition, '{title}', '"tampered"') WHERE id = $1`, cur.Current.ID); err != nil {
		t.Fatal(err)
	}
	second, err := s.GetWorkflowVersion(ctx, cur.Current.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.Definition.Title != "Review" || second.Definition.Title != "Review" {
		t.Fatalf("cache miss: first %q second %q", first.Definition.Title, second.Definition.Title)
	}
}

func TestListAndExportSortedBySlug(t *testing.T) {
	env, s := newStore(t)
	ctx := context.Background()
	applyOK(t, s, env, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("zeta"), testutil.SampleWorkflow("alpha")},
		Forms:     []definition.Form{testutil.SampleForm("b-form", "zeta"), testutil.SampleForm("a-form", "")},
	})
	forms, err := s.ListForms(ctx, env.OrgID)
	if err != nil || len(forms) != 2 || forms[0].Slug != "a-form" || forms[1].Slug != "b-form" {
		t.Fatalf("ListForms = %+v, err = %v", forms, err)
	}
	wfs, err := s.ListWorkflows(ctx, env.OrgID)
	if err != nil || len(wfs) != 2 || wfs[0].Slug != "alpha" {
		t.Fatalf("ListWorkflows = %+v, err = %v", wfs, err)
	}
	ef, ew, err := s.Export(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ef) != 2 || ef[0].Slug != "a-form" || len(ew) != 2 || ew[1].Slug != "zeta" {
		t.Fatalf("Export forms=%+v workflows=%+v", ef, ew)
	}
}

func TestExportEmptyOrgReturnsEmptySlices(t *testing.T) {
	env, s := newStore(t)
	ef, ew, err := s.Export(context.Background(), env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if ef == nil || ew == nil || len(ef) != 0 || len(ew) != 0 {
		t.Fatalf("want empty non-nil slices, got %v %v", ef, ew)
	}
}

func TestListIsOrgScoped(t *testing.T) {
	env, s := newStore(t)
	applyOK(t, s, env, definitions.ApplyInput{Forms: []definition.Form{testutil.SampleForm("plain", "")}})
	other := env.OtherOrg(t)
	forms, err := s.ListForms(context.Background(), other)
	if err != nil || len(forms) != 0 {
		t.Fatalf("other org sees %d forms, err = %v", len(forms), err)
	}
}

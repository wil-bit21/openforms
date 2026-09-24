package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

const contactNoWF = "slug: contact\ntitle: Contact\nsettings:\n  public: true\nfields:\n  - {key: name, type: text, label: Name}\n"

func TestPushAppliesLocalBundleAndPrintsTable(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyRes: client.ApplyResult{Items: []client.ApplyItem{
		{Kind: "form", Slug: "contact", Version: 1, Changed: true, Created: true},
	}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPushCmd(d)); err != nil {
		t.Fatalf("push: %v\n%s", err, tio.err.String())
	}
	if len(fr.applied) != 1 || fr.dryRuns[0] {
		t.Fatalf("applied=%d dryRuns=%v", len(fr.applied), fr.dryRuns)
	}
	if got := fr.applied[0].Forms[0].Slug; got != "contact" {
		t.Fatalf("sent slug %q", got)
	}
	want := "KIND  SLUG     VERSION  STATUS\nform  contact  1        created\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPushDryRun(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyRes: client.ApplyResult{Items: []client.ApplyItem{{Kind: "form", Slug: "contact", Version: 3, Changed: true}}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPushCmd(d), "--dry-run"); err != nil {
		t.Fatal(err)
	}
	if !fr.dryRuns[0] {
		t.Fatal("dry-run flag not forwarded")
	}
	if !strings.HasPrefix(tio.out.String(), "Dry run: nothing was applied.\n") || !strings.Contains(tio.out.String(), "updated") {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestPushStopsOnLocalProblems(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/wrong.yaml", contactNoWF)
	fr := &fakeRemote{}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPushCmd(d))
	if !errors.Is(err, ErrProblems) || len(fr.applied) != 0 {
		t.Fatalf("err=%v applied=%d", err, len(fr.applied))
	}
	if !strings.Contains(tio.err.String(), "forms/wrong.yaml:slug:") {
		t.Fatalf("stderr = %q", tio.err.String())
	}
}

func TestPushPrintsServerValidationProblems(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	fr := &fakeRemote{applyErr: &client.Error{Status: 422, Code: "validation_failed", Message: "invalid",
		Details: []definition.Problem{{Path: "forms[0].fields", Message: "must have at least one field"}}}}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPushCmd(d))
	if !errors.Is(err, ErrProblems) || !strings.Contains(tio.err.String(), "server:forms[0].fields: must have at least one field") {
		t.Fatalf("err=%v stderr=%q", err, tio.err.String())
	}
}

// Review Focus #3: unreachable server gives a readable error.
func TestPushUnreachableServer(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	srv := httptest.NewServer(http.NotFoundHandler())
	base := srv.URL
	srv.Close()
	d, _ := newTestDeps(wd, map[string]string{"OPENFORMS_URL": base, "OPENFORMS_API_KEY": "ofk_x"}, nil)
	d.NewClient = func(server, key string) Remote { return client.New(server, key) }
	err := execute(NewPushCmd(d))
	if err == nil || !strings.Contains(err.Error(), "cannot reach openforms server at "+base) {
		t.Fatalf("err = %v", err)
	}
}

func TestApplyStatus(t *testing.T) {
	cases := map[string]client.ApplyItem{
		"created":   {Created: true, Changed: true},
		"updated":   {Changed: true},
		"unchanged": {},
	}
	for want, it := range cases {
		if got := applyStatus(it); got != want {
			t.Fatalf("applyStatus(%+v) = %s, want %s", it, got, want)
		}
	}
}

func TestDiffBundles(t *testing.T) {
	a := definition.Form{Slug: "a", Title: "A", Fields: []definition.Field{}}
	aChanged := a
	aChanged.Title = "A2"
	b := definition.Form{Slug: "b", Title: "B", Fields: []definition.Field{}}
	c := definition.Form{Slug: "c", Title: "C", Fields: []definition.Field{}}
	w := definition.Workflow{Slug: "w", Title: "W", Initial: "s", States: []definition.State{{Key: "s", Label: "S"}}}

	local := client.Bundle{Forms: []definition.Form{aChanged, b}, Workflows: []definition.Workflow{w}}
	remote := client.Bundle{Forms: []definition.Form{a, b, c}, Workflows: []definition.Workflow{}}
	got, err := diffBundles(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"~ form a", "- form c (remote only)", "+ workflow w (local only)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDiffCommand(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", contactNoWF)
	local, _, _ := LoadLocal(wd + "/openforms")
	fr := &fakeRemote{export: local.Bundle}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewDiffCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "No differences.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}

	fr.export = client.Bundle{Forms: []definition.Form{}, Workflows: []definition.Workflow{}}
	tio.out.Reset()
	if err := execute(NewDiffCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "+ form contact (local only)\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

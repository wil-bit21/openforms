package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func remoteBundle() client.Bundle {
	return client.Bundle{
		Forms: []definition.Form{{Slug: "contact", Title: "Contact", Settings: definition.FormSettings{Public: true},
			Fields: []definition.Field{{Key: "email", Type: definition.FieldEmail, Label: "Email", Required: true}}}},
		Workflows: []definition.Workflow{{Slug: "triage", Title: "Triage", Initial: "new",
			States: []definition.State{{Key: "new", Label: "New"}}, Transitions: []definition.Transition{}}},
	}
}

func TestPullWritesCanonicalFiles(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatalf("pull: %v", err)
	}
	wantForm, _ := CanonicalYAML(remoteBundle().Forms[0])
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); got != string(wantForm) {
		t.Fatalf("form file:\n%s\nwant:\n%s", got, wantForm)
	}
	readRel(t, wd, "openforms/workflows/triage.yaml")
	want := "created forms/contact.yaml\ncreated workflows/triage.yaml\nPulled 1 form(s), 1 workflow(s): 2 created, 0 updated, 0 unchanged.\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPullIsStableAndSkipsUnchanged(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(wd, "openforms", "forms", "contact.yaml")
	before, _ := os.Stat(p)
	tio.out.Reset()
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(p)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("unchanged file was rewritten")
	}
	if tio.out.String() != "Pulled 1 form(s), 1 workflow(s): 0 created, 0 updated, 2 unchanged.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestPullCheckDetectsDriftWithoutWriting(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", "slug: contact\ntitle: Old title\nsettings:\n  public: true\nfields:\n  - {key: name, type: text, label: Name}\n")
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPullCmd(d), "--check")
	if !errors.Is(err, ErrDrift) {
		t.Fatalf("err = %v", err)
	}
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); !strings.Contains(got, "Old title") {
		t.Fatal("--check must not write")
	}
	if _, err := os.Stat(filepath.Join(wd, "openforms", "workflows", "triage.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("--check must not create files")
	}
	want := "would update forms/contact.yaml\nwould create workflows/triage.yaml\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestPullCheckUpToDate(t *testing.T) {
	wd := t.TempDir()
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	tio.out.Reset()
	if err := execute(NewPullCmd(d), "--check"); err != nil {
		t.Fatalf("err = %v", err)
	}
	if tio.out.String() != "Up to date.\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

// Review Focus #5: local-only files survive a pull.
func TestPullKeepsLocalOnlyFiles(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/draft.yaml", "slug: draft\n")
	fr := &fakeRemote{export: remoteBundle()}
	d, tio := newTestDeps(wd, remoteEnv, fr)
	if err := execute(NewPullCmd(d)); err != nil {
		t.Fatal(err)
	}
	if readRel(t, wd, "openforms/forms/draft.yaml") != "slug: draft\n" {
		t.Fatal("local-only file modified")
	}
	if !strings.Contains(tio.out.String(), "note: forms/draft.yaml exists locally but not on the server (left untouched)") {
		t.Fatalf("out = %q", tio.out.String())
	}
}

// Review Focus #5: alternate extensions block the pull instead of creating a duplicate slug.
func TestPullRefusesAlternateExtension(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.json", "{}")
	fr := &fakeRemote{export: remoteBundle()}
	d, _ := newTestDeps(wd, remoteEnv, fr)
	err := execute(NewPullCmd(d))
	if err == nil || !strings.Contains(err.Error(), "forms/contact.json exists; pull only manages .yaml files, rename it to forms/contact.yaml") {
		t.Fatalf("err = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(wd, "openforms", "forms", "contact.yaml")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatal("pull wrote files despite the conflict")
	}
}

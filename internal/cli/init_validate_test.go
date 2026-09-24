package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/definition"
)

var scaffolded = []string{"openforms.yaml", "openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"}

func TestInitScaffoldsProject(t *testing.T) {
	wd := t.TempDir()
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	for _, rel := range scaffolded {
		if _, err := os.Stat(filepath.Join(wd, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
	if got := readRel(t, wd, "openforms.yaml"); got != "server: http://localhost:8080\ndir: openforms\n" {
		t.Fatalf("openforms.yaml = %q", got)
	}
	if !strings.Contains(tio.out.String(), "openforms push") {
		t.Fatalf("expected next steps in output:\n%s", tio.out.String())
	}
}

func TestInitIntoSubdirectory(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d), "myproject"); err != nil {
		t.Fatal(err)
	}
	readRel(t, wd, "myproject/openforms/forms/contact.yaml")
}

func TestInitRefusesToOverwrite(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms/forms/contact.yaml", "mine")
	d, _ := newTestDeps(wd, nil, nil)
	err := execute(NewInitCmd(d))
	if err == nil || !strings.Contains(err.Error(), "openforms/forms/contact.yaml already exists") {
		t.Fatalf("err = %v", err)
	}
	if got := readRel(t, wd, "openforms/forms/contact.yaml"); got != "mine" {
		t.Fatalf("file was overwritten: %q", got)
	}
	if _, err := os.Stat(filepath.Join(wd, "openforms.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("init must write nothing when any target exists, stat err = %v", err)
	}
}

func TestInitWritesCanonicalYAML(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	formSrc := readRel(t, wd, "openforms/forms/contact.yaml")
	f, err := definition.ParseForm([]byte(formSrc))
	if err != nil {
		t.Fatal(err)
	}
	again, _ := CanonicalYAML(f)
	if string(again) != formSrc {
		t.Fatalf("scaffolded form is not canonical:\n%s\n---\n%s", formSrc, again)
	}
	wfSrc := readRel(t, wd, "openforms/workflows/contact-triage.yaml")
	w, err := definition.ParseWorkflow([]byte(wfSrc))
	if err != nil {
		t.Fatal(err)
	}
	again, _ = CanonicalYAML(w)
	if string(again) != wfSrc {
		t.Fatalf("scaffolded workflow is not canonical:\n%s\n---\n%s", wfSrc, again)
	}
}

func TestValidateScaffoldedProject(t *testing.T) {
	wd := t.TempDir()
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewInitCmd(d)); err != nil {
		t.Fatal(err)
	}
	tio.out.Reset()
	if err := execute(NewValidateCmd(d)); err != nil {
		t.Fatalf("validate: %v\n%s", err, tio.err.String())
	}
	if tio.out.String() != "OK: 1 form(s), 1 workflow(s) valid\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

func TestValidateReportsProblemsAndFails(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "defs/forms/feedback.yaml", "slug: contact2\ntitle: Feedback\nsettings: {public: true}\nfields:\n  - {key: name, type: text, label: Name}\n")
	d, tio := newTestDeps(wd, nil, nil)
	err := execute(NewValidateCmd(d), "--dir", "defs")
	if !errors.Is(err, ErrProblems) {
		t.Fatalf("err = %v", err)
	}
	want := "forms/feedback.yaml:slug: slug \"contact2\" must match file name \"feedback\"\n1 problem(s) found\n"
	if tio.err.String() != want {
		t.Fatalf("stderr = %q, want %q", tio.err.String(), want)
	}
}

func TestValidateUsesDirFromProjectConfig(t *testing.T) {
	wd := t.TempDir()
	put(t, wd, "openforms.yaml", "dir: defs\n")
	put(t, wd, "defs/forms/contact.yaml", "slug: contact\ntitle: Contact\nsettings: {public: true}\nfields:\n  - {key: name, type: text, label: Name}\n")
	d, tio := newTestDeps(wd, nil, nil)
	if err := execute(NewValidateCmd(d)); err != nil {
		t.Fatal(err)
	}
	if tio.out.String() != "OK: 1 form(s), 0 workflow(s) valid\n" {
		t.Fatalf("out = %q", tio.out.String())
	}
}

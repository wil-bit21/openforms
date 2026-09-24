package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validFormYAML = `slug: contact
title: Contact
workflow: triage
settings:
  public: true
fields:
  - key: email
    type: email
    label: Email
    required: true
`

const validWorkflowYAML = `slug: triage
title: Triage
initial: new
states:
  - key: new
    label: New
  - key: done
    label: Done
    terminal: true
transitions:
  - key: finish
    label: Finish
    from: [new]
    to: done
    guard: {}
`

func put(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadLocalValid(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/contact.yaml", validFormYAML)
	put(t, dir, "workflows/triage.yml", validWorkflowYAML)
	put(t, dir, "forms/README.md", "not a definition")

	lb, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if len(lb.Bundle.Forms) != 1 || lb.Bundle.Forms[0].Slug != "contact" {
		t.Fatalf("forms = %+v", lb.Bundle.Forms)
	}
	if len(lb.Bundle.Workflows) != 1 || lb.Bundle.Workflows[0].Slug != "triage" {
		t.Fatalf("workflows = %+v", lb.Bundle.Workflows)
	}
	if lb.FormFiles["contact"] != "forms/contact.yaml" || lb.WorkflowFiles["triage"] != "workflows/triage.yml" {
		t.Fatalf("files = %v %v", lb.FormFiles, lb.WorkflowFiles)
	}
}

func TestLoadLocalMissingDirsIsEmpty(t *testing.T) {
	lb, problems, err := LoadLocal(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(problems) != 0 {
		t.Fatalf("err=%v problems=%v", err, problems)
	}
	if lb.Bundle.Forms == nil || lb.Bundle.Workflows == nil || len(lb.Bundle.Forms)+len(lb.Bundle.Workflows) != 0 {
		t.Fatalf("bundle = %+v", lb.Bundle)
	}
}

func TestLoadLocalReportsSchemaProblemsPerFile(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/broken.yaml", "slug: broken\ntitle: Broken\nsettings: {public: true}\nfields:\n  - key: a\n    type: colour\n    label: A\n")
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) == 0 {
		t.Fatal("expected problems")
	}
	for _, p := range problems {
		if p.File != "forms/broken.yaml" {
			t.Fatalf("problem attributed to %q: %v", p.File, p)
		}
	}
}

func TestLoadLocalSlugMustMatchFileName(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/feedback.yaml", strings.Replace(validFormYAML, "workflow: triage\n", "", 1))
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := `forms/feedback.yaml:slug: slug "contact" must match file name "feedback"`
	if len(problems) != 1 || problems[0].String() != want {
		t.Fatalf("problems = %v, want [%s]", problems, want)
	}
}

func TestLoadLocalDuplicateSlugAcrossExtensions(t *testing.T) {
	dir := t.TempDir()
	noWF := strings.Replace(validFormYAML, "workflow: triage\n", "", 1)
	put(t, dir, "forms/contact.yaml", noWF)
	put(t, dir, "forms/contact.json", `{"slug":"contact","title":"Contact","settings":{"public":true},"fields":[{"key":"name","type":"text","label":"Name"}]}`)
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0].Message, "duplicate form slug") {
		t.Fatalf("problems = %v", problems)
	}
}

func TestLoadLocalUnknownWorkflowReference(t *testing.T) {
	dir := t.TempDir()
	put(t, dir, "forms/contact.yaml", validFormYAML) // references workflow "triage", which is absent
	_, problems, err := LoadLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := `forms/contact.yaml:workflow: unknown workflow "triage" (not found in workflows/)`
	if len(problems) != 1 || problems[0].String() != want {
		t.Fatalf("problems = %v, want [%s]", problems, want)
	}
}

// Review Focus #1: files saved on Windows.
func TestLoadLocalAcceptsCRLFAndBOM(t *testing.T) {
	dir := t.TempDir()
	crlf := strings.ReplaceAll(validFormYAML, "\n", "\r\n")
	put(t, dir, "forms/contact.yaml", "\xef\xbb\xbf"+crlf)
	put(t, dir, "workflows/triage.yaml", strings.ReplaceAll(validWorkflowYAML, "\n", "\r\n"))
	lb, problems, err := LoadLocal(dir)
	if err != nil || len(problems) != 0 {
		t.Fatalf("err=%v problems=%v", err, problems)
	}
	if lb.Bundle.Forms[0].Fields[0].Key != "email" {
		t.Fatalf("form = %+v", lb.Bundle.Forms[0])
	}
}

func TestPrintProblems(t *testing.T) {
	var buf bytes.Buffer
	printProblems(&buf, []Problem{
		{File: "forms/a.yaml", Path: "fields[0].type", Message: "bad type"},
		{File: "forms/b.yaml", Message: "yaml: line 2: did not find expected key"},
	})
	want := "forms/a.yaml:fields[0].type: bad type\nforms/b.yaml: yaml: line 2: did not find expected key\n2 problem(s) found\n"
	if buf.String() != want {
		t.Fatalf("got %q", buf.String())
	}
}

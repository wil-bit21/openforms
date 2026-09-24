package examples_test

import (
	"io/fs"
	"path"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/openforms/openforms/examples"
	"github.com/openforms/openforms/internal/definition"
)

func TestBundleIsValid(t *testing.T) {
	forms, workflows, err := examples.Bundle()
	if err != nil {
		t.Fatalf("Bundle: %v", err)
	}
	if err := definition.ValidateBundle(forms, workflows, nil); err != nil {
		t.Fatalf("ValidateBundle: %v", err)
	}
	var formSlugs, workflowSlugs []string
	for _, f := range forms {
		formSlugs = append(formSlugs, f.Slug)
	}
	for _, w := range workflows {
		workflowSlugs = append(workflowSlugs, w.Slug)
	}
	if want := []string{"contact", "job-application"}; !reflect.DeepEqual(formSlugs, want) {
		t.Errorf("form slugs = %v, want %v", formSlugs, want)
	}
	if want := []string{"contact-triage", "hiring"}; !reflect.DeepEqual(workflowSlugs, want) {
		t.Errorf("workflow slugs = %v, want %v", workflowSlugs, want)
	}
}

func TestSlugsMatchFileNames(t *testing.T) {
	for _, dir := range []string{"openforms/forms", "openforms/workflows"} {
		entries, err := fs.ReadDir(examples.FS, dir)
		if err != nil {
			t.Fatalf("ReadDir %s: %v", dir, err)
		}
		for _, e := range entries {
			raw, err := fs.ReadFile(examples.FS, path.Join(dir, e.Name()))
			if err != nil {
				t.Fatal(err)
			}
			base := strings.TrimSuffix(e.Name(), ".yaml")
			var slug string
			if strings.HasSuffix(dir, "forms") {
				f, err := definition.ParseForm(raw)
				if err != nil {
					t.Fatalf("%s: %v", e.Name(), err)
				}
				slug = f.Slug
			} else {
				w, err := definition.ParseWorkflow(raw)
				if err != nil {
					t.Fatalf("%s: %v", e.Name(), err)
				}
				slug = w.Slug
			}
			if slug != base {
				t.Errorf("%s/%s: slug %q does not match file name", dir, e.Name(), slug)
			}
		}
	}
}

func findForm(t *testing.T, slug string) definition.Form {
	t.Helper()
	forms, _, err := examples.Bundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range forms {
		if f.Slug == slug {
			return f
		}
	}
	t.Fatalf("form %q not found", slug)
	return definition.Form{}
}

func findWorkflow(t *testing.T, slug string) definition.Workflow {
	t.Helper()
	_, workflows, err := examples.Bundle()
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range workflows {
		if w.Slug == slug {
			return w
		}
	}
	t.Fatalf("workflow %q not found", slug)
	return definition.Workflow{}
}

func TestJobApplicationShape(t *testing.T) {
	f := findForm(t, "job-application")
	var keys []string
	for _, fld := range f.Fields {
		keys = append(keys, fld.Key)
	}
	want := []string{"name", "email", "role", "years", "portfolio", "coverLetter", "consent"}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("field keys = %v, want %v", keys, want)
	}
	if f.Workflow != "hiring" || !f.Settings.Public {
		t.Errorf("workflow=%q public=%v, want hiring/true", f.Workflow, f.Settings.Public)
	}
	portfolio := f.Fields[4]
	if portfolio.ShowIf == nil || portfolio.ShowIf.Field != "role" || portfolio.ShowIf.Equals != "designer" {
		t.Errorf("portfolio.showIf = %+v, want role == designer", portfolio.ShowIf)
	}
	if f.Fields[6].Type != definition.FieldCheckbox || !f.Fields[6].Required {
		t.Errorf("consent must be a required checkbox, got %+v", f.Fields[6])
	}
}

func TestHiringWorkflowShape(t *testing.T) {
	w := findWorkflow(t, "hiring")
	var states, transitions []string
	for _, s := range w.States {
		states = append(states, s.Key)
	}
	for _, tr := range w.Transitions {
		transitions = append(transitions, tr.Key)
	}
	if want := []string{"new", "screening", "interview", "hired", "rejected"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	if want := []string{"screen", "invite", "hire", "reject"}; !reflect.DeepEqual(transitions, want) {
		t.Errorf("transitions = %v, want %v", transitions, want)
	}
	reject, _ := w.Transition("reject")
	if !reflect.DeepEqual(reject.Guard.RequireFields, []string{"rejectionReason"}) {
		t.Errorf("reject.requireFields = %v", reject.Guard.RequireFields)
	}
	hire, _ := w.Transition("hire")
	if !reflect.DeepEqual(hire.Guard.Roles, []string{"hiring-manager"}) {
		t.Errorf("hire.roles = %v", hire.Guard.Roles)
	}
	if len(w.OnSubmit) != 2 {
		t.Errorf("onSubmit actions = %d, want 2", len(w.OnSubmit))
	}
	for _, tr := range w.Transitions {
		for _, a := range tr.Actions {
			if a.Type == definition.ActionWebhook {
				t.Errorf("transition %q has a webhook; demo bundle must not call external URLs", tr.Key)
			}
		}
	}
}

func TestContactTriageShape(t *testing.T) {
	w := findWorkflow(t, "contact-triage")
	var states []string
	for _, s := range w.States {
		states = append(states, s.Key)
	}
	sort.Strings(states)
	if want := []string{"in_progress", "open", "resolved", "spam"}; !reflect.DeepEqual(states, want) {
		t.Errorf("states = %v, want %v", states, want)
	}
	f := findForm(t, "contact")
	if f.Workflow != "contact-triage" {
		t.Errorf("contact.workflow = %q", f.Workflow)
	}
}

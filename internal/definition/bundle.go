package definition

import "fmt"

// ValidateBundle validates every definition in a bundle, rejects duplicate
// slugs, and checks that each form's workflow resolves to a workflow in the
// bundle or in existingWorkflowSlugs. Problem paths are prefixed with
// "forms[i]" / "workflows[i]".
func ValidateBundle(forms []Form, workflows []Workflow, existingWorkflowSlugs []string) error {
	var ps problems
	known := make(map[string]bool, len(existingWorkflowSlugs)+len(workflows))
	for _, s := range existingWorkflowSlugs {
		known[s] = true
	}

	seenW := make(map[string]bool, len(workflows))
	for i, w := range workflows {
		p := fmt.Sprintf("workflows[%d]", i)
		ps.merge(p, ValidateWorkflow(w))
		if seenW[w.Slug] {
			ps.add(p+".slug", fmt.Sprintf("duplicate workflow slug %q", w.Slug))
		}
		seenW[w.Slug] = true
		known[w.Slug] = true
	}

	seenF := make(map[string]bool, len(forms))
	for i, f := range forms {
		p := fmt.Sprintf("forms[%d]", i)
		ps.merge(p, ValidateForm(f))
		if seenF[f.Slug] {
			ps.add(p+".slug", fmt.Sprintf("duplicate form slug %q", f.Slug))
		}
		seenF[f.Slug] = true
		if f.Workflow != "" && !known[f.Workflow] {
			ps.add(p+".workflow", fmt.Sprintf("unknown workflow %q", f.Workflow))
		}
	}
	return ps.err()
}

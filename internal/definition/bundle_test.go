package definition

import "testing"

func TestValidateBundleAcceptsConsistentBundle(t *testing.T) {
	if err := ValidateBundle([]Form{baseForm()}, []Workflow{baseWorkflow()}, nil); err != nil {
		t.Fatalf("bundle should be valid: %v", err)
	}
}

func TestValidateBundleResolvesExistingWorkflows(t *testing.T) {
	if err := ValidateBundle([]Form{baseForm()}, nil, []string{"hiring"}); err != nil {
		t.Fatalf("existing workflow slug should resolve: %v", err)
	}
}

func TestValidateBundleProblems(t *testing.T) {
	badWorkflow := baseWorkflow()
	badWorkflow.Initial = "draft"
	dupForm := baseForm()
	orphan := baseForm()
	orphan.Slug = "orphan"
	orphan.Workflow = "missing"

	err := ValidateBundle(
		[]Form{baseForm(), dupForm, orphan},
		[]Workflow{baseWorkflow(), baseWorkflow(), badWorkflow},
		nil,
	)
	paths := problemPaths(t, err)
	for _, want := range []string{
		"forms[1].slug",        // duplicate form slug
		"forms[2].workflow",    // unknown workflow
		"workflows[1].slug",    // duplicate workflow slug
		"workflows[2].slug",    // duplicate workflow slug
		"workflows[2].initial", // nested semantic problem, prefixed
	} {
		if !hasPath(paths, want) {
			t.Errorf("missing %q in %v", want, paths)
		}
	}
}

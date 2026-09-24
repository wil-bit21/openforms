package definitions

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
)

// PrefixProblems converts err into problems whose paths are prefixed with
// prefix (e.g. "forms[2]"). A non-validation error becomes one problem at prefix.
func PrefixProblems(prefix string, err error) []definition.Problem {
	if err == nil {
		return nil
	}
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		return []definition.Problem{{Path: prefix, Message: err.Error()}}
	}
	out := make([]definition.Problem, 0, len(ve.Problems))
	for _, p := range ve.Problems {
		path := prefix
		switch {
		case p.Path == "":
		case strings.HasPrefix(p.Path, "["):
			path += p.Path
		default:
			path += "." + p.Path
		}
		out = append(out, definition.Problem{Path: path, Message: p.Message})
	}
	return out
}

func validateInput(ctx context.Context, q db.DBTX, orgID uuid.UUID, in ApplyInput) error {
	var problems []definition.Problem
	seenWF := map[string]bool{}
	for i, wf := range in.Workflows {
		prefix := fmt.Sprintf("workflows[%d]", i)
		if seenWF[wf.Slug] {
			problems = append(problems, definition.Problem{Path: prefix + ".slug", Message: fmt.Sprintf("workflow %q appears more than once in the bundle", wf.Slug)})
		}
		seenWF[wf.Slug] = true
		problems = append(problems, PrefixProblems(prefix, definition.ValidateWorkflow(wf))...)
	}
	seenForm := map[string]bool{}
	for i, f := range in.Forms {
		prefix := fmt.Sprintf("forms[%d]", i)
		if seenForm[f.Slug] {
			problems = append(problems, definition.Problem{Path: prefix + ".slug", Message: fmt.Sprintf("form %q appears more than once in the bundle", f.Slug)})
		}
		seenForm[f.Slug] = true
		problems = append(problems, PrefixProblems(prefix, definition.ValidateForm(f))...)
	}
	if len(problems) > 0 {
		return &definition.ValidationError{Problems: problems}
	}
	existing, err := workflowSlugs(ctx, q, orgID)
	if err != nil {
		return err
	}
	return definition.ValidateBundle(in.Forms, in.Workflows, existing)
}

func workflowSlugs(ctx context.Context, q db.DBTX, orgID uuid.UUID) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT slug FROM workflows WHERE org_id = $1 AND current_version_id IS NOT NULL ORDER BY slug`, orgID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

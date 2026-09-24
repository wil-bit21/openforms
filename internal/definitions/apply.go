package definitions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

// Apply writes a bundle of definitions in one transaction:
//  1. validate every document and the bundle (cross-refs may resolve to existing workflows);
//  2. upsert workflows, then forms (each form pins its workflow's current version);
//  3. re-pin forms outside the bundle whose workflow just got a new version.
//
// Unchanged definitions create no version. DryRun reports the would-be result
// and rolls back.
func (s *Store) Apply(ctx context.Context, orgID uuid.UUID, in ApplyInput) (ApplyResult, error) {
	if in.Source == "" {
		in.Source = SourceAPI
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return ApplyResult{}, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after commit

	if err := validateInput(ctx, tx, orgID, in); err != nil {
		return ApplyResult{}, err
	}

	res := ApplyResult{Items: []ApplyItem{}}
	newWorkflowVersions := map[string]uuid.UUID{}
	for _, wf := range in.Workflows {
		item, vid, err := upsertWorkflow(ctx, tx, orgID, wf, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
		if item.Changed {
			newWorkflowVersions[wf.Slug] = vid
		}
	}

	inBundle := map[string]bool{}
	for _, f := range in.Forms {
		inBundle[f.Slug] = true
		pin, err := currentWorkflowVersion(ctx, tx, orgID, f.Workflow)
		if err != nil {
			return ApplyResult{}, err
		}
		item, err := upsertForm(ctx, tx, orgID, f, pin, in.Source, in.Actor)
		if err != nil {
			return ApplyResult{}, err
		}
		res.Items = append(res.Items, item)
	}

	for _, wf := range in.Workflows {
		vid, ok := newWorkflowVersions[wf.Slug]
		if !ok {
			continue
		}
		deps, err := dependentForms(ctx, tx, orgID, wf.Slug)
		if err != nil {
			return ApplyResult{}, err
		}
		for _, f := range deps {
			if inBundle[f.Slug] {
				continue
			}
			pin := vid
			item, err := upsertForm(ctx, tx, orgID, f, &pin, in.Source, in.Actor)
			if err != nil {
				return ApplyResult{}, err
			}
			if item.Changed {
				res.Items = append(res.Items, item)
			}
		}
	}

	if in.DryRun {
		return res, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return ApplyResult{}, err
	}
	return res, nil
}

// upsertWorkflow returns the item and the id of the workflow's current version
// after the write. INSERT ... ON CONFLICT DO NOTHING followed by SELECT ... FOR
// UPDATE makes concurrent applies of a new slug serialize instead of failing.
func upsertWorkflow(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, wf definition.Workflow, src Source, actor string) (ApplyItem, uuid.UUID, error) {
	canon, hash, err := definition.Canonical(wf)
	if err != nil {
		return ApplyItem{}, uuid.Nil, err
	}
	item := ApplyItem{Kind: "workflow", Slug: wf.Slug}
	tag, err := tx.Exec(ctx, `INSERT INTO workflows (id, org_id, slug) VALUES ($1, $2, $3) ON CONFLICT (org_id, slug) DO NOTHING`, uuid.New(), orgID, wf.Slug)
	if err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("insert workflow %s: %w", wf.Slug, err)
	}
	item.Created = tag.RowsAffected() == 1

	var (
		id         uuid.UUID
		curID      *uuid.UUID
		curVersion *int
		curHash    *string
	)
	err = tx.QueryRow(ctx, `
		SELECT w.id, v.id, v.version, v.hash
		FROM workflows w LEFT JOIN workflow_versions v ON v.id = w.current_version_id
		WHERE w.org_id = $1 AND w.slug = $2
		FOR UPDATE OF w`, orgID, wf.Slug).Scan(&id, &curID, &curVersion, &curHash)
	if err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("lock workflow %s: %w", wf.Slug, err)
	}
	if curHash != nil && *curHash == hash {
		item.Version = *curVersion
		return item, *curID, nil
	}
	next := 1
	if curVersion != nil {
		next = *curVersion + 1
	}
	vid := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO workflow_versions (id, workflow_id, version, definition, hash, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`, vid, id, next, canon, hash, string(src), actor); err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("insert workflow version %s: %w", wf.Slug, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE workflows SET current_version_id = $1, updated_at = now() WHERE id = $2`, vid, id); err != nil {
		return ApplyItem{}, uuid.Nil, fmt.Errorf("update workflow %s: %w", wf.Slug, err)
	}
	item.Version, item.Changed = next, true
	return item, vid, nil
}

func upsertForm(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, f definition.Form, pin *uuid.UUID, src Source, actor string) (ApplyItem, error) {
	canon, hash, err := definition.Canonical(f)
	if err != nil {
		return ApplyItem{}, err
	}
	item := ApplyItem{Kind: "form", Slug: f.Slug}
	tag, err := tx.Exec(ctx, `INSERT INTO forms (id, org_id, slug) VALUES ($1, $2, $3) ON CONFLICT (org_id, slug) DO NOTHING`, uuid.New(), orgID, f.Slug)
	if err != nil {
		return ApplyItem{}, fmt.Errorf("insert form %s: %w", f.Slug, err)
	}
	item.Created = tag.RowsAffected() == 1

	var (
		id         uuid.UUID
		curVersion *int
		curHash    *string
		curPin     *uuid.UUID
	)
	err = tx.QueryRow(ctx, `
		SELECT f.id, v.version, v.hash, v.workflow_version_id
		FROM forms f LEFT JOIN form_versions v ON v.id = f.current_version_id
		WHERE f.org_id = $1 AND f.slug = $2
		FOR UPDATE OF f`, orgID, f.Slug).Scan(&id, &curVersion, &curHash, &curPin)
	if err != nil {
		return ApplyItem{}, fmt.Errorf("lock form %s: %w", f.Slug, err)
	}
	if curHash != nil && *curHash == hash && samePin(curPin, pin) {
		item.Version = *curVersion
		return item, nil
	}
	next := 1
	if curVersion != nil {
		next = *curVersion + 1
	}
	vid := uuid.New()
	if _, err := tx.Exec(ctx, `INSERT INTO form_versions (id, form_id, version, definition, hash, workflow_version_id, source, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`, vid, id, next, canon, hash, pin, string(src), actor); err != nil {
		return ApplyItem{}, fmt.Errorf("insert form version %s: %w", f.Slug, err)
	}
	if _, err := tx.Exec(ctx, `UPDATE forms SET current_version_id = $1, updated_at = now() WHERE id = $2`, vid, id); err != nil {
		return ApplyItem{}, fmt.Errorf("update form %s: %w", f.Slug, err)
	}
	item.Version, item.Changed = next, true
	return item, nil
}

func samePin(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// currentWorkflowVersion returns the current version id of the workflow a form
// references, or nil when the form has no workflow.
func currentWorkflowVersion(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, slug string) (*uuid.UUID, error) {
	if slug == "" {
		return nil, nil
	}
	var id *uuid.UUID
	err := tx.QueryRow(ctx, `SELECT current_version_id FROM workflows WHERE org_id = $1 AND slug = $2`, orgID, slug).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && id == nil) {
		return nil, fmt.Errorf("workflow %q: %w", slug, ErrNotFound)
	}
	return id, err
}

// dependentForms returns the current definitions of forms that reference workflowSlug.
func dependentForms(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, workflowSlug string) ([]definition.Form, error) {
	rows, err := tx.Query(ctx, `
		SELECT v.definition
		FROM forms f JOIN form_versions v ON v.id = f.current_version_id
		WHERE f.org_id = $1 AND v.definition->>'workflow' = $2
		ORDER BY f.slug`, orgID, workflowSlug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []definition.Form
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var f definition.Form
		if err := json.Unmarshal(raw, &f); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

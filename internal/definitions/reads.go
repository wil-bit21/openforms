package definitions

import (
	"context"

	"github.com/google/uuid"
	"github.com/openforms/openforms/internal/definition"
)

// FormVersions lists a form's versions, newest first.
func (s *Store) FormVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error) {
	return s.versions(ctx, `SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
		FROM form_versions v JOIN forms f ON f.id = v.form_id
		WHERE f.org_id = $1 AND f.slug = $2 ORDER BY v.version DESC`, orgID, slug)
}

func (s *Store) versions(ctx context.Context, sql string, args ...any) ([]VersionInfo, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VersionInfo
	for rows.Next() {
		var (
			v   VersionInfo
			src string
		)
		if err := rows.Scan(&v.ID, &v.Version, &v.Hash, &src, &v.CreatedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Source = Source(src)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, ErrNotFound
	}
	return out, nil
}

// GetFormVersion returns a form at a specific version id. Cached: versions are immutable.
func (s *Store) GetFormVersion(ctx context.Context, versionID uuid.UUID) (FormRecord, error) {
	if v, ok := s.formVersions.Load(versionID); ok {
		return v.(FormRecord), nil
	}
	rec, err := scanForm(s.pool.QueryRow(ctx, formCols+` WHERE v.id = $1`, versionID))
	if err != nil {
		return FormRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	s.formVersions.Store(versionID, rec)
	return rec, nil
}

// FormVersion returns a form at version number n.
func (s *Store) FormVersion(ctx context.Context, orgID uuid.UUID, slug string, n int) (FormRecord, error) {
	rec, err := scanForm(s.pool.QueryRow(ctx, formCols+` WHERE f.org_id = $1 AND f.slug = $2 AND v.version = $3`, orgID, slug, n))
	if err != nil {
		return FormRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	return rec, nil
}

// ListForms returns every form at its current version, ordered by slug.
func (s *Store) ListForms(ctx context.Context, orgID uuid.UUID) ([]FormRecord, error) {
	rows, err := s.pool.Query(ctx, formCols+` WHERE f.org_id = $1 AND f.current_version_id = v.id ORDER BY f.slug`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FormRecord{}
	for rows.Next() {
		rec, err := scanForm(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// GetWorkflowVersion returns a workflow at a specific version id. Cached: versions are immutable.
func (s *Store) GetWorkflowVersion(ctx context.Context, versionID uuid.UUID) (WorkflowRecord, error) {
	if v, ok := s.workflowVersions.Load(versionID); ok {
		return v.(WorkflowRecord), nil
	}
	rec, err := scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE v.id = $1`, versionID))
	if err != nil {
		return WorkflowRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	s.workflowVersions.Store(versionID, rec)
	return rec, nil
}

// WorkflowVersion returns a workflow at version number n.
func (s *Store) WorkflowVersion(ctx context.Context, orgID uuid.UUID, slug string, n int) (WorkflowRecord, error) {
	rec, err := scanWorkflow(s.pool.QueryRow(ctx, workflowCols+` WHERE w.org_id = $1 AND w.slug = $2 AND v.version = $3`, orgID, slug, n))
	if err != nil {
		return WorkflowRecord{}, err
	}
	rec.UpdatedAt = rec.Current.CreatedAt
	return rec, nil
}

// ListWorkflows returns every workflow at its current version, ordered by slug.
func (s *Store) ListWorkflows(ctx context.Context, orgID uuid.UUID) ([]WorkflowRecord, error) {
	rows, err := s.pool.Query(ctx, workflowCols+` WHERE w.org_id = $1 AND w.current_version_id = v.id ORDER BY w.slug`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowRecord{}
	for rows.Next() {
		rec, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rec)
	}
	return out, rows.Err()
}

// WorkflowVersions lists a workflow's versions, newest first.
func (s *Store) WorkflowVersions(ctx context.Context, orgID uuid.UUID, slug string) ([]VersionInfo, error) {
	return s.versions(ctx, `SELECT v.id, v.version, v.hash, v.source, v.created_by, v.created_at
		FROM workflow_versions v JOIN workflows w ON w.id = v.workflow_id
		WHERE w.org_id = $1 AND w.slug = $2 ORDER BY v.version DESC`, orgID, slug)
}

// Export returns the current definitions of the org, sorted by slug.
func (s *Store) Export(ctx context.Context, orgID uuid.UUID) ([]definition.Form, []definition.Workflow, error) {
	forms, err := s.ListForms(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	wfs, err := s.ListWorkflows(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	outF := make([]definition.Form, 0, len(forms))
	for _, f := range forms {
		outF = append(outF, f.Definition)
	}
	outW := make([]definition.Workflow, 0, len(wfs))
	for _, w := range wfs {
		outW = append(outW, w.Definition)
	}
	return outF, outW, nil
}

package definitions

import (
	"context"

	"github.com/google/uuid"
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

package submissions

import (
	"context"
	"encoding/base64"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ListFilter struct {
	FormSlug, State string
	AssigneeID      *uuid.UUID
	Unassigned      bool
	Cursor          string
	Limit           int // default 50, max 200
}

func encodeCursor(t time.Time, id uuid.UUID) string {
	return base64.RawURLEncoding.EncodeToString([]byte(t.UTC().Format(time.RFC3339Nano) + "|" + id.String()))
}

func decodeCursor(c string) (time.Time, uuid.UUID, error) {
	raw, err := base64.RawURLEncoding.DecodeString(c)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	ts, idStr, ok := strings.Cut(string(raw), "|")
	if !ok {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	t, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	id, err := uuid.Parse(idStr)
	if err != nil {
		return time.Time{}, uuid.Nil, ErrInvalidCursor
	}
	return t, id, nil
}

// List returns submissions newest first (created_at DESC, id DESC) and an
// opaque cursor for the next page ("" on the last page).
func (s *Service) List(ctx context.Context, orgID uuid.UUID, f ListFilter) ([]Submission, string, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	var (
		curAt *time.Time
		curID *uuid.UUID
	)
	if f.Cursor != "" {
		t, id, err := decodeCursor(f.Cursor)
		if err != nil {
			return nil, "", err
		}
		curAt, curID = &t, &id
	}
	rows, err := s.pool.Query(ctx, submissionCols+`
		WHERE s.org_id = $1
		  AND ($2::text = '' OR f.slug = $2)
		  AND ($3::text = '' OR s.state = $3)
		  AND ($4::uuid IS NULL OR s.assignee_id = $4)
		  AND (NOT $5::bool OR s.assignee_id IS NULL)
		  AND ($6::timestamptz IS NULL OR (s.created_at, s.id) < ($6::timestamptz, $7::uuid))
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $8`,
		orgID, f.FormSlug, f.State, f.AssigneeID, f.Unassigned, curAt, curID, limit+1)
	if err != nil {
		return nil, "", err
	}
	items := []Submission{}
	for rows.Next() {
		sub, err := scanRow(rows)
		if err != nil {
			rows.Close()
			return nil, "", err
		}
		items = append(items, sub)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	next := ""
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	for i := range items {
		if err := s.ResolveState(ctx, &items[i]); err != nil {
			return nil, "", err
		}
	}
	return items, next, nil
}

// CountByForm returns the number of submissions per form id.
func (s *Service) CountByForm(ctx context.Context, orgID uuid.UUID) (map[uuid.UUID]int, error) {
	rows, err := s.pool.Query(ctx, `SELECT form_id, count(*) FROM submissions WHERE org_id = $1 GROUP BY form_id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]int{}
	for rows.Next() {
		var (
			id uuid.UUID
			n  int
		)
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}

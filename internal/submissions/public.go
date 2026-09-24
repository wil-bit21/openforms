package submissions

import (
	"context"
	"crypto/subtle"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/definition"
)

type PublicStatus struct {
	ID                           uuid.UUID
	FormTitle, State, StateLabel string
	Terminal                     bool
	States                       []definition.State
	History                      []PublicHistoryItem
	CreatedAt                    time.Time
}

type PublicHistoryItem struct {
	State, Label string
	At           time.Time
}

// PublicStatus returns what a respondent may see about their submission.
// Any unknown id or token mismatch yields ErrNotFound (constant-time compare).
func (s *Service) PublicStatus(ctx context.Context, id uuid.UUID, token string) (PublicStatus, error) {
	var (
		orgID      uuid.UUID
		storedHash string
	)
	err := s.pool.QueryRow(ctx, `SELECT org_id, receipt_token_hash FROM submissions WHERE id = $1`, id).Scan(&orgID, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicStatus{}, ErrNotFound
	}
	if err != nil {
		return PublicStatus{}, err
	}
	if token == "" || subtle.ConstantTimeCompare([]byte(hashToken(token)), []byte(storedHash)) != 1 {
		return PublicStatus{}, ErrNotFound
	}

	sub, err := s.Get(ctx, orgID, id)
	if err != nil {
		return PublicStatus{}, err
	}
	form, err := s.defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		return PublicStatus{}, err
	}
	var wf *definition.Workflow
	states := []definition.State{{Key: NoWorkflowState, Label: NoWorkflowStateLabel, Terminal: true}}
	if sub.WorkflowVersionID != nil {
		rec, err := s.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			return PublicStatus{}, err
		}
		wf = &rec.Definition
		states = wf.States
	}

	rows, err := s.pool.Query(ctx, `SELECT to_state, created_at FROM submission_events
		WHERE submission_id = $1 AND to_state IS NOT NULL ORDER BY id`, id)
	if err != nil {
		return PublicStatus{}, err
	}
	defer rows.Close()
	history := []PublicHistoryItem{}
	for rows.Next() {
		var item PublicHistoryItem
		if err := rows.Scan(&item.State, &item.At); err != nil {
			return PublicStatus{}, err
		}
		item.Label, _ = stateLabel(wf, item.State)
		history = append(history, item)
	}
	if err := rows.Err(); err != nil {
		return PublicStatus{}, err
	}
	return PublicStatus{
		ID: sub.ID, FormTitle: form.Definition.Title,
		State: sub.State, StateLabel: sub.StateLabel, Terminal: sub.Terminal,
		States: states, History: history, CreatedAt: sub.CreatedAt,
	}, nil
}

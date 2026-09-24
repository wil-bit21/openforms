// Package submissions stores form submissions and their event history.
package submissions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
)

var (
	ErrNotFound      = errors.New("submission not found")
	ErrFormNotPublic = errors.New("form is not public")
	ErrInvalidCursor = errors.New("invalid cursor")
)

// State used by forms that have no workflow.
const (
	NoWorkflowState      = "submitted"
	NoWorkflowStateLabel = "Submitted"
)

type Submission struct {
	ID, OrgID, FormID, FormVersionID uuid.UUID
	FormSlug                         string
	FormVersion                      int
	WorkflowVersionID                *uuid.UUID
	State, StateLabel                string
	Terminal                         bool
	Data, Fields                     map[string]any
	AssigneeID                       *uuid.UUID
	AssigneeName, AssigneeEmail      string
	CreatedAt, UpdatedAt             time.Time
}

// CreatedHook runs inside the transaction that inserts a submission.
// wf is nil when the form has no workflow. Returning an error rolls back.
type CreatedHook func(ctx context.Context, tx pgx.Tx, sub Submission, wf *definition.Workflow) error

type Service struct {
	pool *pgxpool.Pool
	defs *definitions.Store
	hook CreatedHook
}

func NewService(pool *pgxpool.Pool, defs *definitions.Store) *Service {
	return &Service{pool: pool, defs: defs}
}

// SetCreatedHook registers the hook run for every new submission. Call during wiring only.
func (s *Service) SetCreatedHook(h CreatedHook) { s.hook = h }

const submissionCols = `SELECT s.id, s.org_id, s.form_id, s.form_version_id, f.slug, fv.version,
	s.workflow_version_id, s.state, s.data, s.fields, s.assignee_id,
	COALESCE(u.name, ''), COALESCE(u.email, ''), s.created_at, s.updated_at
FROM submissions s
JOIN forms f ON f.id = s.form_id
JOIN form_versions fv ON fv.id = s.form_version_id
LEFT JOIN users u ON u.id = s.assignee_id`

// scanRow reads one row of submissionCols. StateLabel/Terminal are filled by ResolveState.
func scanRow(row pgx.Row) (Submission, error) {
	var (
		sub          Submission
		data, fields []byte
	)
	err := row.Scan(&sub.ID, &sub.OrgID, &sub.FormID, &sub.FormVersionID, &sub.FormSlug, &sub.FormVersion,
		&sub.WorkflowVersionID, &sub.State, &data, &fields, &sub.AssigneeID,
		&sub.AssigneeName, &sub.AssigneeEmail, &sub.CreatedAt, &sub.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Submission{}, ErrNotFound
	}
	if err != nil {
		return Submission{}, fmt.Errorf("scan submission: %w", err)
	}
	if err := json.Unmarshal(data, &sub.Data); err != nil {
		return Submission{}, fmt.Errorf("decode data: %w", err)
	}
	if err := json.Unmarshal(fields, &sub.Fields); err != nil {
		return Submission{}, fmt.Errorf("decode fields: %w", err)
	}
	if sub.Data == nil {
		sub.Data = map[string]any{}
	}
	if sub.Fields == nil {
		sub.Fields = map[string]any{}
	}
	return sub, nil
}

// ResolveState fills StateLabel and Terminal from the submission's pinned workflow version.
func (s *Service) ResolveState(ctx context.Context, sub *Submission) error {
	if sub.WorkflowVersionID == nil {
		sub.StateLabel, sub.Terminal = NoWorkflowStateLabel, true
		return nil
	}
	wf, err := s.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
	if err != nil {
		return fmt.Errorf("load workflow version: %w", err)
	}
	sub.StateLabel, sub.Terminal = stateLabel(&wf.Definition, sub.State)
	return nil
}

// stateLabel returns the label and terminal flag of state in wf (nil = no workflow).
func stateLabel(wf *definition.Workflow, state string) (string, bool) {
	if wf == nil {
		return NoWorkflowStateLabel, true
	}
	st, ok := wf.State(state)
	if !ok {
		return state, false
	}
	return st.Label, st.Terminal
}

func (s *Service) Get(ctx context.Context, orgID, id uuid.UUID) (Submission, error) {
	sub, err := scanRow(s.pool.QueryRow(ctx, submissionCols+` WHERE s.org_id = $1 AND s.id = $2`, orgID, id))
	if err != nil {
		return Submission{}, err
	}
	return sub, s.ResolveState(ctx, &sub)
}

// GetForUpdate locks the submission row for the rest of tx.
func (s *Service) GetForUpdate(ctx context.Context, tx pgx.Tx, orgID, id uuid.UUID) (Submission, error) {
	sub, err := scanRow(tx.QueryRow(ctx, submissionCols+` WHERE s.org_id = $1 AND s.id = $2 FOR UPDATE OF s`, orgID, id))
	if err != nil {
		return Submission{}, err
	}
	return sub, s.ResolveState(ctx, &sub)
}

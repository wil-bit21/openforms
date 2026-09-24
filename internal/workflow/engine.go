package workflow

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

type AvailableTransition struct {
	Key, Label, To, ToLabel string
	RequireFields           []string
	Allowed                 bool
	Reason                  string // "" or "role"
}

type TransitionInput struct {
	Transition    string
	Fields        map[string]any
	Comment       string
	ExpectedState string
}

type Engine struct {
	pool  *pgxpool.Pool
	defs  *definitions.Store
	subs  *submissions.Service
	auth  *auth.Service
	queue *jobs.Queue

	// beforeCommit runs at the end of a transition transaction; tests only.
	beforeCommit func() error
}

func NewEngine(pool *pgxpool.Pool, defs *definitions.Store, subs *submissions.Service, authSvc *auth.Service, q *jobs.Queue) *Engine {
	return &Engine{pool: pool, defs: defs, subs: subs, auth: authSvc, queue: q}
}

// workflowFor returns the workflow version pinned by the submission.
func (e *Engine) workflowFor(ctx context.Context, sub submissions.Submission) (*definition.Workflow, error) {
	if sub.WorkflowVersionID == nil {
		return nil, ErrNoWorkflow
	}
	rec, err := e.defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
	if err != nil {
		return nil, err
	}
	wf := rec.Definition
	return &wf, nil
}

// Available lists the transitions leaving the submission's current state and
// whether p may perform each. Forms without a workflow yield an empty slice.
func (e *Engine) Available(ctx context.Context, p auth.Principal, sub submissions.Submission) ([]AvailableTransition, error) {
	out := []AvailableTransition{}
	wf, err := e.workflowFor(ctx, sub)
	if errors.Is(err, ErrNoWorkflow) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	for _, t := range wf.Transitions {
		if !slices.Contains(t.From, sub.State) {
			continue
		}
		at := AvailableTransition{
			Key: t.Key, Label: t.Label, To: t.To,
			RequireFields: append([]string{}, t.Guard.RequireFields...),
			Allowed:       true,
		}
		if st, ok := wf.State(t.To); ok {
			at.ToLabel = st.Label
		}
		if !p.HasAnyRole(t.Guard.Roles...) {
			at.Allowed, at.Reason = false, "role"
		}
		out = append(out, at)
	}
	return out, nil
}

// OnCreated enqueues the workflow's onSubmit actions in the creating
// transaction. Registered with submissions.Service.SetCreatedHook.
func (e *Engine) OnCreated(ctx context.Context, tx pgx.Tx, sub submissions.Submission, wf *definition.Workflow) error {
	if wf == nil || len(wf.OnSubmit) == 0 {
		return nil
	}
	var eventID int64
	err := tx.QueryRow(ctx,
		`SELECT id FROM submission_events WHERE submission_id = $1 AND type = 'created' ORDER BY id LIMIT 1`,
		sub.ID).Scan(&eventID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	for _, a := range wf.OnSubmit {
		if err := actions.Enqueue(ctx, e.queue, tx, sub.OrgID, actions.Payload{
			SubmissionID: sub.ID, Trigger: actions.TriggerSubmit, EventID: eventID, Action: a,
		}); err != nil {
			return err
		}
	}
	return nil
}

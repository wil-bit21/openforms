package workflow

import (
	"context"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

// Transition runs the spec §6.10 algorithm in one transaction: state change,
// event and action jobs commit together or not at all.
func (e *Engine) Transition(ctx context.Context, p auth.Principal, id uuid.UUID, in TransitionInput) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		wf, err := e.workflowFor(ctx, sub)
		if err != nil {
			return err
		}
		t, ok := wf.Transition(in.Transition)
		if !ok {
			return ErrUnknownTransition
		}
		if in.ExpectedState != "" && in.ExpectedState != sub.State {
			return ErrStateConflict
		}
		if !slices.Contains(t.From, sub.State) {
			return ErrInvalidState
		}
		if !p.HasAnyRole(t.Guard.Roles...) {
			return ErrForbidden
		}
		merged, changed, err := mergeFields(*wf, sub.Fields, in.Fields)
		if err != nil {
			return err
		}
		var problems []definition.Problem
		for _, k := range t.Guard.RequireFields {
			if isEmpty(merged[k]) {
				problems = append(problems, definition.Problem{Path: "fields." + k, Message: "required for transition " + t.Key})
			}
		}
		if len(problems) > 0 {
			return &definition.ValidationError{Problems: problems}
		}

		if _, err := tx.Exec(ctx,
			`UPDATE submissions SET state = $3, fields = $4, updated_at = now() WHERE org_id = $1 AND id = $2`,
			p.OrgID, id, t.To, merged); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		ev, err := submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id,
			OrgID:        p.OrgID,
			Type:         "transition",
			FromState:    sub.State,
			ToState:      t.To,
			Transition:   t.Key,
			ActorType:    actor.Type,
			ActorID:      actor.ID,
			ActorName:    actor.Name,
			Payload:      map[string]any{"comment": strings.TrimSpace(in.Comment), "fields": changed},
		})
		if err != nil {
			return err
		}
		for _, a := range t.Actions {
			if err := actions.Enqueue(ctx, e.queue, tx, p.OrgID, actions.Payload{
				SubmissionID: id, Trigger: t.Key, EventID: ev.ID, Action: a,
			}); err != nil {
				return err
			}
		}
		if e.beforeCommit != nil {
			return e.beforeCommit()
		}
		return nil
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}

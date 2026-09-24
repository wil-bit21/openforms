package workflow

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
)

const maxCommentRunes = 10000

// UpdateFields edits workflow fields without changing state.
func (e *Engine) UpdateFields(ctx context.Context, p auth.Principal, id uuid.UUID, fields map[string]any) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		wf, err := e.workflowFor(ctx, sub)
		if err != nil {
			return err
		}
		merged, changed, err := mergeFields(*wf, sub.Fields, fields)
		if err != nil {
			return err
		}
		if len(changed) == 0 {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE submissions SET fields = $3, updated_at = now() WHERE org_id = $1 AND id = $2`,
			p.OrgID, id, merged); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		_, err = submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id, OrgID: p.OrgID, Type: "fields_updated",
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
			Payload: map[string]any{"fields": changed},
		})
		return err
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}

// Comment appends a comment event to the submission's timeline.
func (e *Engine) Comment(ctx context.Context, p auth.Principal, id uuid.UUID, body string) (submissions.Event, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return submissions.Event{}, &definition.ValidationError{Problems: []definition.Problem{{Path: "body", Message: "comment cannot be empty"}}}
	}
	if utf8.RuneCountInString(body) > maxCommentRunes {
		return submissions.Event{}, &definition.ValidationError{Problems: []definition.Problem{{Path: "body", Message: "comment is too long (max 10000 characters)"}}}
	}
	if _, err := e.subs.Get(ctx, p.OrgID, id); err != nil {
		return submissions.Event{}, err
	}
	actor := submissions.ActorFromPrincipal(p)
	return submissions.InsertEvent(ctx, e.pool, submissions.Event{
		SubmissionID: id, OrgID: p.OrgID, Type: "comment",
		ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
		Payload: map[string]any{"body": body},
	})
}

// Assign sets or clears the submission's assignee (userID nil = unassign).
func (e *Engine) Assign(ctx context.Context, p auth.Principal, id uuid.UUID, userID *uuid.UUID) (submissions.Submission, error) {
	err := db.WithTx(ctx, e.pool, func(tx pgx.Tx) error {
		sub, err := e.subs.GetForUpdate(ctx, tx, p.OrgID, id)
		if err != nil {
			return err
		}
		payload := map[string]any{"assigneeId": nil, "assigneeName": ""}
		if userID != nil {
			var name string
			err := tx.QueryRow(ctx, `SELECT name FROM users WHERE org_id = $1 AND id = $2`, p.OrgID, *userID).Scan(&name)
			if errors.Is(err, pgx.ErrNoRows) {
				return &definition.ValidationError{Problems: []definition.Problem{{Path: "userId", Message: "unknown user"}}}
			}
			if err != nil {
				return err
			}
			payload = map[string]any{"assigneeId": userID.String(), "assigneeName": name}
		}
		if sameAssignee(sub.AssigneeID, userID) {
			return nil
		}
		if err := submissions.SetAssignee(ctx, tx, p.OrgID, id, userID); err != nil {
			return err
		}
		actor := submissions.ActorFromPrincipal(p)
		_, err = submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: id, OrgID: p.OrgID, Type: "assigned",
			ActorType: actor.Type, ActorID: actor.ID, ActorName: actor.Name,
			Payload: payload,
		})
		return err
	})
	if err != nil {
		return submissions.Submission{}, err
	}
	return e.subs.Get(ctx, p.OrgID, id)
}

func sameAssignee(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

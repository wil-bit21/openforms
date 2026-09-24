package actions

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
)

const (
	KindWebhook = "action.webhook"
	KindEmail   = "action.email"
	KindAssign  = "action.assign"

	// TriggerSubmit is Payload.Trigger for onSubmit actions; otherwise Trigger
	// is the transition key.
	TriggerSubmit = "submit"

	// SystemActorName is the actor_name on events written by actions.
	SystemActorName = "openforms"
)

type Payload struct {
	SubmissionID uuid.UUID         `json:"submissionId"`
	Trigger      string            `json:"trigger"`
	EventID      int64             `json:"eventId"`
	Action       definition.Action `json:"action"`
}

func KindFor(t definition.ActionType) (string, error) {
	switch t {
	case definition.ActionWebhook:
		return KindWebhook, nil
	case definition.ActionEmail:
		return KindEmail, nil
	case definition.ActionAssign:
		return KindAssign, nil
	}
	return "", fmt.Errorf("actions: unknown action type %q", t)
}

// Enqueue schedules p as a job inside tx (transactional outbox).
func Enqueue(ctx context.Context, q *jobs.Queue, tx db.DBTX, orgID uuid.UUID, p Payload) error {
	kind, err := KindFor(p.Action.Type)
	if err != nil {
		return err
	}
	_, err = q.Enqueue(ctx, tx, orgID, kind, p)
	return err
}

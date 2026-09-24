package submissions

import (
	"context"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/db"
)

// SetAssignee sets (or clears, when userID is nil) a submission's assignee.
// It writes no event; callers record the "assigned" event themselves.
func SetAssignee(ctx context.Context, q db.DBTX, orgID, id uuid.UUID, userID *uuid.UUID) error {
	tag, err := q.Exec(ctx,
		`UPDATE submissions SET assignee_id = $3, updated_at = now() WHERE org_id = $1 AND id = $2`,
		orgID, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

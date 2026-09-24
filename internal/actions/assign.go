package actions

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

func (r *runner) assign(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	a := ac.payload.Action
	var target auth.User
	switch {
	case a.User != "":
		u, err := r.d.Auth.GetUserByEmail(ctx, job.OrgID, a.User)
		if errors.Is(err, auth.ErrNotFound) {
			return jobs.Permanent(fmt.Errorf("assign: no user with email %q", a.User))
		}
		if err != nil {
			return err
		}
		target = u
	case a.Role != "":
		u, err := r.leastLoaded(ctx, job.OrgID, a.Role)
		if err != nil {
			return err
		}
		target = u
	default:
		return jobs.Permanent(errors.New("assign: action needs user or role"))
	}

	err = db.WithTx(ctx, r.d.Pool, func(tx pgx.Tx) error {
		if err := submissions.SetAssignee(ctx, tx, job.OrgID, ac.sub.ID, &target.ID); err != nil {
			return err
		}
		_, err := submissions.InsertEvent(ctx, tx, submissions.Event{
			SubmissionID: ac.sub.ID,
			OrgID:        job.OrgID,
			Type:         "assigned",
			ActorType:    "system",
			ActorName:    SystemActorName,
			Payload:      map[string]any{"assigneeId": target.ID.String(), "assigneeName": target.Name, "trigger": ac.payload.Trigger},
		})
		return err
	})
	if err != nil {
		return err
	}
	return r.recordSuccess(ctx, ac)
}

// leastLoaded picks, among users with role (or admins if none), the one with
// the fewest assigned non-terminal submissions; ties go to the earliest created.
func (r *runner) leastLoaded(ctx context.Context, orgID uuid.UUID, role string) (auth.User, error) {
	cands, err := r.d.Auth.UsersWithRole(ctx, orgID, role)
	if err != nil {
		return auth.User{}, err
	}
	if len(cands) == 0 {
		if cands, err = r.d.Auth.UsersWithRole(ctx, orgID, auth.RoleAdmin); err != nil {
			return auth.User{}, err
		}
	}
	if len(cands) == 0 {
		return auth.User{}, jobs.Permanent(fmt.Errorf("assign: no users with role %q or admin", role))
	}
	ids := make([]uuid.UUID, len(cands))
	for i, c := range cands {
		ids[i] = c.ID
	}
	rows, err := r.d.Pool.Query(ctx,
		`SELECT assignee_id, workflow_version_id, state FROM submissions
		 WHERE org_id = $1 AND assignee_id = ANY($2)`, orgID, ids)
	if err != nil {
		return auth.User{}, err
	}
	defer rows.Close()
	load := map[uuid.UUID]int{}
	for rows.Next() {
		var assignee uuid.UUID
		var wv *uuid.UUID
		var state string
		if err := rows.Scan(&assignee, &wv, &state); err != nil {
			return auth.User{}, err
		}
		terminal, err := r.isTerminal(ctx, wv, state)
		if err != nil {
			return auth.User{}, err
		}
		if !terminal {
			load[assignee]++
		}
	}
	if err := rows.Err(); err != nil {
		return auth.User{}, err
	}
	sort.SliceStable(cands, func(i, j int) bool {
		li, lj := load[cands[i].ID], load[cands[j].ID]
		if li != lj {
			return li < lj
		}
		return cands[i].CreatedAt.Before(cands[j].CreatedAt)
	})
	return cands[0], nil
}

func (r *runner) isTerminal(ctx context.Context, workflowVersionID *uuid.UUID, state string) (bool, error) {
	if workflowVersionID == nil {
		return true, nil // forms without a workflow end in "submitted", which is terminal
	}
	rec, err := r.d.Defs.GetWorkflowVersion(ctx, *workflowVersionID)
	if err != nil {
		return false, err
	}
	st, ok := rec.Definition.State(state)
	return ok && st.Terminal, nil
}

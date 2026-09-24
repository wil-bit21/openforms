package actions_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

func TestEmail_RendersAndSends(t *testing.T) {
	f, m := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "{{submission.data.email}}",
		Subject: "Thanks {{submission.data.name}}", Body: "See {{submission.url}}",
	})

	runOne(t, f)

	if len(m.sent) != 1 {
		t.Fatalf("sent = %+v", m.sent)
	}
	s := m.sent[0]
	if s.To != "ada@example.com" || s.Subject != "Thanks Ada" || s.Body != "See http://test.local/admin/submissions/"+sub.ID.String() {
		t.Fatalf("mail = %+v", s)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded")) != 1 {
		t.Fatal("missing action_succeeded event")
	}
}

func TestEmail_EmptyRecipientIsPermanent(t *testing.T) {
	f, m := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "{{submission.data.missing}}", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
	if len(m.sent) != 0 || len(eventsOfType(t, f, sub.ID, "action_failed")) != 1 {
		t.Fatalf("sent=%v", m.sent)
	}
}

func TestEmail_InvalidRecipientIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "not-an-email", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
}

func TestEmail_MailerErrorRetries(t *testing.T) {
	f, m := setupActions(t, nil)
	m.err = errors.New("smtp down")
	job := enqueueAction(t, f, f.Applicant(t), actions.TriggerSubmit, definition.Action{
		Type: definition.ActionEmail, To: "a@example.com", Subject: "s", Body: "b",
	})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusPending || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
}

func TestAssign_ByRolePicksLeastLoaded(t *testing.T) {
	f, _ := setupActions(t, nil)
	ctx := context.Background()
	busy := f.User(t, "busy@example.com", "reviewer")
	idle := f.User(t, "idle@example.com", "reviewer")
	f.User(t, "other@example.com", "hiring-manager")

	// busy has one open (non-terminal) assignment.
	open := f.Applicant(t)
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, open.ID, &busy.ID); err != nil {
		t.Fatal(err)
	}
	// idle has one assignment, but it is terminal (rejected) and must not count.
	closed := f.Applicant(t)
	if _, err := f.Pool.Exec(ctx, `UPDATE submissions SET state = 'rejected', assignee_id = $2 WHERE id = $1`, closed.ID, idle.ID); err != nil {
		t.Fatal(err)
	}

	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)

	got := f.Reload(t, sub.ID)
	if got.AssigneeID == nil || *got.AssigneeID != idle.ID {
		t.Fatalf("assigned to %v, want idle %v", got.AssigneeID, idle.ID)
	}
	evs := eventsOfType(t, f, sub.ID, "assigned")
	if len(evs) != 1 || evs[0].Payload["assigneeId"] != idle.ID.String() || evs[0].ActorType != "system" {
		t.Fatalf("assigned events = %+v", evs)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded")) != 1 {
		t.Fatal("missing action_succeeded")
	}
}

func TestAssign_TieGoesToEarliestCreated(t *testing.T) {
	f, _ := setupActions(t, nil)
	ctx := context.Background()
	later := f.User(t, "later@example.com", "reviewer")
	earlier := f.User(t, "earlier@example.com", "reviewer")
	base := time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
	f.Pool.Exec(ctx, `UPDATE users SET created_at = $2 WHERE id = $1`, later.ID, base.Add(time.Hour))
	f.Pool.Exec(ctx, `UPDATE users SET created_at = $2 WHERE id = $1`, earlier.ID, base)

	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != earlier.ID {
		t.Fatalf("assigned to %v, want earliest %v", got.AssigneeID, earlier.ID)
	}
}

func TestAssign_FallsBackToAdmin(t *testing.T) {
	f, _ := setupActions(t, nil)
	admin := f.User(t, "admin@example.com", "admin")
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "nobody-has-this"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != admin.ID {
		t.Fatalf("assigned to %v, want admin", got.AssigneeID)
	}
}

func TestAssign_NoCandidatesIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, Role: "reviewer"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_failed")) != 1 {
		t.Fatal("missing action_failed")
	}
}

func TestAssign_ByUserEmail(t *testing.T) {
	f, _ := setupActions(t, nil)
	u := f.User(t, "named@example.com", "reviewer")
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, User: "NAMED@example.com"})
	runOne(t, f)
	if got := f.Reload(t, sub.ID); got.AssigneeID == nil || *got.AssigneeID != u.ID {
		t.Fatalf("assigned to %v", got.AssigneeID)
	}

	sub2 := f.Applicant(t)
	job := enqueueAction(t, f, sub2, actions.TriggerSubmit, definition.Action{Type: definition.ActionAssign, User: "ghost@example.com"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("unknown user job = %+v", j)
	}
}

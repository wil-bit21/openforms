package workflow_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func countEvents(t *testing.T, f *fixture.Fixture, id uuid.UUID, typ string) int {
	t.Helper()
	n := 0
	for _, e := range f.Events(t, id) {
		if e.Type == typ {
			n++
		}
	}
	return n
}

func TestUpdateFields(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	got, err := e.UpdateFields(ctx, p, sub.ID, map[string]any{"score": 3, "priority": "high"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Fields["score"] != float64(3) || got.Fields["priority"] != "high" || got.State != "new" {
		t.Fatalf("fields = %v", got.Fields)
	}

	got, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"priority": nil})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Fields["priority"]; ok || got.Fields["score"] != float64(3) {
		t.Fatalf("after unset: %v", got.Fields)
	}
	if n := countEvents(t, f, sub.ID, "fields_updated"); n != 2 {
		t.Fatalf("fields_updated events = %d", n)
	}

	if _, err := e.UpdateFields(ctx, p, sub.ID, map[string]any{"score": 3}); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, f, sub.ID, "fields_updated"); n != 2 {
		t.Fatalf("no-op update wrote an event (%d)", n)
	}

	_, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"priority": "urgent"})
	var ve *definition.ValidationError
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.priority" {
		t.Fatalf("invalid option err = %#v", err)
	}
	_, err = e.UpdateFields(ctx, p, sub.ID, map[string]any{"ghost": nil})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.ghost" {
		t.Fatalf("unknown unset err = %#v", err)
	}

	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	if _, err := e.UpdateFields(ctx, p, plain.ID, map[string]any{"score": 1}); !errors.Is(err, workflow.ErrNoWorkflow) {
		t.Fatalf("no workflow err = %v", err)
	}
}

func TestComment(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t)
	ctx := context.Background()

	ev, err := e.Comment(ctx, p, sub.ID, "  Strong portfolio.  ")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Type != "comment" || ev.Payload["body"] != "Strong portfolio." || ev.ActorName != p.Name || ev.ID == 0 {
		t.Fatalf("event = %+v", ev)
	}

	var ve *definition.ValidationError
	if _, err := e.Comment(ctx, p, sub.ID, "   "); !errors.As(err, &ve) || ve.Problems[0].Path != "body" {
		t.Fatalf("empty comment err = %#v", err)
	}
	if _, err := e.Comment(ctx, p, sub.ID, strings.Repeat("x", 10001)); !errors.As(err, &ve) {
		t.Fatalf("long comment err = %#v", err)
	}
	if _, err := e.Comment(ctx, p, uuid.New(), "hi"); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("unknown submission err = %v", err)
	}
}

func TestAssign(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	u := f.User(t, "assignee@example.com", "reviewer")
	ctx := context.Background()

	got, err := e.Assign(ctx, p, sub.ID, &u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.AssigneeID == nil || *got.AssigneeID != u.ID || got.AssigneeEmail != "assignee@example.com" {
		t.Fatalf("after assign: %+v", got)
	}
	if _, err := e.Assign(ctx, p, sub.ID, &u.ID); err != nil {
		t.Fatal(err)
	}
	if n := countEvents(t, f, sub.ID, "assigned"); n != 1 {
		t.Fatalf("re-assigning same user wrote events: %d", n)
	}

	got, err = e.Assign(ctx, p, sub.ID, nil)
	if err != nil || got.AssigneeID != nil {
		t.Fatalf("unassign: %+v, %v", got, err)
	}
	var last submissions.Event
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "assigned" {
			last = ev
		}
	}
	if last.Payload["assigneeId"] != nil || last.ActorName != p.Name {
		t.Fatalf("unassign event = %+v", last)
	}

	ghost := uuid.New()
	var ve *definition.ValidationError
	if _, err := e.Assign(ctx, p, sub.ID, &ghost); !errors.As(err, &ve) || ve.Problems[0].Path != "userId" {
		t.Fatalf("unknown user err = %#v", err)
	}
}

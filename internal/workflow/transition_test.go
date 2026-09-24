package workflow_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func newEngine(t *testing.T) (*fixture.Fixture, *workflow.Engine) {
	t.Helper()
	f := fixture.Setup(t)
	return f, workflow.NewEngine(f.Pool, f.Defs, f.Subs, f.Auth, f.Queue)
}

func payloads(t *testing.T, f *fixture.Fixture) []actions.Payload {
	t.Helper()
	var out []actions.Payload
	for _, j := range f.Jobs(t) {
		var p actions.Payload
		if err := json.Unmarshal(j.Payload, &p); err != nil {
			t.Fatal(err)
		}
		out = append(out, p)
	}
	return out
}

func transitionEvents(t *testing.T, f *fixture.Fixture, id uuid.UUID) []submissions.Event {
	t.Helper()
	var out []submissions.Event
	for _, e := range f.Events(t, id) {
		if e.Type == "transition" {
			out = append(out, e)
		}
	}
	return out
}

func TestAvailable(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	ctx := context.Background()

	reviewer := f.PrincipalWithRoles(t, "reviewer")
	av, err := e.Available(ctx, reviewer, sub)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]workflow.AvailableTransition{}
	for _, a := range av {
		keys[a.Key] = a
	}
	if len(av) != 2 || !keys["screen"].Allowed || !keys["reject"].Allowed {
		t.Fatalf("reviewer available = %+v", av)
	}
	if keys["screen"].ToLabel != "Screening" || keys["screen"].RequireFields == nil {
		t.Fatalf("screen = %+v", keys["screen"])
	}
	if got := keys["reject"].RequireFields; len(got) != 1 || got[0] != "rejectionReason" {
		t.Fatalf("reject requireFields = %v", got)
	}

	nobody := f.PrincipalWithRoles(t)
	av, _ = e.Available(ctx, nobody, sub)
	for _, a := range av {
		if a.Allowed || a.Reason != "role" {
			t.Fatalf("role-less principal got %+v", a)
		}
	}

	admin := f.PrincipalWithRoles(t, auth.RoleAdmin)
	av, _ = e.Available(ctx, admin, sub)
	for _, a := range av {
		if !a.Allowed {
			t.Fatalf("admin must pass every guard: %+v", a)
		}
	}

	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	av, err = e.Available(ctx, admin, plain)
	if err != nil || av == nil || len(av) != 0 {
		t.Fatalf("no-workflow available = %v, %v; want empty non-nil", av, err)
	}
}

func TestTransition_Success(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")

	got, err := e.Transition(context.Background(), p, sub.ID, workflow.TransitionInput{Transition: "screen", Comment: "  looks good  "})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "screening" || got.StateLabel != "Screening" {
		t.Fatalf("submission = %+v", got)
	}
	evs := transitionEvents(t, f, sub.ID)
	if len(evs) != 1 {
		t.Fatalf("transition events = %+v", evs)
	}
	ev := evs[0]
	if ev.FromState != "new" || ev.ToState != "screening" || ev.Transition != "screen" ||
		ev.ActorType != "user" || ev.ActorID == nil || *ev.ActorID != p.ID || ev.ActorName != p.Name {
		t.Fatalf("event = %+v", ev)
	}
	if ev.Payload["comment"] != "looks good" {
		t.Fatalf("payload = %v", ev.Payload)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("screen has no actions, got %d jobs", n)
	}
}

func TestTransition_Errors(t *testing.T) {
	f, e := newEngine(t)
	ctx := context.Background()
	reviewer := f.PrincipalWithRoles(t, "reviewer")
	sub := f.Applicant(t)
	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})

	cases := []struct {
		name string
		p    auth.Principal
		id   uuid.UUID
		in   workflow.TransitionInput
		want error
	}{
		{"unknown transition", reviewer, sub.ID, workflow.TransitionInput{Transition: "teleport"}, workflow.ErrUnknownTransition},
		{"wrong from-state", reviewer, sub.ID, workflow.TransitionInput{Transition: "invite", Fields: map[string]any{"score": 5}}, workflow.ErrInvalidState},
		{"missing role", f.PrincipalWithRoles(t, "hiring-manager"), sub.ID, workflow.TransitionInput{Transition: "screen"}, workflow.ErrForbidden},
		{"expected state mismatch", reviewer, sub.ID, workflow.TransitionInput{Transition: "screen", ExpectedState: "interview"}, workflow.ErrStateConflict},
		{"no workflow", reviewer, plain.ID, workflow.TransitionInput{Transition: "screen"}, workflow.ErrNoWorkflow},
		{"unknown submission", reviewer, uuid.New(), workflow.TransitionInput{Transition: "screen"}, submissions.ErrNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := e.Transition(ctx, c.p, c.id, c.in)
			if !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	if got := f.Reload(t, sub.ID); got.State != "new" {
		t.Fatalf("state changed to %s", got.State)
	}
}

func TestTransition_RequireFields(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	_, err := e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"rejectionReason": "   "}})
	var ve *definition.ValidationError
	if !errors.As(err, &ve) || len(ve.Problems) != 1 || ve.Problems[0].Path != "fields.rejectionReason" ||
		!strings.Contains(ve.Problems[0].Message, "required for transition reject") {
		t.Fatalf("err = %#v", err)
	}
	if got := f.Reload(t, sub.ID); got.State != "new" || len(got.Fields) != 0 {
		t.Fatalf("submission changed: %+v", got)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("failed transition enqueued %d jobs", n)
	}

	_, err = e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"score": "not a number", "rejectionReason": "x"}})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.score" {
		t.Fatalf("invalid field type err = %#v", err)
	}
	_, err = e.Transition(ctx, p, sub.ID, workflow.TransitionInput{Transition: "reject", Fields: map[string]any{"nope": 1, "rejectionReason": "x"}})
	if !errors.As(err, &ve) || ve.Problems[0].Path != "fields.nope" {
		t.Fatalf("unknown field err = %#v", err)
	}
}

func TestTransition_EnqueuesActionsWithEventID(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	ctx := context.Background()

	got, err := e.Transition(ctx, p, sub.ID, workflow.TransitionInput{
		Transition: "reject", Fields: map[string]any{"rejectionReason": "Not a fit"}, Comment: "sorry",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "rejected" || !got.Terminal || got.Fields["rejectionReason"] != "Not a fit" {
		t.Fatalf("submission = %+v", got)
	}
	ev := transitionEvents(t, f, sub.ID)[0]
	if fields, _ := ev.Payload["fields"].(map[string]any); fields["rejectionReason"] != "Not a fit" {
		t.Fatalf("event payload = %v", ev.Payload)
	}
	ps := payloads(t, f)
	if len(ps) != 2 || ps[0].Action.Type != definition.ActionEmail || ps[1].Action.Type != definition.ActionAssign {
		t.Fatalf("payloads = %+v", ps)
	}
	for _, pl := range ps {
		if pl.Trigger != "reject" || pl.EventID != ev.ID || pl.SubmissionID != sub.ID {
			t.Fatalf("payload = %+v, event id %d", pl, ev.ID)
		}
	}
	kinds := []string{f.Jobs(t)[0].Kind, f.Jobs(t)[1].Kind}
	if kinds[0] != actions.KindEmail || kinds[1] != actions.KindAssign {
		t.Fatalf("kinds = %v", kinds)
	}
}

func TestTransition_FieldsAlreadySetSatisfyGuard(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	admin := f.PrincipalWithRoles(t, auth.RoleAdmin)
	ctx := context.Background()
	if _, err := e.Transition(ctx, admin, sub.ID, workflow.TransitionInput{Transition: "screen", Fields: map[string]any{"score": 4}}); err != nil {
		t.Fatal(err)
	}
	got, err := e.Transition(ctx, admin, sub.ID, workflow.TransitionInput{Transition: "invite"})
	if err != nil {
		t.Fatalf("score set earlier must satisfy requireFields: %v", err)
	}
	if got.State != "interview" || got.Fields["score"] != float64(4) {
		t.Fatalf("submission = %+v", got)
	}
}

func TestTransition_RollbackLeavesNoJobs(t *testing.T) {
	f, e := newEngine(t)
	sub := f.Applicant(t)
	p := f.PrincipalWithRoles(t, "reviewer")
	boom := errors.New("commit exploded")
	workflow.SetBeforeCommitHook(e, func() error { return boom })

	_, err := e.Transition(context.Background(), p, sub.ID, workflow.TransitionInput{
		Transition: "reject", Fields: map[string]any{"rejectionReason": "x"},
	})
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("rolled-back transition left %d jobs", n)
	}
	if got := f.Reload(t, sub.ID); got.State != "new" || len(got.Fields) != 0 {
		t.Fatalf("rolled-back transition changed submission: %+v", got)
	}
	if n := len(transitionEvents(t, f, sub.ID)); n != 0 {
		t.Fatalf("rolled-back transition left %d events", n)
	}
}

func TestTransition_ConcurrentOnlyOneWins(t *testing.T) {
	for _, expected := range []string{"", "new"} {
		t.Run("expectedState="+expected, func(t *testing.T) {
			f, e := newEngine(t)
			sub := f.Applicant(t)
			p := f.PrincipalWithRoles(t, "reviewer")

			start := make(chan struct{})
			errs := make([]error, 2)
			var wg sync.WaitGroup
			for i := range errs {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					<-start
					_, errs[i] = e.Transition(context.Background(), p, sub.ID,
						workflow.TransitionInput{Transition: "screen", ExpectedState: expected})
				}(i)
			}
			close(start)
			wg.Wait()

			wins, losses := 0, 0
			for _, err := range errs {
				switch {
				case err == nil:
					wins++
				case errors.Is(err, workflow.ErrInvalidState), errors.Is(err, workflow.ErrStateConflict):
					losses++
				default:
					t.Fatalf("unexpected err: %v", err)
				}
			}
			if wins != 1 || losses != 1 {
				t.Fatalf("wins=%d losses=%d errs=%v", wins, losses, errs)
			}
			if n := len(transitionEvents(t, f, sub.ID)); n != 1 {
				t.Fatalf("transition events = %d", n)
			}
		})
	}
}

func TestOnCreated_EnqueuesOnSubmitActions(t *testing.T) {
	f, e := newEngine(t)
	f.Subs.SetCreatedHook(e.OnCreated)

	sub := f.Applicant(t)
	ps := payloads(t, f)
	if len(ps) != 1 || ps[0].Trigger != actions.TriggerSubmit || ps[0].SubmissionID != sub.ID ||
		ps[0].Action.Type != definition.ActionEmail {
		t.Fatalf("payloads = %+v", ps)
	}
	var createdID int64
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "created" {
			createdID = ev.ID
		}
	}
	if createdID != 0 && ps[0].EventID != createdID {
		t.Fatalf("EventID = %d, want created event %d", ps[0].EventID, createdID)
	}

	f.Submit(t, "feedback", map[string]any{"message": "hi"})
	if n := len(f.Jobs(t)); n != 1 {
		t.Fatalf("form without workflow enqueued jobs: %d total", n)
	}
}

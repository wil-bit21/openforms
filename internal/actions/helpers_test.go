package actions_test

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

type sentMail struct{ To, Subject, Body string }

type fakeMailer struct {
	mu   sync.Mutex
	sent []sentMail
	err  error
}

func (m *fakeMailer) Send(ctx context.Context, to, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, sentMail{to, subject, body})
	return nil
}

// setupActions builds a fixture and registers the action handlers.
// mutate may adjust Deps (e.g. set a webhook secret) before registration.
func setupActions(t *testing.T, mutate func(*actions.Deps)) (*fixture.Fixture, *fakeMailer) {
	t.Helper()
	f := fixture.Setup(t)
	m := &fakeMailer{}
	d := actions.Deps{Config: f.Config, Subs: f.Subs, Defs: f.Defs, Auth: f.Auth, Mailer: m, Pool: f.Pool}
	if mutate != nil {
		mutate(&d)
	}
	actions.Register(f.Queue, d)
	return f, m
}

func enqueueAction(t *testing.T, f *fixture.Fixture, sub submissions.Submission, trigger string, a definition.Action) jobs.Job {
	t.Helper()
	p := actions.Payload{SubmissionID: sub.ID, Trigger: trigger, Action: a}
	if err := actions.Enqueue(context.Background(), f.Queue, f.Pool, f.OrgID, p); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	all := f.Jobs(t)
	return all[len(all)-1]
}

func runOne(t *testing.T, f *fixture.Fixture) {
	t.Helper()
	if p, err := f.Queue.RunOnce(context.Background()); !p || err != nil {
		t.Fatalf("RunOnce = %v, %v", p, err)
	}
}

func jobByID(t *testing.T, f *fixture.Fixture, id int64) jobs.Job {
	t.Helper()
	for _, j := range f.Jobs(t) {
		if j.ID == id {
			return j
		}
	}
	t.Fatalf("job %d missing", id)
	return jobs.Job{}
}

func eventsOfType(t *testing.T, f *fixture.Fixture, id uuid.UUID, typ string) []submissions.Event {
	t.Helper()
	var out []submissions.Event
	for _, e := range f.Events(t, id) {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

package app

import (
	"context"
	"testing"
	"time"

	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

func TestWorkerProcessesOnSubmitActions(t *testing.T) {
	f := fixture.Setup(t)
	cfg := f.Config
	cfg.SMTPHost = "" // log mailer
	cfg.WorkerConcurrency = 2
	a := &App{Config: cfg, Pool: f.Pool, Auth: f.Auth, Defs: f.Defs, Subs: f.Subs, OrgID: f.OrgID}
	a.wireWorkflow()
	if a.Queue == nil || a.Engine == nil {
		t.Fatal("wireWorkflow must set Queue and Engine")
	}
	a.Queue.SetPollInterval(20 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	wait := a.runWorker(ctx)
	defer func() { cancel(); wait() }()

	sub := f.Applicant(t) // created hook → onSubmit email job

	deadline := time.Now().Add(10 * time.Second)
	for {
		done, err := a.Queue.List(context.Background(), f.OrgID, jobs.StatusDone, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(done) == 1 {
			break
		}
		if time.Now().After(deadline) {
			all, _ := a.Queue.List(context.Background(), f.OrgID, "", 10)
			t.Fatalf("job not processed: %+v", all)
		}
		time.Sleep(20 * time.Millisecond)
	}
	var succeeded bool
	for _, ev := range f.Events(t, sub.ID) {
		if ev.Type == "action_succeeded" {
			succeeded = true
		}
	}
	if !succeeded {
		t.Fatal("expected an action_succeeded event")
	}
}

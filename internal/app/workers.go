package app

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/workflow"
)

// wireWorkflow builds the job queue and workflow engine, hooks onSubmit
// actions into submission creation and registers the action handlers.
// Requires a.Config, a.Pool, a.Auth, a.Defs and a.Subs.
func (a *App) wireWorkflow() {
	a.Queue = jobs.NewQueue(a.Pool)
	a.Engine = workflow.NewEngine(a.Pool, a.Defs, a.Subs, a.Auth, a.Queue)
	a.Subs.SetCreatedHook(a.Engine.OnCreated)
	actions.Register(a.Queue, actions.Deps{
		Config:     a.Config,
		Subs:       a.Subs,
		Defs:       a.Defs,
		Auth:       a.Auth,
		Mailer:     actions.NewMailer(a.Config),
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
		Pool:       a.Pool,
	})
}

// runWorker starts the job worker; the returned func blocks until it stops.
func (a *App) runWorker(ctx context.Context) (wait func()) {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.Queue.Run(ctx, a.Config.WorkerConcurrency)
	}()
	return wg.Wait
}

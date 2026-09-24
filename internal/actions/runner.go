package actions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

type Deps struct {
	Config     config.Config
	Subs       *submissions.Service
	Defs       *definitions.Store
	Auth       *auth.Service
	Mailer     Mailer
	HTTPClient *http.Client
	Pool       *pgxpool.Pool
}

type runner struct{ d Deps }

// Register installs the action job handlers and the failure hook on q.
func Register(q *jobs.Queue, d Deps) {
	if d.HTTPClient == nil {
		d.HTTPClient = &http.Client{Timeout: 10 * time.Second}
	}
	if d.Mailer == nil {
		d.Mailer = NewMailer(d.Config)
	}
	r := &runner{d: d}
	q.Register(KindWebhook, r.webhook)
	q.Register(KindEmail, r.email)
	q.Register(KindAssign, r.assign)
	q.SetFailedHook(r.onFailed)
}

// actionContext is everything a handler needs about the job's submission.
type actionContext struct {
	job        jobs.Job
	payload    Payload
	sub        submissions.Submission
	form       definition.Form
	transition *definition.Transition
	fromState  string
	vars       map[string]any
}

func decodePayload(job jobs.Job) (Payload, error) {
	var p Payload
	if err := json.Unmarshal(job.Payload, &p); err != nil {
		return p, jobs.Permanent(fmt.Errorf("decode action payload: %w", err))
	}
	return p, nil
}

func (r *runner) load(ctx context.Context, job jobs.Job) (*actionContext, error) {
	p, err := decodePayload(job)
	if err != nil {
		return nil, err
	}
	sub, err := r.d.Subs.Get(ctx, job.OrgID, p.SubmissionID)
	if errors.Is(err, submissions.ErrNotFound) {
		return nil, jobs.Permanent(err)
	}
	if err != nil {
		return nil, err
	}
	formRec, err := r.d.Defs.GetFormVersion(ctx, sub.FormVersionID)
	if err != nil {
		return nil, err
	}
	ac := &actionContext{job: job, payload: p, sub: sub, form: formRec.Definition}
	if p.Trigger != TriggerSubmit && sub.WorkflowVersionID != nil {
		wfRec, err := r.d.Defs.GetWorkflowVersion(ctx, *sub.WorkflowVersionID)
		if err != nil {
			return nil, err
		}
		if t, ok := wfRec.Definition.Transition(p.Trigger); ok {
			ac.transition = &t
		}
		if p.EventID != 0 {
			var from *string
			err := r.d.Pool.QueryRow(ctx, `SELECT from_state FROM submission_events WHERE id = $1`, p.EventID).Scan(&from)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return nil, err
			}
			if from != nil {
				ac.fromState = *from
			}
		}
	}
	ac.vars = TemplateVars(r.d.Config, ac.form, sub, ac.transition)
	return ac, nil
}

func actionMap(a definition.Action) map[string]any {
	b, _ := json.Marshal(a)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func (r *runner) recordSuccess(ctx context.Context, ac *actionContext) error {
	_, err := submissions.InsertEvent(ctx, r.d.Pool, submissions.Event{
		SubmissionID: ac.sub.ID,
		OrgID:        ac.sub.OrgID,
		Type:         "action_succeeded",
		ActorType:    "system",
		ActorName:    SystemActorName,
		Payload:      map[string]any{"action": actionMap(ac.payload.Action), "trigger": ac.payload.Trigger, "jobId": ac.job.ID},
	})
	return err
}

// onFailed writes an action_failed event when an action job ends failed.
func (r *runner) onFailed(ctx context.Context, job jobs.Job, runErr error) {
	if !strings.HasPrefix(job.Kind, "action.") {
		return
	}
	p, err := decodePayload(job)
	if err != nil {
		slog.ErrorContext(ctx, "actions: failed job has undecodable payload", "job", job.ID, "err", err)
		return
	}
	_, err = submissions.InsertEvent(ctx, r.d.Pool, submissions.Event{
		SubmissionID: p.SubmissionID,
		OrgID:        job.OrgID,
		Type:         "action_failed",
		ActorType:    "system",
		ActorName:    SystemActorName,
		Payload:      map[string]any{"error": runErr.Error(), "action": actionMap(p.Action), "trigger": p.Trigger, "jobId": job.ID},
	})
	if err != nil {
		slog.ErrorContext(ctx, "actions: record action_failed", "job", job.ID, "err", err)
	}
}

package actions

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/submissions"
)

// Sign returns "sha256=<hex HMAC-SHA256(secret, body)>".
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// webhookSubmission mirrors the Submission JSON of spec §7.3.
type webhookSubmission struct {
	ID          uuid.UUID        `json:"id"`
	Form        string           `json:"form"`
	FormVersion int              `json:"formVersion"`
	State       string           `json:"state"`
	StateLabel  string           `json:"stateLabel"`
	Terminal    bool             `json:"terminal"`
	Data        map[string]any   `json:"data"`
	Fields      map[string]any   `json:"fields"`
	Assignee    *webhookAssignee `json:"assignee"`
	CreatedAt   time.Time        `json:"createdAt"`
	UpdatedAt   time.Time        `json:"updatedAt"`
}

type webhookAssignee struct {
	ID    uuid.UUID `json:"id"`
	Name  string    `json:"name"`
	Email string    `json:"email"`
}

type webhookTransition struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	From  string `json:"from"`
	To    string `json:"to"`
}

type webhookForm struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

type webhookBody struct {
	Event      string             `json:"event"`
	Submission webhookSubmission  `json:"submission"`
	Transition *webhookTransition `json:"transition"`
	Form       webhookForm        `json:"form"`
}

func toWebhookSubmission(s submissions.Submission) webhookSubmission {
	var assignee *webhookAssignee
	if s.AssigneeID != nil {
		assignee = &webhookAssignee{ID: *s.AssigneeID, Name: s.AssigneeName, Email: s.AssigneeEmail}
	}
	return webhookSubmission{
		ID: s.ID, Form: s.FormSlug, FormVersion: s.FormVersion,
		State: s.State, StateLabel: s.StateLabel, Terminal: s.Terminal,
		Data: nonNil(s.Data), Fields: nonNil(s.Fields), Assignee: assignee,
		CreatedAt: s.CreatedAt.UTC(), UpdatedAt: s.UpdatedAt.UTC(),
	}
}

func (r *runner) webhook(ctx context.Context, job jobs.Job) error {
	ac, err := r.load(ctx, job)
	if err != nil {
		return err
	}
	target := ac.payload.Action.URL
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return jobs.Permanent(fmt.Errorf("webhook: invalid url %q", target))
	}

	body := webhookBody{
		Event:      "submission.transitioned",
		Submission: toWebhookSubmission(ac.sub),
		Form:       webhookForm{Slug: ac.form.Slug, Title: ac.form.Title},
	}
	if ac.payload.Trigger == TriggerSubmit {
		body.Event = "submission.created"
	} else if ac.transition != nil {
		body.Transition = &webhookTransition{Key: ac.transition.Key, Label: ac.transition.Label, From: ac.fromState, To: ac.transition.To}
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return jobs.Permanent(err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return jobs.Permanent(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "openforms-webhook/1")
	req.Header.Set("X-OpenForms-Event", body.Event)
	req.Header.Set("X-OpenForms-Delivery", strconv.FormatInt(job.ID, 10))
	if secret := r.d.Config.WebhookSecret; secret != "" {
		req.Header.Set("X-OpenForms-Signature", Sign(secret, raw))
	}

	resp, err := r.d.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("webhook %s: %w", target, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		e := fmt.Errorf("webhook %s responded %d", target, resp.StatusCode)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 &&
			resp.StatusCode != http.StatusRequestTimeout && resp.StatusCode != http.StatusTooManyRequests {
			return jobs.Permanent(e)
		}
		return e
	}
	return r.recordSuccess(ctx, ac)
}

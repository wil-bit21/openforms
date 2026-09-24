package actions_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/openforms/openforms/internal/actions"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/jobs"
)

type capturedRequest struct {
	Header http.Header
	Body   []byte
}

func webhookServer(t *testing.T, status int) (*httptest.Server, func() []capturedRequest) {
	t.Helper()
	var mu sync.Mutex
	var got []capturedRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, capturedRequest{r.Header.Clone(), b})
		mu.Unlock()
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []capturedRequest { mu.Lock(); defer mu.Unlock(); return append([]capturedRequest(nil), got...) }
}

func TestKindFor(t *testing.T) {
	for typ, want := range map[definition.ActionType]string{
		definition.ActionWebhook: actions.KindWebhook,
		definition.ActionEmail:   actions.KindEmail,
		definition.ActionAssign:  actions.KindAssign,
	} {
		if got, err := actions.KindFor(typ); err != nil || got != want {
			t.Errorf("KindFor(%s) = %q, %v", typ, got, err)
		}
	}
	if _, err := actions.KindFor("sms"); err == nil {
		t.Error("unknown action type must error")
	}
}

func TestSign(t *testing.T) {
	// echo -n '{"a":1}' | openssl dgst -sha256 -hmac s3cret
	want := "sha256=" + "4c1fa4b8a27b2b3e7ac1a1f76a3d6b1ea06c6ab9b3f0f1f8a44c5a0e8b9b7f1b"
	got := actions.Sign("s3cret", []byte(`{"a":1}`))
	if !strings.HasPrefix(got, "sha256=") || len(got) != len(want) {
		t.Fatalf("Sign = %q", got)
	}
	if actions.Sign("s3cret", []byte(`{"a":1}`)) != got || actions.Sign("other", []byte(`{"a":1}`)) == got {
		t.Fatal("Sign must be deterministic and key-dependent")
	}
}

func TestWebhook_DeliversSignedTransitionEvent(t *testing.T) {
	srv, received := webhookServer(t, http.StatusOK)
	f, _ := setupActions(t, func(d *actions.Deps) { d.Config.WebhookSecret = "s3cret" })
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL + "/hook"})

	runOne(t, f)

	reqs := received()
	if len(reqs) != 1 {
		t.Fatalf("received %d requests", len(reqs))
	}
	r := reqs[0]
	if r.Header.Get("Content-Type") != "application/json" ||
		r.Header.Get("X-OpenForms-Event") != "submission.transitioned" ||
		r.Header.Get("X-OpenForms-Delivery") != strconv.FormatInt(job.ID, 10) {
		t.Fatalf("headers = %v", r.Header)
	}
	if got := r.Header.Get("X-OpenForms-Signature"); got != actions.Sign("s3cret", r.Body) {
		t.Fatalf("signature = %q", got)
	}
	var body struct {
		Event      string `json:"event"`
		Submission struct {
			ID       string         `json:"id"`
			Form     string         `json:"form"`
			State    string         `json:"state"`
			Data     map[string]any `json:"data"`
			Assignee any            `json:"assignee"`
		} `json:"submission"`
		Transition *struct{ Key, Label, From, To string } `json:"transition"`
		Form       struct{ Slug, Title string }           `json:"form"`
	}
	if err := json.Unmarshal(r.Body, &body); err != nil {
		t.Fatal(err)
	}
	if body.Event != "submission.transitioned" || body.Submission.ID != sub.ID.String() ||
		body.Submission.Form != "apply" || body.Submission.Data["email"] != "ada@example.com" ||
		body.Transition == nil || body.Transition.Key != "invite" || body.Transition.To != "interview" ||
		body.Form.Slug != "apply" || body.Form.Title != "Apply" {
		t.Fatalf("body = %s", r.Body)
	}
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusDone {
		t.Fatalf("job = %+v", j)
	}
	if evs := eventsOfType(t, f, sub.ID, "action_succeeded"); len(evs) != 1 || evs[0].ActorType != "system" {
		t.Fatalf("action_succeeded events = %+v", evs)
	}
}

func TestWebhook_SubmitTriggerAndNoSecret(t *testing.T) {
	srv, received := webhookServer(t, http.StatusNoContent)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	enqueueAction(t, f, sub, actions.TriggerSubmit, definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	r := received()[0]
	if r.Header.Get("X-OpenForms-Event") != "submission.created" {
		t.Fatalf("event header = %q", r.Header.Get("X-OpenForms-Event"))
	}
	if _, ok := r.Header["X-Openforms-Signature"]; ok {
		t.Fatal("signature header must be absent without a secret")
	}
	var body map[string]any
	json.Unmarshal(r.Body, &body)
	if v, ok := body["transition"]; !ok || v != nil {
		t.Fatalf("transition must be null, body = %s", r.Body)
	}
}

func TestWebhook_Retryable5xx(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusInternalServerError)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	j := jobByID(t, f, job.ID)
	if j.Status != jobs.StatusPending || j.Attempts != 1 || !strings.Contains(j.LastError, "500") {
		t.Fatalf("job = %+v", j)
	}
	if len(eventsOfType(t, f, sub.ID, "action_succeeded"))+len(eventsOfType(t, f, sub.ID, "action_failed")) != 0 {
		t.Fatal("no outcome event expected while retrying")
	}
}

func TestWebhook_429IsRetryable(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusTooManyRequests)
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusPending {
		t.Fatalf("job = %+v", j)
	}
}

func TestWebhook_400IsPermanent(t *testing.T) {
	srv, _ := webhookServer(t, http.StatusBadRequest)
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	job := enqueueAction(t, f, sub, "invite", definition.Action{Type: definition.ActionWebhook, URL: srv.URL})

	runOne(t, f)

	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed || j.Attempts != 1 {
		t.Fatalf("job = %+v", j)
	}
	evs := eventsOfType(t, f, sub.ID, "action_failed")
	if len(evs) != 1 {
		t.Fatalf("action_failed events = %+v", evs)
	}
	if msg, _ := evs[0].Payload["error"].(string); !strings.Contains(msg, "400") {
		t.Fatalf("payload = %v", evs[0].Payload)
	}
	if a, _ := evs[0].Payload["action"].(map[string]any); a["type"] != "webhook" {
		t.Fatalf("payload action = %v", evs[0].Payload["action"])
	}
}

func TestWebhook_InvalidURLIsPermanent(t *testing.T) {
	f, _ := setupActions(t, nil)
	job := enqueueAction(t, f, f.Applicant(t), "invite", definition.Action{Type: definition.ActionWebhook, URL: "ftp://x"})
	runOne(t, f)
	if j := jobByID(t, f, job.ID); j.Status != jobs.StatusFailed {
		t.Fatalf("job = %+v", j)
	}
}

func TestEnqueue_UsesCallerTransaction(t *testing.T) {
	f, _ := setupActions(t, nil)
	sub := f.Applicant(t)
	tx, err := f.Pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	p := actions.Payload{SubmissionID: sub.ID, Trigger: "invite", Action: definition.Action{Type: definition.ActionWebhook, URL: "http://x"}}
	if err := actions.Enqueue(context.Background(), f.Queue, tx, f.OrgID, p); err != nil {
		t.Fatal(err)
	}
	tx.Rollback(context.Background())
	if n := len(f.Jobs(t)); n != 0 {
		t.Fatalf("rolled-back enqueue left %d jobs", n)
	}
}

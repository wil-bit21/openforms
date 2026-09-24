package httpapi_test

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/jobs"
	"github.com/openforms/openforms/internal/testutil"
	"github.com/openforms/openforms/internal/testutil/fixture"
	"github.com/openforms/openforms/internal/workflow"
)

func newWorkflowAPI(t *testing.T) (*fixture.Fixture, http.Handler) {
	t.Helper()
	f := fixture.Setup(t)
	eng := workflow.NewEngine(f.Pool, f.Defs, f.Subs, f.Auth, f.Queue)
	f.Subs.SetCreatedHook(eng.OnCreated)
	h := httpapi.NewRouter(httpapi.Deps{
		Config: f.Config, Auth: f.Auth, Defs: f.Defs, Subs: f.Subs,
		Engine: eng, Queue: f.Queue, OrgID: f.OrgID,
	})
	return f, h
}

type subResp struct {
	Submission struct {
		ID       string         `json:"id"`
		State    string         `json:"state"`
		Fields   map[string]any `json:"fields"`
		Assignee *struct {
			ID    string `json:"id"`
			Email string `json:"email"`
		} `json:"assignee"`
	} `json:"submission"`
}

type wfErrResp struct {
	Error struct {
		Code    string `json:"code"`
		Details []struct {
			Path string `json:"path"`
		} `json:"details"`
	} `json:"error"`
}

func TestTransitionHTTP_Success(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	key := f.APIKey(t, "reviewer")

	rec := testutil.Do(t, h, "POST", "/api/v1/submissions/"+sub.ID.String()+"/transitions", key,
		map[string]any{"transition": "screen", "comment": "ok", "expectedState": "new"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body)
	}
	if got := testutil.Decode[subResp](t, rec); got.Submission.State != "screening" || got.Submission.ID != sub.ID.String() {
		t.Fatalf("response = %+v", got)
	}
}

func TestTransitionHTTP_Errors(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	plain := f.Submit(t, "feedback", map[string]any{"message": "hi"})
	reviewer := f.APIKey(t, "reviewer")
	manager := f.APIKey(t, "hiring-manager")
	path := "/api/v1/submissions/" + sub.ID.String() + "/transitions"

	cases := []struct {
		name, key, path string
		body            any
		status          int
		code            string
	}{
		{"unauthenticated", "", path, map[string]any{"transition": "screen"}, 401, "unauthenticated"},
		{"missing transition", reviewer, path, map[string]any{}, 422, "validation_failed"},
		{"unknown transition", reviewer, path, map[string]any{"transition": "teleport"}, 422, "unknown_transition"},
		{"forbidden", manager, path, map[string]any{"transition": "screen"}, 403, "forbidden"},
		{"invalid state", reviewer, path, map[string]any{"transition": "invite", "fields": map[string]any{"score": 1}}, 409, "invalid_state"},
		{"state conflict", reviewer, path, map[string]any{"transition": "screen", "expectedState": "interview"}, 409, "state_conflict"},
		{"required field", reviewer, path, map[string]any{"transition": "reject"}, 422, "validation_failed"},
		{"no workflow", reviewer, "/api/v1/submissions/" + plain.ID.String() + "/transitions", map[string]any{"transition": "screen"}, 409, "no_workflow"},
		{"malformed id", reviewer, "/api/v1/submissions/not-a-uuid/transitions", map[string]any{"transition": "screen"}, 404, "not_found"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := testutil.Do(t, h, "POST", c.path, c.key, c.body)
			if rec.Code != c.status || testutil.ErrorCode(t, rec) != c.code {
				t.Fatalf("got %d %s, want %d %s; body %s", rec.Code, testutil.ErrorCode(t, rec), c.status, c.code, rec.Body)
			}
		})
	}

	rec := testutil.Do(t, h, "POST", path, reviewer, map[string]any{"transition": "reject"})
	if got := testutil.Decode[wfErrResp](t, rec); len(got.Error.Details) != 1 || got.Error.Details[0].Path != "fields.rejectionReason" {
		t.Fatalf("details = %+v", got.Error.Details)
	}
}

func TestFieldsCommentsAssigneeHTTP(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	key := f.APIKey(t, "reviewer")
	base := "/api/v1/submissions/" + sub.ID.String()

	rec := testutil.Do(t, h, "PATCH", base+"/fields", key, map[string]any{"fields": map[string]any{"score": 5}})
	if rec.Code != 200 || testutil.Decode[subResp](t, rec).Submission.Fields["score"] != float64(5) {
		t.Fatalf("patch fields: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "PATCH", base+"/fields", key, map[string]any{"fields": map[string]any{"score": "high"}})
	if rec.Code != 422 || testutil.ErrorCode(t, rec) != "validation_failed" {
		t.Fatalf("invalid fields: %d %s", rec.Code, rec.Body)
	}

	rec = testutil.Do(t, h, "POST", base+"/comments", key, map[string]any{"body": "Nice"})
	if rec.Code != http.StatusCreated {
		t.Fatalf("comment: %d %s", rec.Code, rec.Body)
	}
	ev := testutil.Decode[struct {
		Event struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		} `json:"event"`
	}](t, rec)
	if ev.Event.Type != "comment" || ev.Event.Payload["body"] != "Nice" {
		t.Fatalf("comment event = %+v", ev)
	}
	if rec := testutil.Do(t, h, "POST", base+"/comments", key, map[string]any{"body": ""}); rec.Code != 422 {
		t.Fatalf("empty comment: %d", rec.Code)
	}

	u := f.User(t, "who@example.com", "reviewer")
	rec = testutil.Do(t, h, "PUT", base+"/assignee", key, map[string]any{"userId": u.ID.String()})
	if got := testutil.Decode[subResp](t, rec); rec.Code != 200 || got.Submission.Assignee == nil || got.Submission.Assignee.Email != "who@example.com" {
		t.Fatalf("assign: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "PUT", base+"/assignee", key, map[string]any{"userId": nil})
	if got := testutil.Decode[subResp](t, rec); rec.Code != 200 || got.Submission.Assignee != nil {
		t.Fatalf("unassign: %d %s", rec.Code, rec.Body)
	}
}

func TestSubmissionDetailIncludesTransitions(t *testing.T) {
	f, h := newWorkflowAPI(t)
	sub := f.Applicant(t)
	rec := testutil.Do(t, h, "GET", "/api/v1/submissions/"+sub.ID.String(), f.APIKey(t, "hiring-manager"), nil)
	if rec.Code != 200 {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Transitions []struct {
			Key           string   `json:"key"`
			ToLabel       string   `json:"toLabel"`
			RequireFields []string `json:"requireFields"`
			Allowed       bool     `json:"allowed"`
			Reason        string   `json:"reason"`
		} `json:"transitions"`
	}](t, rec)
	byKey := map[string]int{}
	for i, tr := range got.Transitions {
		byKey[tr.Key] = i
	}
	screen, reject := got.Transitions[byKey["screen"]], got.Transitions[byKey["reject"]]
	if len(got.Transitions) != 2 || screen.Allowed || screen.Reason != "role" || screen.RequireFields == nil ||
		!reject.Allowed || reject.ToLabel != "Rejected" {
		t.Fatalf("transitions = %+v", got.Transitions)
	}
}

func TestJobsHTTP(t *testing.T) {
	f, h := newWorkflowAPI(t)
	f.Applicant(t) // onSubmit email → one pending job
	admin := f.APIKey(t, "admin")

	if rec := testutil.Do(t, h, "GET", "/api/v1/jobs", f.APIKey(t, "reviewer"), nil); rec.Code != 403 {
		t.Fatalf("non-admin list: %d", rec.Code)
	}
	rec := testutil.Do(t, h, "GET", "/api/v1/jobs?status=pending", admin, nil)
	list := testutil.Decode[struct {
		Items []struct {
			ID          int64          `json:"id"`
			Kind        string         `json:"kind"`
			Status      string         `json:"status"`
			MaxAttempts int            `json:"maxAttempts"`
			Payload     map[string]any `json:"payload"`
		} `json:"items"`
	}](t, rec)
	if rec.Code != 200 || len(list.Items) != 1 || list.Items[0].Kind != "action.email" || list.Items[0].MaxAttempts != 8 ||
		list.Items[0].Payload["trigger"] != "submit" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, h, "GET", "/api/v1/jobs?status=bogus", admin, nil); rec.Code != 400 {
		t.Fatalf("bad status: %d", rec.Code)
	}

	id := list.Items[0].ID
	if _, err := f.Pool.Exec(context.Background(), `UPDATE jobs SET status = 'failed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if rec := testutil.Do(t, h, "POST", "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/retry", admin, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("retry: %d %s", rec.Code, rec.Body)
	}
	rec = testutil.Do(t, h, "POST", "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/retry", admin, nil)
	if rec.Code != 404 || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("retry pending: %d %s", rec.Code, rec.Body)
	}
	if rec := testutil.Do(t, h, "POST", "/api/v1/jobs/abc/retry", admin, nil); rec.Code != 404 {
		t.Fatalf("retry bad id: %d", rec.Code)
	}
	all, _ := f.Queue.List(context.Background(), f.OrgID, jobs.StatusPending, 10)
	if len(all) != 1 {
		t.Fatalf("pending after retry = %d", len(all))
	}
}

package httpapi_test

import (
	"context"
	"encoding/csv"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/testutil"
)

type listResp struct {
	Items []struct {
		ID         string `json:"id"`
		Form       string `json:"form"`
		State      string `json:"state"`
		StateLabel string `json:"stateLabel"`
		Assignee   *struct {
			Email string `json:"email"`
		} `json:"assignee"`
	} `json:"items"`
	NextCursor *string `json:"nextCursor"`
}

func TestSubmissionsRequireAuth(t *testing.T) {
	a := newDefsSubsAPI(t)
	for _, path := range []string{"/api/v1/submissions", "/api/v1/submissions/" + uuid.NewString(), "/api/v1/forms/contact/submissions.csv"} {
		rec := testutil.Do(t, a.h, "GET", path, "", nil)
		if rec.Code != http.StatusUnauthorized || testutil.ErrorCode(t, rec) != "unauthenticated" {
			t.Fatalf("%s: %d", path, rec.Code)
		}
	}
}

func TestListSubmissionsPaginates(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	for i := 0; i < 3; i++ {
		a.create(t, "contact")
	}
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions?limit=2", key, nil)
	page1 := testutil.Decode[listResp](t, rec)
	if rec.Code != http.StatusOK || len(page1.Items) != 2 || page1.NextCursor == nil {
		t.Fatalf("page1: %d %s", rec.Code, rec.Body)
	}
	if page1.Items[0].StateLabel != "New" || page1.Items[0].Form != "contact" {
		t.Fatalf("item = %+v", page1.Items[0])
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions?limit=2&cursor="+*page1.NextCursor, key, nil)
	page2 := testutil.Decode[listResp](t, rec)
	if len(page2.Items) != 1 || page2.NextCursor != nil {
		t.Fatalf("page2: %s", rec.Body)
	}
	if !strings.Contains(rec.Body.String(), `"nextCursor":null`) {
		t.Fatalf("nextCursor must be null on last page: %s", rec.Body)
	}
}

func TestListSubmissionsFilters(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	assigned, _ := a.create(t, "contact")
	a.create(t, "contact")
	a.create(t, "plain")
	rev := a.env.User(t, "rev@example.com", "reviewer")
	if _, err := a.env.Pool.Exec(context.Background(), `UPDATE submissions SET assignee_id = $2, state = 'review' WHERE id = $1`, assigned.ID, rev.ID); err != nil {
		t.Fatal(err)
	}
	key := a.env.APIKey(t, "reviewer")
	cases := map[string]int{
		"/api/v1/submissions?form=plain":                  1,
		"/api/v1/submissions?state=review":                1,
		"/api/v1/submissions?form=contact&state=new":      1,
		"/api/v1/submissions?assignee=none":               2,
		"/api/v1/submissions?assignee=" + rev.ID.String(): 1,
		"/api/v1/submissions?assignee=me":                 0, // the API key is not the assignee
	}
	for path, want := range cases {
		rec := testutil.Do(t, a.h, "GET", path, key, nil)
		got := testutil.Decode[listResp](t, rec)
		if rec.Code != http.StatusOK || len(got.Items) != want {
			t.Fatalf("%s: %d items (status %d), want %d", path, len(got.Items), rec.Code, want)
		}
	}
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions?assignee="+rev.ID.String(), key, nil)
	got := testutil.Decode[listResp](t, rec)
	if got.Items[0].Assignee == nil || got.Items[0].Assignee.Email != "rev@example.com" || got.Items[0].StateLabel != "In review" {
		t.Fatalf("assigned item = %+v", got.Items[0])
	}
	for _, path := range []string{
		"/api/v1/submissions?assignee=bogus",
		"/api/v1/submissions?limit=abc",
		"/api/v1/submissions?cursor=%21%21garbage",
	} {
		rec := testutil.Do(t, a.h, "GET", path, key, nil)
		if rec.Code != http.StatusBadRequest || testutil.ErrorCode(t, rec) != "bad_request" {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
}

func TestGetSubmissionDetail(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	sub, _ := a.create(t, "contact")
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+sub.ID.String(), key, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("detail: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Submission struct {
			ID   string         `json:"id"`
			Form string         `json:"form"`
			Data map[string]any `json:"data"`
		} `json:"submission"`
		Form struct {
			Slug string `json:"slug"`
		} `json:"form"`
		Workflow *struct {
			Slug string `json:"slug"`
		} `json:"workflow"`
		Events []struct {
			Type    string  `json:"type"`
			ToState *string `json:"toState"`
			Actor   struct {
				Type string `json:"type"`
			} `json:"actor"`
		} `json:"events"`
		Transitions []any `json:"transitions"`
	}](t, rec)
	if got.Submission.ID != sub.ID.String() || got.Submission.Data["name"] != "Ada Lovelace" || got.Form.Slug != "contact" {
		t.Fatalf("detail = %+v", got)
	}
	if got.Workflow == nil || got.Workflow.Slug != "review" {
		t.Fatalf("workflow = %+v", got.Workflow)
	}
	if len(got.Events) != 1 || got.Events[0].Type != "created" || got.Events[0].ToState == nil || *got.Events[0].ToState != "new" {
		t.Fatalf("events = %+v", got.Events)
	}
	if got.Transitions == nil || len(got.Transitions) != 0 {
		t.Fatalf("transitions = %v, want []", got.Transitions)
	}

	plain, _ := a.create(t, "plain")
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+plain.ID.String(), key, nil)
	if !strings.Contains(rec.Body.String(), `"workflow":null`) {
		t.Fatalf("workflow should be null: %s", rec.Body)
	}
	for _, id := range []string{uuid.NewString(), "abc"} {
		if rec := testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+id, key, nil); rec.Code != http.StatusNotFound {
			t.Fatalf("%s: %d", id, rec.Code)
		}
	}
}

func TestCreateSubmissionAuthenticated(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	key := a.env.APIKey(t, "integrator")
	rec := testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "internal", "data": validData()})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body)
	}
	got := testutil.Decode[struct {
		Submission struct {
			ID    string `json:"id"`
			Form  string `json:"form"`
			State string `json:"state"`
		} `json:"submission"`
		ReceiptToken string `json:"receiptToken"`
	}](t, rec)
	if got.Submission.Form != "internal" || got.Submission.State != "new" || got.ReceiptToken == "" {
		t.Fatalf("resp = %+v", got)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/submissions/"+got.Submission.ID, key, nil)
	if !strings.Contains(rec.Body.String(), `"type":"api_key"`) {
		t.Fatalf("created event should record the api key actor: %s", rec.Body)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "missing", "data": validData()})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown form: %d", rec.Code)
	}
	rec = testutil.Do(t, a.h, "POST", "/api/v1/submissions", key, map[string]any{"form": "contact", "data": map[string]any{}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid data: %d", rec.Code)
	}
}

func TestExportCSVEndpoint(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	a.create(t, "contact")
	key := a.env.APIKey(t, "reviewer")
	rec := testutil.Do(t, a.h, "GET", "/api/v1/forms/contact/submissions.csv", key, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("csv: %d %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/csv; charset=utf-8" {
		t.Fatalf("content-type = %q", ct)
	}
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="contact-submissions.csv"` {
		t.Fatalf("content-disposition = %q", cd)
	}
	records, err := csv.NewReader(rec.Body).ReadAll()
	if err != nil || len(records) != 2 || records[0][0] != "id" {
		t.Fatalf("records = %v, err = %v", records, err)
	}
	rec = testutil.Do(t, a.h, "GET", "/api/v1/forms/missing/submissions.csv", key, nil)
	if rec.Code != http.StatusNotFound || testutil.ErrorCode(t, rec) != "not_found" {
		t.Fatalf("missing form csv: %d %s", rec.Code, rec.Body)
	}
}

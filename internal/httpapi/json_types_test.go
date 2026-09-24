package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
)

var (
	jsonTestTime = time.Date(2026, 9, 23, 11, 0, 0, 0, time.FixedZone("CEST", 2*3600))
	jsonTestID   = uuid.MustParse("11111111-1111-4111-8111-111111111111")
	jsonTestUser = uuid.MustParse("22222222-2222-4222-8222-222222222222")
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestSubmissionJSONShape(t *testing.T) {
	sub := submissions.Submission{ID: jsonTestID, FormSlug: "contact", FormVersion: 2, State: "new", StateLabel: "New", CreatedAt: jsonTestTime, UpdatedAt: jsonTestTime}
	got := mustJSON(t, toSubmissionJSON(sub))
	want := `{"id":"11111111-1111-4111-8111-111111111111","form":"contact","formVersion":2,"state":"new","stateLabel":"New","terminal":false,"data":{},"fields":{},"assignee":null,"createdAt":"2026-09-23T09:00:00Z","updatedAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
	sub.AssigneeID, sub.AssigneeName, sub.AssigneeEmail = &jsonTestUser, "Rita", "rita@example.com"
	sub.Data = map[string]any{"name": "Ada"}
	got = mustJSON(t, toSubmissionJSON(sub))
	want = `{"id":"11111111-1111-4111-8111-111111111111","form":"contact","formVersion":2,"state":"new","stateLabel":"New","terminal":false,"data":{"name":"Ada"},"fields":{},"assignee":{"id":"22222222-2222-4222-8222-222222222222","name":"Rita","email":"rita@example.com"},"createdAt":"2026-09-23T09:00:00Z","updatedAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestEventJSONShape(t *testing.T) {
	ev := submissions.Event{ID: 7, Type: "created", ToState: "new", ActorType: "respondent", ActorName: "Respondent", CreatedAt: jsonTestTime}
	got := mustJSON(t, toEventJSON(ev))
	want := `{"id":7,"type":"created","fromState":null,"toState":"new","transition":null,"actor":{"type":"respondent","id":null,"name":"Respondent"},"payload":{},"createdAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestVersionJSONShape(t *testing.T) {
	v := definitions.VersionInfo{ID: jsonTestID, Version: 3, Hash: "abc", Source: definitions.SourceCLI, CreatedBy: "dev", CreatedAt: jsonTestTime}
	got := mustJSON(t, toVersionJSON(v))
	want := `{"version":3,"hash":"abc","source":"cli","createdBy":"dev","createdAt":"2026-09-23T09:00:00Z"}`
	if got != want {
		t.Fatalf("got  %s\nwant %s", got, want)
	}
}

func TestListJSONNullCursor(t *testing.T) {
	got := mustJSON(t, newListJSON([]int{}, ""))
	if got != `{"items":[],"nextCursor":null}` {
		t.Fatalf("got %s", got)
	}
	got = mustJSON(t, newListJSON([]int{1}, "abc"))
	if got != `{"items":[1],"nextCursor":"abc"}` {
		t.Fatalf("got %s", got)
	}
}

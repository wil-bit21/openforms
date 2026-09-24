package submissions_test

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/submissions"
)

func TestListPaginatesWithoutDuplicates(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	want := map[uuid.UUID]bool{}
	for i := 0; i < 5; i++ {
		sub, _ := mustCreate(t, s, env, "contact")
		want[sub.ID] = true
	}
	// Force identical timestamps to exercise the id tie-break.
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET created_at = '2026-09-23T10:00:00Z'`); err != nil {
		t.Fatal(err)
	}
	seen := map[uuid.UUID]bool{}
	cursor := ""
	pages := 0
	for {
		items, next, err := s.List(ctx, env.OrgID, submissions.ListFilter{Limit: 2, Cursor: cursor})
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, it := range items {
			if seen[it.ID] {
				t.Fatalf("duplicate %s on page %d", it.ID, pages)
			}
			seen[it.ID] = true
			if it.StateLabel != "New" {
				t.Fatalf("state label not resolved: %+v", it)
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}
	if pages != 3 || len(seen) != len(want) {
		t.Fatalf("pages=%d seen=%d want=%d", pages, len(seen), len(want))
	}
}

func TestListNewestFirst(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	older, _ := mustCreate(t, s, env, "contact")
	newer, _ := mustCreate(t, s, env, "contact")
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET created_at = created_at - interval '1 hour' WHERE id = $1`, older.ID); err != nil {
		t.Fatal(err)
	}
	items, next, err := s.List(ctx, env.OrgID, submissions.ListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != newer.ID || next != "" {
		t.Fatalf("items = %v, next = %q", items, next)
	}
}

func TestListFilters(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	a, _ := mustCreate(t, s, env, "contact")
	b, _ := mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "plain")
	reviewer := env.User(t, "rev@example.com", "reviewer")
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET state = 'review', assignee_id = $2 WHERE id = $1`, a.ID, reviewer.ID); err != nil {
		t.Fatal(err)
	}

	check := func(name string, f submissions.ListFilter, want int) []submissions.Submission {
		t.Helper()
		items, _, err := s.List(ctx, env.OrgID, f)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(items) != want {
			t.Fatalf("%s: got %d items, want %d", name, len(items), want)
		}
		return items
	}
	check("form", submissions.ListFilter{FormSlug: "plain"}, 1)
	byState := check("state", submissions.ListFilter{State: "review"}, 1)
	if byState[0].ID != a.ID || byState[0].StateLabel != "In review" {
		t.Fatalf("state filter returned %+v", byState[0])
	}
	assigned := check("assignee", submissions.ListFilter{AssigneeID: &reviewer.ID}, 1)
	if assigned[0].AssigneeName == "" || assigned[0].AssigneeEmail != "rev@example.com" {
		t.Fatalf("assignee not joined: %+v", assigned[0])
	}
	unassigned := check("unassigned", submissions.ListFilter{Unassigned: true}, 2)
	for _, it := range unassigned {
		if it.ID == a.ID {
			t.Fatal("assigned submission in unassigned filter")
		}
	}
	check("combined", submissions.ListFilter{FormSlug: "contact", State: "new"}, 1)
	_ = b
	check("other org", submissions.ListFilter{}, 3)
	items, _, err := s.List(ctx, env.OtherOrg(t), submissions.ListFilter{})
	if err != nil || len(items) != 0 {
		t.Fatalf("other org sees %d submissions, err = %v", len(items), err)
	}
}

func TestListLimitBounds(t *testing.T) {
	env, _, s := setup(t)
	for i := 0; i < 3; i++ {
		mustCreate(t, s, env, "plain")
	}
	items, next, err := s.List(context.Background(), env.OrgID, submissions.ListFilter{Limit: 1000})
	if err != nil || len(items) != 3 || next != "" {
		t.Fatalf("limit clamp: %d items next=%q err=%v", len(items), next, err)
	}
}

func TestListInvalidCursor(t *testing.T) {
	env, _, s := setup(t)
	for _, c := range []string{"garbage!!", "bm90LWEtY3Vyc29y"} {
		if _, _, err := s.List(context.Background(), env.OrgID, submissions.ListFilter{Cursor: c}); !errors.Is(err, submissions.ErrInvalidCursor) {
			t.Fatalf("cursor %q: err = %v, want ErrInvalidCursor", c, err)
		}
	}
}

func TestEvents(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, _ := mustCreate(t, s, env, "contact")
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{
		SubmissionID: sub.ID, OrgID: env.OrgID, Type: submissions.EventComment,
		ActorType: "system", ActorName: "System", Payload: map[string]any{"body": "hello"},
	}); err != nil {
		t.Fatal(err)
	}
	evs, err := s.Events(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 {
		t.Fatalf("got %d events", len(evs))
	}
	if evs[0].Type != "created" || evs[0].ToState != "new" || evs[0].FromState != "" || evs[0].ActorType != "respondent" {
		t.Fatalf("created event = %+v", evs[0])
	}
	if evs[1].Type != "comment" || evs[1].Payload["body"] != "hello" || evs[1].ToState != "" {
		t.Fatalf("comment event = %+v", evs[1])
	}
	if _, err := s.Events(ctx, env.OtherOrg(t), sub.ID); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("other org err = %v", err)
	}
}

func TestCountByForm(t *testing.T) {
	env, defs, s := setup(t)
	ctx := context.Background()
	mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "contact")
	mustCreate(t, s, env, "plain")
	counts, err := s.CountByForm(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	contact, _ := defs.GetForm(ctx, env.OrgID, "contact")
	plain, _ := defs.GetForm(ctx, env.OrgID, "plain")
	if counts[contact.ID] != 2 || counts[plain.ID] != 1 {
		t.Fatalf("counts = %v", counts)
	}
}

func TestPublicStatus(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	st, err := s.PublicStatus(ctx, sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if st.ID != sub.ID || st.FormTitle != "Contact" || st.State != "new" || st.StateLabel != "New" || st.Terminal {
		t.Fatalf("status = %+v", st)
	}
	if len(st.States) != 3 || st.States[2].Key != "done" || !st.States[2].Terminal {
		t.Fatalf("states = %+v", st.States)
	}
	if len(st.History) != 1 || st.History[0].State != "new" || st.History[0].Label != "New" {
		t.Fatalf("history = %+v", st.History)
	}
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET state = 'review' WHERE id = $1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{SubmissionID: sub.ID, OrgID: env.OrgID, Type: "transition", FromState: "new", ToState: "review", Transition: "start", ActorType: "system"}); err != nil {
		t.Fatal(err)
	}
	if _, err := submissions.InsertEvent(ctx, env.Pool, submissions.Event{SubmissionID: sub.ID, OrgID: env.OrgID, Type: "comment", ActorType: "system", Payload: map[string]any{"body": "internal note"}}); err != nil {
		t.Fatal(err)
	}
	st, err = s.PublicStatus(ctx, sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if st.StateLabel != "In review" || len(st.History) != 2 || st.History[1].Label != "In review" {
		t.Fatalf("after transition: %+v", st)
	}
}

func TestPublicStatusWithoutWorkflow(t *testing.T) {
	env, _, s := setup(t)
	sub, token := mustCreate(t, s, env, "plain")
	st, err := s.PublicStatus(context.Background(), sub.ID, token)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Terminal || len(st.States) != 1 || st.States[0].Key != "submitted" || st.States[0].Label != "Submitted" {
		t.Fatalf("status = %+v", st)
	}
}

func TestPublicStatusRejectsBadToken(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	for name, tc := range map[string]struct {
		id    uuid.UUID
		token string
	}{
		"wrong token":   {sub.ID, strings.Repeat("A", len(token))},
		"empty token":   {sub.ID, ""},
		"unknown id":    {uuid.New(), token},
		"token as hash": {sub.ID, "x"},
	} {
		if _, err := s.PublicStatus(ctx, tc.id, tc.token); !errors.Is(err, submissions.ErrNotFound) {
			t.Fatalf("%s: err = %v, want ErrNotFound", name, err)
		}
	}
}

func TestExportCSVEscapesFormulas(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	data := validData()
	data["name"] = "=HYPERLINK(\"http://evil\")"
	sub, _, err := s.Create(ctx, env.OrgID, "contact", data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Pool.Exec(ctx, `UPDATE submissions SET fields = '{"note": "+1 looks good"}' WHERE id = $1`, sub.ID); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := s.ExportCSV(ctx, env.OrgID, "contact", &buf); err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(&buf).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	wantHeader := []string{"id", "createdAt", "state", "formVersion", "name", "email", "topic", "fields.note"}
	if strings.Join(records[0], ",") != strings.Join(wantHeader, ",") {
		t.Fatalf("header = %v", records[0])
	}
	if len(records) != 2 {
		t.Fatalf("got %d records", len(records))
	}
	row := records[1]
	if row[0] != sub.ID.String() || row[2] != "new" || row[3] != "1" {
		t.Fatalf("row = %v", row)
	}
	if row[4] != "'=HYPERLINK(\"http://evil\")" {
		t.Fatalf("formula not escaped: %q", row[4])
	}
	if row[7] != "'+1 looks good" {
		t.Fatalf("workflow field not escaped: %q", row[7])
	}
}

func TestExportCSVUnknownFormWritesNothing(t *testing.T) {
	env, _, s := setup(t)
	var buf bytes.Buffer
	if err := s.ExportCSV(context.Background(), env.OrgID, "missing", &buf); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %q before failing", buf.String())
	}
}

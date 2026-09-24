package submissions_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil/fixture"
)

func TestSetAssignee(t *testing.T) {
	f := fixture.Setup(t)
	sub := f.Applicant(t)
	u := f.User(t, "rev@example.com", "reviewer")
	ctx := context.Background()

	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, sub.ID, &u.ID); err != nil {
		t.Fatal(err)
	}
	got := f.Reload(t, sub.ID)
	if got.AssigneeID == nil || *got.AssigneeID != u.ID || got.AssigneeEmail != "rev@example.com" {
		t.Fatalf("after assign: %+v", got)
	}
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, sub.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got := f.Reload(t, sub.ID); got.AssigneeID != nil {
		t.Fatalf("after unassign: %+v", got)
	}
	if err := submissions.SetAssignee(ctx, f.Pool, f.OrgID, uuid.New(), nil); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("unknown submission err = %v", err)
	}
}

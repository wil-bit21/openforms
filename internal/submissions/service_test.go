package submissions_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

// setup applies workflow "review" and forms "contact" (public, review),
// "plain" (public, no workflow) and "internal" (private, review).
func setup(t *testing.T) (*testutil.Env, *definitions.Store, *submissions.Service) {
	t.Helper()
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	internal := testutil.SampleForm("internal", "review")
	internal.Settings.Public = false
	if _, err := defs.Apply(context.Background(), env.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", ""), internal},
	}); err != nil {
		t.Fatalf("seed definitions: %v", err)
	}
	return env, defs, submissions.NewService(env.Pool, defs)
}

func validData() map[string]any {
	return map[string]any{"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}
}

func mustCreate(t *testing.T, s *submissions.Service, env *testutil.Env, slug string) (submissions.Submission, string) {
	t.Helper()
	sub, token, err := s.Create(context.Background(), env.OrgID, slug, validData(), true)
	if err != nil {
		t.Fatalf("Create(%s): %v", slug, err)
	}
	return sub, token
}

func TestCreateStartsInInitialState(t *testing.T) {
	env, _, s := setup(t)
	ctx := context.Background()
	sub, token := mustCreate(t, s, env, "contact")
	if sub.State != "new" || sub.StateLabel != "New" || sub.Terminal {
		t.Fatalf("state = %q/%q terminal=%v", sub.State, sub.StateLabel, sub.Terminal)
	}
	if sub.FormSlug != "contact" || sub.FormVersion != 1 || sub.WorkflowVersionID == nil {
		t.Fatalf("unexpected pins %+v", sub)
	}
	if len(token) != 32 {
		t.Fatalf("receipt token length = %d, want 32 (24 bytes base64url)", len(token))
	}
	if !reflect.DeepEqual(sub.Data, validData()) {
		t.Fatalf("data = %v", sub.Data)
	}
	if sub.Fields == nil || len(sub.Fields) != 0 {
		t.Fatalf("fields = %v, want empty map", sub.Fields)
	}

	got, err := s.Get(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "new" || got.StateLabel != "New" || got.FormSlug != "contact" || !got.CreatedAt.Equal(sub.CreatedAt) {
		t.Fatalf("Get = %+v", got)
	}

	var typ, toState, actorType, actorName string
	if err := env.Pool.QueryRow(ctx, `SELECT type, to_state, actor_type, actor_name FROM submission_events WHERE submission_id = $1`, sub.ID).
		Scan(&typ, &toState, &actorType, &actorName); err != nil {
		t.Fatal(err)
	}
	if typ != "created" || toState != "new" || actorType != "respondent" || actorName != "Respondent" {
		t.Fatalf("event = %s %s %s %s", typ, toState, actorType, actorName)
	}
	var stored string
	if err := env.Pool.QueryRow(ctx, `SELECT receipt_token_hash FROM submissions WHERE id = $1`, sub.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token || len(stored) != 64 {
		t.Fatalf("token must be stored as sha256 hex, got %q", stored)
	}
}

func TestCreateStripsUnknownKeys(t *testing.T) {
	env, _, s := setup(t)
	data := validData()
	data["isAdmin"] = true
	sub, _, err := s.Create(context.Background(), env.OrgID, "contact", data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sub.Data["isAdmin"]; ok {
		t.Fatalf("unknown key kept: %v", sub.Data)
	}
}

func TestCreateValidationError(t *testing.T) {
	env, _, s := setup(t)
	data := validData()
	delete(data, "email")
	_, _, err := s.Create(context.Background(), env.OrgID, "contact", data, true)
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
	found := false
	for _, p := range ve.Problems {
		if p.Path == "data.email" {
			found = true
		}
	}
	if !found {
		t.Fatalf("problems = %+v, want data.email", ve.Problems)
	}
}

func TestCreateNilDataIsValidatedAsEmpty(t *testing.T) {
	env, _, s := setup(t)
	_, _, err := s.Create(context.Background(), env.OrgID, "contact", nil, true)
	var ve *definition.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %v, want ValidationError", err)
	}
}

func TestCreateRequirePublic(t *testing.T) {
	env, _, s := setup(t)
	if _, _, err := s.Create(context.Background(), env.OrgID, "internal", validData(), true); !errors.Is(err, submissions.ErrFormNotPublic) {
		t.Fatalf("err = %v, want ErrFormNotPublic", err)
	}
	if _, _, err := s.Create(context.Background(), env.OrgID, "internal", validData(), false); err != nil {
		t.Fatalf("authenticated create on private form: %v", err)
	}
}

func TestCreateUnknownForm(t *testing.T) {
	env, _, s := setup(t)
	if _, _, err := s.Create(context.Background(), env.OrgID, "missing", validData(), true); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestCreateWithoutWorkflow(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "plain")
	if sub.State != "submitted" || sub.StateLabel != "Submitted" || !sub.Terminal || sub.WorkflowVersionID != nil {
		t.Fatalf("sub = %+v", sub)
	}
}

func TestCreateRecordsPrincipalActor(t *testing.T) {
	env, _, s := setup(t)
	ctx := auth.WithPrincipal(context.Background(), auth.Principal{OrgID: env.OrgID, Kind: auth.PrincipalAPIKey, ID: env.OrgID, Name: "ci-key"})
	sub, _, err := s.Create(ctx, env.OrgID, "contact", validData(), false)
	if err != nil {
		t.Fatal(err)
	}
	var actorType, actorName string
	if err := env.Pool.QueryRow(ctx, `SELECT actor_type, actor_name FROM submission_events WHERE submission_id = $1`, sub.ID).Scan(&actorType, &actorName); err != nil {
		t.Fatal(err)
	}
	if actorType != "api_key" || actorName != "ci-key" {
		t.Fatalf("actor = %s %s", actorType, actorName)
	}
}

func TestCreatedHookReceivesWorkflowInTransaction(t *testing.T) {
	env, _, s := setup(t)
	var gotWF *definition.Workflow
	var gotSub submissions.Submission
	s.SetCreatedHook(func(ctx context.Context, tx pgx.Tx, sub submissions.Submission, wf *definition.Workflow) error {
		gotSub, gotWF = sub, wf
		var n int
		// The row is visible inside the hook's transaction.
		return tx.QueryRow(ctx, `SELECT count(*) FROM submissions WHERE id = $1`, sub.ID).Scan(&n)
	})
	sub, _ := mustCreate(t, s, env, "contact")
	if gotWF == nil || gotWF.Slug != "review" || gotSub.ID != sub.ID {
		t.Fatalf("hook got wf=%v sub=%v", gotWF, gotSub.ID)
	}
	gotWF = &definition.Workflow{}
	mustCreate(t, s, env, "plain")
	if gotWF != nil {
		t.Fatalf("hook for form without workflow got wf=%v, want nil", gotWF)
	}
}

func TestCreatedHookErrorRollsBack(t *testing.T) {
	env, _, s := setup(t)
	boom := errors.New("boom")
	s.SetCreatedHook(func(context.Context, pgx.Tx, submissions.Submission, *definition.Workflow) error { return boom })
	if _, _, err := s.Create(context.Background(), env.OrgID, "contact", validData(), true); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want boom", err)
	}
	var n int
	if err := env.Pool.QueryRow(context.Background(), `SELECT count(*) FROM submissions`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("submission persisted despite hook error (%d rows)", n)
	}
}

func TestGetIsOrgScoped(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "contact")
	if _, err := s.Get(context.Background(), env.OtherOrg(t), sub.ID); !errors.Is(err, submissions.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestGetForUpdateInsideTx(t *testing.T) {
	env, _, s := setup(t)
	sub, _ := mustCreate(t, s, env, "contact")
	err := db.WithTx(context.Background(), env.Pool, func(tx pgx.Tx) error {
		got, err := s.GetForUpdate(context.Background(), tx, env.OrgID, sub.ID)
		if err != nil {
			return err
		}
		if got.ID != sub.ID || got.StateLabel != "New" {
			t.Errorf("GetForUpdate = %+v", got)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSubmissionKeepsPinnedVersions(t *testing.T) {
	env, defs, s := setup(t)
	ctx := context.Background()
	sub, _ := mustCreate(t, s, env, "contact")

	wf := testutil.SampleWorkflow("review")
	wf.States[0].Label = "Fresh"
	if _, err := defs.Apply(ctx, env.OrgID, definitions.ApplyInput{Workflows: []definition.Workflow{wf}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, env.OrgID, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.StateLabel != "New" || got.FormVersion != 1 {
		t.Fatalf("pinned label/version changed: %q v%d", got.StateLabel, got.FormVersion)
	}
	newer, _ := mustCreate(t, s, env, "contact")
	if newer.StateLabel != "Fresh" || newer.FormVersion != 2 {
		t.Fatalf("new submission label/version = %q v%d", newer.StateLabel, newer.FormVersion)
	}
}

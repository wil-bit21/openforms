package httpapi_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definition"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

type defsSubsAPI struct {
	env   *testutil.Env
	h     http.Handler
	defs  *definitions.Store
	subs  *submissions.Service
	admin string // plaintext API key with the admin role
}

func newDefsSubsAPI(t *testing.T) *defsSubsAPI {
	t.Helper()
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	subs := submissions.NewService(env.Pool, defs)
	h := httpapi.NewRouter(httpapi.Deps{Config: env.Config, Auth: env.Auth, Defs: defs, Subs: subs, OrgID: env.OrgID})
	return &defsSubsAPI{env: env, h: h, defs: defs, subs: subs, admin: env.APIKey(t, auth.RoleAdmin)}
}

// seed applies workflow "review" and forms "contact" (public, review),
// "plain" (public, no workflow) and "internal" (private, review).
func (a *defsSubsAPI) seed(t *testing.T) {
	t.Helper()
	internal := testutil.SampleForm("internal", "review")
	internal.Settings.Public = false
	if _, err := a.defs.Apply(context.Background(), a.env.OrgID, definitions.ApplyInput{
		Workflows: []definition.Workflow{testutil.SampleWorkflow("review")},
		Forms:     []definition.Form{testutil.SampleForm("contact", "review"), testutil.SampleForm("plain", ""), internal},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func (a *defsSubsAPI) create(t *testing.T, slug string) (submissions.Submission, string) {
	t.Helper()
	sub, token, err := a.subs.Create(context.Background(), a.env.OrgID, slug, validData(), false)
	if err != nil {
		t.Fatalf("create %s: %v", slug, err)
	}
	return sub, token
}

func validData() map[string]any {
	return map[string]any{"name": "Ada Lovelace", "email": "ada@example.com", "topic": "support"}
}

func TestDefsSubsFixtureSeeds(t *testing.T) {
	a := newDefsSubsAPI(t)
	a.seed(t)
	sub, _ := a.create(t, "contact")
	if sub.State != "new" {
		t.Fatalf("state = %q", sub.State)
	}
}

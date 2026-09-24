package seed_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/seed"
	"github.com/openforms/openforms/internal/testutil"
)

func TestDemoSeedsBundleAndUsers(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	defs := definitions.NewStore(env.Pool)

	res, err := seed.Demo(ctx, env.Auth, defs, env.OrgID)
	if err != nil {
		t.Fatalf("Demo: %v", err)
	}
	if len(res.Apply.Items) != 4 {
		t.Fatalf("apply items = %d, want 4: %+v", len(res.Apply.Items), res.Apply.Items)
	}
	for _, it := range res.Apply.Items {
		if !it.Created || it.Version != 1 {
			t.Errorf("item %s/%s: created=%v version=%d, want created v1", it.Kind, it.Slug, it.Created, it.Version)
		}
	}
	if len(res.UsersCreated) != 3 {
		t.Errorf("users created = %v, want 3", res.UsersCreated)
	}
	for _, du := range seed.DemoUsers {
		u, err := env.Auth.GetUserByEmail(ctx, env.OrgID, du.Email)
		if err != nil {
			t.Fatalf("GetUserByEmail(%s): %v", du.Email, err)
		}
		if !reflect.DeepEqual(u.Roles, du.Roles) {
			t.Errorf("%s roles = %v, want %v", du.Email, u.Roles, du.Roles)
		}
	}
	rec, err := defs.GetForm(ctx, env.OrgID, "job-application")
	if err != nil {
		t.Fatalf("GetForm: %v", err)
	}
	if rec.Current.Source != definitions.SourceSeed {
		t.Errorf("source = %q, want seed", rec.Current.Source)
	}
	if _, _, err := env.Auth.Login(ctx, "reviewer@demo.local", seed.DemoPassword); err != nil {
		t.Errorf("reviewer login: %v", err)
	}
}

func TestDemoIsIdempotent(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	defs := definitions.NewStore(env.Pool)

	if _, err := seed.Demo(ctx, env.Auth, defs, env.OrgID); err != nil {
		t.Fatalf("first Demo: %v", err)
	}
	res, err := seed.Demo(ctx, env.Auth, defs, env.OrgID)
	if err != nil {
		t.Fatalf("second Demo: %v", err)
	}
	for _, it := range res.Apply.Items {
		if it.Changed || it.Created {
			t.Errorf("second seed changed %s/%s: %+v", it.Kind, it.Slug, it)
		}
	}
	if len(res.UsersCreated) != 0 {
		t.Errorf("second seed created users %v", res.UsersCreated)
	}
	versions, err := defs.FormVersions(ctx, env.OrgID, "job-application")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 {
		t.Errorf("job-application versions = %d, want 1", len(versions))
	}
	users, err := env.Auth.ListUsers(ctx, env.OrgID)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Errorf("users = %d, want 3", len(users))
	}
}

func TestDemoKeepsExistingUser(t *testing.T) {
	env := testutil.NewEnv(t)
	ctx := context.Background()
	if _, err := env.Auth.CreateUser(ctx, env.OrgID, "reviewer@demo.local", "Existing", "my-own-password", []string{"reviewer"}); err != nil {
		t.Fatal(err)
	}
	res, err := seed.Demo(ctx, env.Auth, definitions.NewStore(env.Pool), env.OrgID)
	if err != nil {
		t.Fatalf("Demo: %v", err)
	}
	for _, e := range res.UsersCreated {
		if e == "reviewer@demo.local" {
			t.Errorf("existing user was re-created")
		}
	}
	if _, _, err := env.Auth.Login(ctx, "reviewer@demo.local", "my-own-password"); err != nil {
		t.Errorf("existing password no longer works: %v", err)
	}
}

func TestEnabledFromEnv(t *testing.T) {
	cases := map[string]bool{"": false, "false": false, "0": false, "true": true, "TRUE": true, "1": true}
	for v, want := range cases {
		t.Setenv("OPENFORMS_SEED_DEMO", v)
		if got := seed.EnabledFromEnv(); got != want {
			t.Errorf("OPENFORMS_SEED_DEMO=%q → %v, want %v", v, got, want)
		}
	}
}

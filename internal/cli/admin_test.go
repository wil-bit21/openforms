package cli

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/testutil"
)

func adminDeps(t *testing.T) (Deps, *testIO, *testutil.Env) {
	t.Helper()
	env := testutil.NewEnv(t)
	d, tio := newTestDeps(t.TempDir(), nil, nil)
	d.OpenAuth = func(context.Context) (*auth.Service, uuid.UUID, func(), error) {
		return env.Auth, env.OrgID, func() {}, nil
	}
	return d, tio, env
}

func TestSplitRoles(t *testing.T) {
	if got := splitRoles(" admin, reviewer ,,"); !reflect.DeepEqual(got, []string{"admin", "reviewer"}) {
		t.Fatalf("got %v", got)
	}
	if got := splitRoles(""); got == nil || len(got) != 0 {
		t.Fatalf("empty must be non-nil empty slice, got %#v", got)
	}
}

func TestAdminCreateUser(t *testing.T) {
	d, tio, env := adminDeps(t)
	err := execute(NewAdminCmd(d), "create-user", "--email", "Ada@Example.com", "--password", "correct-horse", "--roles", "admin,reviewer")
	if err != nil {
		t.Fatalf("create-user: %v", err)
	}
	u, err := env.Auth.GetUserByEmail(context.Background(), env.OrgID, "ada@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if u.Name != "Ada" || !reflect.DeepEqual(u.Roles, []string{"admin", "reviewer"}) {
		t.Fatalf("user = %+v", u)
	}
	want := "Created user ada@example.com (" + u.ID.String() + ") with roles [admin, reviewer]\n"
	if tio.out.String() != want {
		t.Fatalf("out = %q, want %q", tio.out.String(), want)
	}
}

func TestAdminCreateUserDuplicate(t *testing.T) {
	d, _, _ := adminDeps(t)
	args := []string{"create-user", "--email", "bob@example.com", "--password", "password123"}
	if err := execute(NewAdminCmd(d), args...); err != nil {
		t.Fatal(err)
	}
	err := execute(NewAdminCmd(d), args...)
	if err == nil || !strings.Contains(err.Error(), "a user with email bob@example.com already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminCreateUserRequiresFlags(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil) // OpenAuth fails if reached
	err := execute(NewAdminCmd(d), "create-user", "--email", "x@example.com")
	if err == nil || !strings.Contains(err.Error(), "--email and --password are required") {
		t.Fatalf("err = %v", err)
	}
}

func TestAdminCreateAPIKey(t *testing.T) {
	d, tio, env := adminDeps(t)
	if err := execute(NewAdminCmd(d), "create-api-key", "--name", "ci", "--roles", "admin"); err != nil {
		t.Fatal(err)
	}
	var key string
	for _, line := range strings.Split(tio.out.String(), "\n") {
		if strings.HasPrefix(line, "ofk_") {
			key = line
		}
	}
	if len(key) != 44 {
		t.Fatalf("no ofk_ key line (len 44) in output:\n%s", tio.out.String())
	}
	p, err := env.Auth.PrincipalFromAPIKey(context.Background(), key)
	if err != nil || !p.IsAdmin() || p.Name != "ci" {
		t.Fatalf("principal=%+v err=%v", p, err)
	}
	if !strings.Contains(tio.out.String(), "it will not be shown again") {
		t.Fatalf("missing warning:\n%s", tio.out.String())
	}
}

func TestAdminCreateAPIKeyRequiresName(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil)
	err := execute(NewAdminCmd(d), "create-api-key", "--roles", "admin")
	if err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("err = %v", err)
	}
}

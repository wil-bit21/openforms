package auth

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/openforms/openforms/internal/db/dbtest"
	"github.com/openforms/openforms/internal/definition"
)

func TestMain(m *testing.M) {
	bcryptCost = bcrypt.MinCost // keep tests fast; production uses 12
	os.Exit(m.Run())
}

func newTestService(t *testing.T) (*Service, uuid.UUID) {
	t.Helper()
	s := NewService(dbtest.New(t))
	org, err := s.EnsureDefaultOrg(context.Background())
	if err != nil {
		t.Fatalf("EnsureDefaultOrg: %v", err)
	}
	return s, org
}

func problemPaths(t *testing.T, err error) []string {
	t.Helper()
	var verr *definition.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("err = %v, want *definition.ValidationError", err)
	}
	var paths []string
	for _, p := range verr.Problems {
		paths = append(paths, p.Path)
	}
	return paths
}

func TestEnsureDefaultOrgIsIdempotent(t *testing.T) {
	s, org := newTestService(t)
	again, err := s.EnsureDefaultOrg(context.Background())
	if err != nil || again != org {
		t.Fatalf("second call = %v, %v; want %v", again, err, org)
	}
}

func TestCreateUserNormalisesAndRejectsDuplicates(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, err := s.CreateUser(ctx, org, "  Ada@Example.com ", " Ada ", "password123", []string{"reviewer", "admin", "reviewer"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if u.Email != "ada@example.com" || u.Name != "Ada" || u.OrgID != org {
		t.Fatalf("user = %+v", u)
	}
	if len(u.Roles) != 2 || u.Roles[0] != "reviewer" || u.Roles[1] != "admin" {
		t.Fatalf("roles = %v, want [reviewer admin]", u.Roles)
	}
	if _, err := s.CreateUser(ctx, org, "ADA@example.com", "Other", "password123", nil); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate err = %v, want ErrEmailTaken", err)
	}
}

func TestCreateUserValidation(t *testing.T) {
	s, org := newTestService(t)
	_, err := s.CreateUser(context.Background(), org, "not-an-email", " ", "short", []string{"Bad Role"})
	got := problemPaths(t, err)
	want := []string{"email", "name", "password", "roles[0]"}
	if len(got) != len(want) {
		t.Fatalf("paths = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("paths = %v, want %v", got, want)
		}
	}
}

func TestListGetAndUsersWithRole(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	a, _ := s.CreateUser(ctx, org, "a@example.com", "A", "password123", []string{"reviewer"})
	s.CreateUser(ctx, org, "b@example.com", "B", "password123", []string{"hiring-manager"}) //nolint:errcheck
	all, err := s.ListUsers(ctx, org)
	if err != nil || len(all) != 2 {
		t.Fatalf("ListUsers = %d, %v", len(all), err)
	}
	got, err := s.GetUserByEmail(ctx, org, "A@EXAMPLE.COM")
	if err != nil || got.ID != a.ID {
		t.Fatalf("GetUserByEmail = %+v, %v", got, err)
	}
	if _, err := s.GetUserByEmail(ctx, org, "nobody@example.com"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user err = %v", err)
	}
	reviewers, err := s.UsersWithRole(ctx, org, "reviewer")
	if err != nil || len(reviewers) != 1 || reviewers[0].ID != a.ID {
		t.Fatalf("UsersWithRole = %+v, %v", reviewers, err)
	}
}

func TestUpdateUser(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "root@example.com", "Root", "password123", []string{"admin"}) //nolint:errcheck
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", []string{"reviewer"})

	name, pw := "Ada L.", "newpassword1"
	got, err := s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Name: &name, Password: &pw, Roles: []string{"hiring-manager"}})
	if err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}
	if got.Name != "Ada L." || len(got.Roles) != 1 || got.Roles[0] != "hiring-manager" {
		t.Fatalf("updated = %+v", got)
	}
	// nil Roles leaves roles unchanged
	got, err = s.UpdateUser(ctx, org, u.ID, UpdateUserInput{})
	if err != nil || got.Roles[0] != "hiring-manager" || got.Name != "Ada L." {
		t.Fatalf("no-op update = %+v, %v", got, err)
	}
	if _, err := s.UpdateUser(ctx, org, uuid.New(), UpdateUserInput{Name: &name}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing user err = %v", err)
	}
	empty := " "
	if paths := problemPaths(t, mustErr(s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Name: &empty}))); paths[0] != "name" {
		t.Fatalf("paths = %v", paths)
	}
}

func mustErr(_ User, err error) error { return err }

func TestLastAdminCannotBeDemotedOrDeleted(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	root, _ := s.CreateUser(ctx, org, "root@example.com", "Root", "password123", []string{"admin"})

	_, err := s.UpdateUser(ctx, org, root.ID, UpdateUserInput{Roles: []string{"reviewer"}})
	if paths := problemPaths(t, err); paths[0] != "roles" {
		t.Fatalf("demote paths = %v", paths)
	}
	if paths := problemPaths(t, s.DeleteUser(ctx, org, root.ID)); paths[0] != "id" {
		t.Fatalf("delete paths = %v", paths)
	}
	still, _ := s.GetUserByEmail(ctx, org, "root@example.com")
	if len(still.Roles) != 1 || still.Roles[0] != "admin" {
		t.Fatalf("root changed: %+v", still)
	}

	// With a second admin both operations succeed.
	s.CreateUser(ctx, org, "second@example.com", "Second", "password123", []string{"admin"}) //nolint:errcheck
	if _, err := s.UpdateUser(ctx, org, root.ID, UpdateUserInput{Roles: []string{"reviewer"}}); err != nil {
		t.Fatalf("demote with another admin: %v", err)
	}
	if err := s.DeleteUser(ctx, org, root.ID); err != nil {
		t.Fatalf("delete non-admin: %v", err)
	}
	if err := s.DeleteUser(ctx, org, root.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second delete err = %v", err)
	}
}

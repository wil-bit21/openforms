package auth_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/openforms/openforms/internal/auth"
)

func TestHasAnyRole(t *testing.T) {
	reviewer := auth.Principal{Roles: []string{"reviewer"}}
	admin := auth.Principal{Roles: []string{"admin"}}
	none := auth.Principal{}
	cases := []struct {
		name  string
		p     auth.Principal
		roles []string
		want  bool
	}{
		{"empty guard allows anyone", none, nil, true},
		{"matching role", reviewer, []string{"hiring-manager", "reviewer"}, true},
		{"non-matching role", reviewer, []string{"hiring-manager"}, false},
		{"admin satisfies any guard", admin, []string{"hiring-manager"}, true},
		{"no roles vs guard", none, []string{"reviewer"}, false},
	}
	for _, c := range cases {
		if got := c.p.HasAnyRole(c.roles...); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
	if !admin.IsAdmin() || reviewer.IsAdmin() {
		t.Error("IsAdmin wrong")
	}
}

func TestPrincipalContext(t *testing.T) {
	if _, ok := auth.PrincipalFrom(context.Background()); ok {
		t.Fatal("empty context should have no principal")
	}
	p := auth.Principal{ID: uuid.New(), Kind: auth.PrincipalUser, Name: "Ada"}
	got, ok := auth.PrincipalFrom(auth.WithPrincipal(context.Background(), p))
	if !ok || got.ID != p.ID || got.Name != "Ada" {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

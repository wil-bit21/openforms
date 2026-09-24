package auth

import (
	"context"
	"errors"
	"testing"
)

func TestLoginAndSessionLifecycle(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", []string{"reviewer"})

	token, got, err := s.Login(ctx, "ADA@example.com ", "password123")
	if err != nil || got.ID != u.ID || token == "" {
		t.Fatalf("Login = %q, %+v, %v", token, got, err)
	}
	var stored string
	s.pool.QueryRow(ctx, `SELECT token_hash FROM sessions`).Scan(&stored) //nolint:errcheck
	if stored == token || stored != hashToken(token) {
		t.Fatal("session must store sha256 of token, not the token")
	}

	p, err := s.PrincipalFromSession(ctx, token)
	if err != nil {
		t.Fatalf("PrincipalFromSession: %v", err)
	}
	if p.Kind != PrincipalUser || p.ID != u.ID || p.OrgID != org || p.Email != "ada@example.com" || p.Roles[0] != "reviewer" {
		t.Fatalf("principal = %+v", p)
	}

	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("after logout err = %v", err)
	}
	if err := s.Logout(ctx, token); err != nil {
		t.Fatalf("second Logout should be a no-op, got %v", err)
	}
}

func TestLoginFailures(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil) //nolint:errcheck
	if _, _, err := s.Login(ctx, "ada@example.com", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password err = %v", err)
	}
	if _, _, err := s.Login(ctx, "nobody@example.com", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown user err = %v", err)
	}
}

func TestExpiredSessionIsRejected(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil) //nolint:errcheck
	token, _, _ := s.Login(ctx, "ada@example.com", "password123")
	if _, err := s.pool.Exec(ctx, `UPDATE sessions SET expires_at = now() - interval '1 hour'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expired session err = %v", err)
	}
	if _, err := s.PrincipalFromSession(ctx, ""); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("empty token err = %v", err)
	}
}

func TestPasswordChangeRevokesSessions(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()
	u, _ := s.CreateUser(ctx, org, "ada@example.com", "Ada", "password123", nil)
	token, _, _ := s.Login(ctx, "ada@example.com", "password123")
	pw := "brand-new-pass"
	if _, err := s.UpdateUser(ctx, org, u.ID, UpdateUserInput{Password: &pw}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.PrincipalFromSession(ctx, token); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("old session err = %v", err)
	}
	if _, _, err := s.Login(ctx, "ada@example.com", pw); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

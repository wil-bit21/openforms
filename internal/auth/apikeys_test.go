package auth

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/google/uuid"
)

func TestAPIKeyLifecycle(t *testing.T) {
	s, org := newTestService(t)
	ctx := context.Background()

	plaintext, k, err := s.CreateAPIKey(ctx, org, " CI deploy ", []string{"admin"})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if !regexp.MustCompile(`^ofk_[0-9A-Za-z]{40}$`).MatchString(plaintext) {
		t.Fatalf("plaintext %q has wrong format", plaintext)
	}
	if k.Prefix != plaintext[:8] || k.Name != "CI deploy" || k.OrgID != org || k.RevokedAt != nil {
		t.Fatalf("key = %+v", k)
	}

	p, err := s.PrincipalFromAPIKey(ctx, plaintext)
	if err != nil {
		t.Fatalf("PrincipalFromAPIKey: %v", err)
	}
	if p.Kind != PrincipalAPIKey || p.ID != k.ID || p.Name != "CI deploy" || !p.IsAdmin() || p.OrgID != org {
		t.Fatalf("principal = %+v", p)
	}

	keys, err := s.ListAPIKeys(ctx, org)
	if err != nil || len(keys) != 1 || keys[0].LastUsedAt == nil {
		t.Fatalf("ListAPIKeys = %+v, %v (LastUsedAt must be set after use)", keys, err)
	}

	if err := s.RevokeAPIKey(ctx, org, k.ID); err != nil {
		t.Fatalf("RevokeAPIKey: %v", err)
	}
	if _, err := s.PrincipalFromAPIKey(ctx, plaintext); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("revoked key err = %v", err)
	}
	if err := s.RevokeAPIKey(ctx, org, k.ID); err != nil {
		t.Fatalf("second revoke should be idempotent, got %v", err)
	}
	if err := s.RevokeAPIKey(ctx, org, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown key err = %v", err)
	}
}

func TestAPIKeyRejectsGarbage(t *testing.T) {
	s, _ := newTestService(t)
	for _, k := range []string{"", "garbage", "ofk_doesnotexist"} {
		if _, err := s.PrincipalFromAPIKey(context.Background(), k); !errors.Is(err, ErrUnauthenticated) {
			t.Errorf("%q: err = %v", k, err)
		}
	}
}

func TestCreateAPIKeyValidation(t *testing.T) {
	s, org := newTestService(t)
	_, _, err := s.CreateAPIKey(context.Background(), org, "  ", []string{"NOPE"})
	paths := problemPaths(t, err)
	if len(paths) != 2 || paths[0] != "name" || paths[1] != "roles[0]" {
		t.Fatalf("paths = %v", paths)
	}
}

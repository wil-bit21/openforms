// Package auth owns orgs, users, sessions and API keys.
package auth

import (
	"context"
	"slices"

	"github.com/google/uuid"
)

const RoleAdmin = "admin"

type PrincipalKind string

const (
	PrincipalUser   PrincipalKind = "user"
	PrincipalAPIKey PrincipalKind = "api_key"
)

// Principal is the authenticated caller of a request.
type Principal struct {
	OrgID uuid.UUID
	Kind  PrincipalKind
	ID    uuid.UUID
	Name  string
	Email string
	Roles []string
}

func (p Principal) IsAdmin() bool { return slices.Contains(p.Roles, RoleAdmin) }

// HasAnyRole is true if roles is empty, the principal is admin, or any role matches.
func (p Principal) HasAnyRole(roles ...string) bool {
	if len(roles) == 0 || p.IsAdmin() {
		return true
	}
	for _, r := range roles {
		if slices.Contains(p.Roles, r) {
			return true
		}
	}
	return false
}

type principalKey struct{}

func WithPrincipal(ctx context.Context, p Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

func PrincipalFrom(ctx context.Context) (Principal, bool) {
	p, ok := ctx.Value(principalKey{}).(Principal)
	return p, ok
}

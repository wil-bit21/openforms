// Package seed loads the embedded demo bundle and demo users.
package seed

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/openforms/openforms/examples"
	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/definitions"
)

// DemoPassword is the password of every demo user.
const DemoPassword = "demo1234"

// DemoUser describes one account created by Demo.
type DemoUser struct {
	Email string
	Name  string
	Roles []string
}

// DemoUsers are created by Demo when absent.
var DemoUsers = []DemoUser{
	{Email: "admin@demo.local", Name: "Avery Admin", Roles: []string{auth.RoleAdmin}},
	{Email: "reviewer@demo.local", Name: "Riley Reviewer", Roles: []string{"reviewer"}},
	{Email: "manager@demo.local", Name: "Morgan Manager", Roles: []string{"hiring-manager"}},
}

// Result reports what Demo changed.
type Result struct {
	Apply        definitions.ApplyResult
	UsersCreated []string
}

// Demo applies the example bundle (source "seed") and creates missing demo users.
// It is idempotent: unchanged definitions create no versions and existing users are left untouched.
func Demo(ctx context.Context, authSvc *auth.Service, defs *definitions.Store, orgID uuid.UUID) (Result, error) {
	var res Result
	forms, workflows, err := examples.Bundle()
	if err != nil {
		return res, fmt.Errorf("load example bundle: %w", err)
	}
	res.Apply, err = defs.Apply(ctx, orgID, definitions.ApplyInput{
		Forms:     forms,
		Workflows: workflows,
		Source:    definitions.SourceSeed,
		Actor:     "seed",
	})
	if err != nil {
		return res, fmt.Errorf("apply example bundle: %w", err)
	}
	for _, du := range DemoUsers {
		_, err := authSvc.GetUserByEmail(ctx, orgID, du.Email)
		if err == nil {
			continue
		}
		if !errors.Is(err, auth.ErrNotFound) {
			return res, fmt.Errorf("look up %s: %w", du.Email, err)
		}
		if _, err := authSvc.CreateUser(ctx, orgID, du.Email, du.Name, DemoPassword, du.Roles); err != nil {
			if errors.Is(err, auth.ErrEmailTaken) {
				continue // created concurrently by another instance
			}
			return res, fmt.Errorf("create %s: %w", du.Email, err)
		}
		res.UsersCreated = append(res.UsersCreated, du.Email)
	}
	return res, nil
}

// EnabledFromEnv reports whether OPENFORMS_SEED_DEMO asks for seeding on serve.
func EnabledFromEnv() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("OPENFORMS_SEED_DEMO")))
	return v == "true" || v == "1"
}

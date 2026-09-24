package auth

import (
	"fmt"
	"net/mail"
	"regexp"
	"slices"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

var roleRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func normalizeEmail(email string) (string, *definition.Problem) {
	e := strings.ToLower(strings.TrimSpace(email))
	addr, err := mail.ParseAddress(e)
	if err != nil || addr.Address != e {
		return e, &definition.Problem{Path: "email", Message: "must be a valid email address"}
	}
	return e, nil
}

func nameProblem(name string) *definition.Problem {
	if name == "" {
		return &definition.Problem{Path: "name", Message: "is required"}
	}
	return nil
}

func passwordProblem(pw string) *definition.Problem {
	switch {
	case len(pw) < 8:
		return &definition.Problem{Path: "password", Message: "must be at least 8 characters"}
	case len(pw) > 72:
		return &definition.Problem{Path: "password", Message: "must be at most 72 bytes"}
	}
	return nil
}

// normalizeRoles trims, de-duplicates (keeping first occurrence) and validates role names.
// It always returns a non-nil slice.
func normalizeRoles(roles []string) ([]string, []definition.Problem) {
	out := []string{}
	var probs []definition.Problem
	for i, r := range roles {
		r = strings.TrimSpace(r)
		if !roleRe.MatchString(r) {
			probs = append(probs, definition.Problem{
				Path:    fmt.Sprintf("roles[%d]", i),
				Message: "must be lowercase letters, digits, '-' or '_' (max 32)",
			})
			continue
		}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	return out, probs
}

func collect(probs []definition.Problem, more ...*definition.Problem) []definition.Problem {
	for _, p := range more {
		if p != nil {
			probs = append(probs, *p)
		}
	}
	return probs
}

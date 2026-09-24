package definition

import (
	"errors"
	"sort"
	"testing"
)

// problemPaths returns the sorted problem paths of a *ValidationError (nil for nil error).
func problemPaths(t *testing.T, err error) []string {
	t.Helper()
	if err == nil {
		return nil
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected *ValidationError, got %T: %v", err, err)
	}
	out := make([]string, len(ve.Problems))
	for i, p := range ve.Problems {
		out[i] = p.Path
	}
	sort.Strings(out)
	return out
}

func hasPath(paths []string, want string) bool {
	for _, p := range paths {
		if p == want {
			return true
		}
	}
	return false
}

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

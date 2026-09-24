package definition

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
	keyRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]{0,63}$`)
)

const (
	slugRule = "must match ^[a-z0-9][a-z0-9-]{0,62}$"
	keyRule  = "must match ^[a-zA-Z][a-zA-Z0-9_]{0,63}$"
)

// validateOptions checks an options list. allowed reports whether the owning
// field type takes options; when it does, at least one option is required.
func validateOptions(ps *problems, path string, allowed bool, notAllowedMsg string, opts []Option) {
	if !allowed {
		if len(opts) > 0 {
			ps.add(path+".options", notAllowedMsg)
		}
		return
	}
	if len(opts) == 0 {
		ps.add(path+".options", "at least one option is required")
		return
	}
	seen := make(map[string]bool, len(opts))
	for j, o := range opts {
		op := fmt.Sprintf("%s.options[%d]", path, j)
		switch {
		case o.Value == "":
			ps.add(op+".value", "is required")
		case strings.TrimSpace(o.Value) != o.Value:
			ps.add(op+".value", "must not start or end with whitespace")
		case seen[o.Value]:
			ps.add(op+".value", fmt.Sprintf("duplicate option value %q", o.Value))
		}
		seen[o.Value] = true
		if strings.TrimSpace(o.Label) == "" {
			ps.add(op+".label", "is required")
		}
	}
}

// Package examples embeds the demo bundle used by `openforms seed --demo`
// and displayed on the /demo page.
package examples

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/definition"
)

// FS holds the bundle rooted at "openforms/" (forms/ and workflows/ subdirectories).
//
//go:embed openforms
var FS embed.FS

// Bundle parses every embedded form and workflow, sorted by file name.
func Bundle() ([]definition.Form, []definition.Workflow, error) {
	forms, err := parseDir(FS, "openforms/forms", definition.ParseForm)
	if err != nil {
		return nil, nil, err
	}
	workflows, err := parseDir(FS, "openforms/workflows", definition.ParseWorkflow)
	if err != nil {
		return nil, nil, err
	}
	return forms, workflows, nil
}

func parseDir[T any](fsys fs.FS, dir string, parse func([]byte) (T, error)) ([]T, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	out := make([]T, 0, len(names))
	for _, name := range names {
		p := path.Join(dir, name)
		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		v, err := parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		out = append(out, v)
	}
	return out, nil
}

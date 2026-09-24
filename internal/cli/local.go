package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

// Problem is one validation finding for a local definition file.
type Problem struct {
	File    string // slash-separated, relative to the definitions directory
	Path    string // path inside the document, e.g. "fields[2].showIf.field"
	Message string
}

func (p Problem) String() string {
	if p.Path == "" {
		return fmt.Sprintf("%s: %s", p.File, p.Message)
	}
	return fmt.Sprintf("%s:%s: %s", p.File, p.Path, p.Message)
}

// LocalBundle is a parsed definitions directory.
type LocalBundle struct {
	Bundle        client.Bundle
	FormFiles     map[string]string
	WorkflowFiles map[string]string
}

var utf8BOM = []byte("\xef\xbb\xbf")

// LoadLocal reads <dir>/forms/* and <dir>/workflows/*, parses and validates every
// file, and checks cross-references. The error is reserved for I/O failures.
func LoadLocal(dir string) (LocalBundle, []Problem, error) {
	lb := LocalBundle{
		Bundle:        client.Bundle{Forms: []definition.Form{}, Workflows: []definition.Workflow{}},
		FormFiles:     map[string]string{},
		WorkflowFiles: map[string]string{},
	}
	var problems []Problem

	for _, kind := range []string{"forms", "workflows"} {
		files, err := definitionFiles(filepath.Join(dir, kind))
		if err != nil {
			return lb, nil, err
		}
		for _, path := range files {
			rel := kind + "/" + filepath.Base(path)
			raw, err := os.ReadFile(path)
			if err != nil {
				return lb, nil, err
			}
			raw = bytes.TrimPrefix(raw, utf8BOM)
			base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

			var slug string
			if kind == "forms" {
				f, err := definition.ParseForm(raw)
				if err != nil {
					problems = append(problems, toProblems(rel, err)...)
					continue
				}
				slug = f.Slug
				if p, ok := checkSlug(rel, "form", slug, base, lb.FormFiles); !ok {
					problems = append(problems, p)
					continue
				}
				lb.FormFiles[slug] = rel
				lb.Bundle.Forms = append(lb.Bundle.Forms, f)
			} else {
				w, err := definition.ParseWorkflow(raw)
				if err != nil {
					problems = append(problems, toProblems(rel, err)...)
					continue
				}
				slug = w.Slug
				if p, ok := checkSlug(rel, "workflow", slug, base, lb.WorkflowFiles); !ok {
					problems = append(problems, p)
					continue
				}
				lb.WorkflowFiles[slug] = rel
				lb.Bundle.Workflows = append(lb.Bundle.Workflows, w)
			}
		}
	}

	for _, f := range lb.Bundle.Forms {
		if f.Workflow != "" {
			if _, ok := lb.WorkflowFiles[f.Workflow]; !ok {
				problems = append(problems, Problem{
					File: lb.FormFiles[f.Slug], Path: "workflow",
					Message: fmt.Sprintf("unknown workflow %q (not found in workflows/)", f.Workflow),
				})
			}
		}
	}

	// Backstop: any remaining bundle-level rule from the definition package.
	if len(problems) == 0 {
		if err := definition.ValidateBundle(lb.Bundle.Forms, lb.Bundle.Workflows, nil); err != nil {
			problems = append(problems, toProblems("(bundle)", err)...)
		}
	}

	sort.SliceStable(problems, func(i, j int) bool {
		if problems[i].File != problems[j].File {
			return problems[i].File < problems[j].File
		}
		return problems[i].Path < problems[j].Path
	})
	return lb, problems, nil
}

func checkSlug(rel, kind, slug, base string, seen map[string]string) (Problem, bool) {
	if slug != base {
		return Problem{File: rel, Path: "slug", Message: fmt.Sprintf("slug %q must match file name %q", slug, base)}, false
	}
	if prev, dup := seen[slug]; dup {
		return Problem{File: rel, Path: "slug", Message: fmt.Sprintf("duplicate %s slug %q (also defined in %s)", kind, slug, prev)}, false
	}
	return Problem{}, true
}

func toProblems(file string, err error) []Problem {
	var ve *definition.ValidationError
	if errors.As(err, &ve) && len(ve.Problems) > 0 {
		out := make([]Problem, 0, len(ve.Problems))
		for _, p := range ve.Problems {
			out = append(out, Problem{File: file, Path: p.Path, Message: p.Message})
		}
		return out
	}
	return []Problem{{File: file, Message: err.Error()}}
}

// definitionFiles lists *.yaml, *.yml and *.json files directly inside dir.
func definitionFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		switch strings.ToLower(filepath.Ext(e.Name())) {
		case ".yaml", ".yml", ".json":
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	sort.Strings(out)
	return out, nil
}

func printProblems(w io.Writer, problems []Problem) {
	for _, p := range problems {
		fmt.Fprintln(w, p.String())
	}
	fmt.Fprintf(w, "%d problem(s) found\n", len(problems))
}

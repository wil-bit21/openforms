package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
)

type pullChange struct {
	Rel    string // e.g. "forms/contact.yaml"
	Path   string // absolute
	Data   []byte
	Status string // created | updated | unchanged
}

func NewPullCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	var check bool
	cmd := &cobra.Command{
		Use:          "pull",
		Short:        "Write the server's definitions to local files as canonical YAML",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			dir := d.resolveDir(rf.dir, pc)
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			b, err := d.NewClient(server, key).Export(cmd.Context())
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			changes, localOnly, err := planPull(dir, b)
			if err != nil {
				return err
			}
			if check {
				drift := 0
				for _, c := range changes {
					if c.Status != "unchanged" {
						drift++
						fmt.Fprintf(d.Out, "would %s %s\n", strings.TrimSuffix(c.Status, "d"), c.Rel)
					}
				}
				if drift > 0 {
					return ErrDrift
				}
				fmt.Fprintln(d.Out, "Up to date.")
				return nil
			}
			counts := map[string]int{}
			for _, c := range changes {
				counts[c.Status]++
				if c.Status == "unchanged" {
					continue
				}
				if err := os.MkdirAll(filepath.Dir(c.Path), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(c.Path, c.Data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(d.Out, "%s %s\n", c.Status, c.Rel)
			}
			for _, rel := range localOnly {
				fmt.Fprintf(d.Out, "note: %s exists locally but not on the server (left untouched)\n", rel)
			}
			fmt.Fprintf(d.Out, "Pulled %d form(s), %d workflow(s): %d created, %d updated, %d unchanged.\n",
				len(b.Forms), len(b.Workflows), counts["created"], counts["updated"], counts["unchanged"])
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	cmd.Flags().BoolVar(&check, "check", false, "write nothing; exit 1 if any file would change (CI drift check)")
	return cmd
}

// planPull computes what pull would write, without touching the filesystem.
func planPull(dir string, b client.Bundle) ([]pullChange, []string, error) {
	var changes []pullChange
	remote := map[string]bool{}
	add := func(kind, slug string, v any) error {
		remote[kind+"/"+slug] = true
		for _, ext := range []string{".yml", ".json"} {
			if _, err := os.Stat(filepath.Join(dir, kind, slug+ext)); err == nil {
				return fmt.Errorf("%s/%s%s exists; pull only manages .yaml files, rename it to %s/%s.yaml", kind, slug, ext, kind, slug)
			}
		}
		data, err := CanonicalYAML(v)
		if err != nil {
			return err
		}
		c := pullChange{Rel: kind + "/" + slug + ".yaml", Path: filepath.Join(dir, kind, slug+".yaml"), Data: data, Status: "created"}
		existing, err := os.ReadFile(c.Path)
		switch {
		case err == nil && bytes.Equal(existing, data):
			c.Status = "unchanged"
		case err == nil:
			c.Status = "updated"
		case !errors.Is(err, fs.ErrNotExist):
			return err
		}
		changes = append(changes, c)
		return nil
	}
	for _, f := range b.Forms {
		if err := add("forms", f.Slug, f); err != nil {
			return nil, nil, err
		}
	}
	for _, w := range b.Workflows {
		if err := add("workflows", w.Slug, w); err != nil {
			return nil, nil, err
		}
	}
	var localOnly []string
	for _, kind := range []string{"forms", "workflows"} {
		files, err := definitionFiles(filepath.Join(dir, kind))
		if err != nil {
			return nil, nil, err
		}
		for _, p := range files {
			base := strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
			if !remote[kind+"/"+base] {
				localOnly = append(localOnly, kind+"/"+filepath.Base(p))
			}
		}
	}
	return changes, localOnly, nil
}

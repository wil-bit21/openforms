package cli

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/definition"
)

//go:embed templates/contact.yaml
var tplContactForm []byte

//go:embed templates/contact-triage.yaml
var tplContactTriage []byte

const initConfig = "server: http://localhost:8080\ndir: openforms\n"

func NewInitCmd(d Deps) *cobra.Command {
	return &cobra.Command{
		Use:          "init [dir]",
		Short:        "Scaffold openforms.yaml and a sample form + workflow",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			target := d.WorkDir
			if len(args) == 1 {
				target = d.abs(args[0])
			}
			form, err := definition.ParseForm(tplContactForm)
			if err != nil {
				return fmt.Errorf("built-in form template is invalid: %w", err)
			}
			wf, err := definition.ParseWorkflow(tplContactTriage)
			if err != nil {
				return fmt.Errorf("built-in workflow template is invalid: %w", err)
			}
			formYAML, err := CanonicalYAML(form)
			if err != nil {
				return err
			}
			wfYAML, err := CanonicalYAML(wf)
			if err != nil {
				return err
			}
			files := []struct {
				rel  string
				data []byte
			}{
				{ConfigFile, []byte(initConfig)},
				{"openforms/forms/contact.yaml", formYAML},
				{"openforms/workflows/contact-triage.yaml", wfYAML},
			}
			for _, f := range files {
				_, err := os.Stat(filepath.Join(target, filepath.FromSlash(f.rel)))
				if err == nil {
					return fmt.Errorf("%s already exists; refusing to overwrite", f.rel)
				}
				if !errors.Is(err, fs.ErrNotExist) {
					return err
				}
			}
			for _, f := range files {
				p := filepath.Join(target, filepath.FromSlash(f.rel))
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, f.data, 0o644); err != nil {
					return err
				}
				fmt.Fprintf(d.Out, "created %s\n", f.rel)
			}
			fmt.Fprintln(d.Out, "\nNext steps:")
			fmt.Fprintln(d.Out, "  openforms validate")
			fmt.Fprintln(d.Out, "  export OPENFORMS_API_KEY=<key from `openforms admin create-api-key --name cli --roles admin`>")
			fmt.Fprintln(d.Out, "  openforms push")
			return nil
		},
	}
}

package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewValidateCmd(d Deps) *cobra.Command {
	var dirFlag string
	cmd := &cobra.Command{
		Use:          "validate",
		Short:        "Validate local form and workflow definitions (offline)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(dirFlag, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			fmt.Fprintf(d.Out, "OK: %d form(s), %d workflow(s) valid\n", len(lb.Bundle.Forms), len(lb.Bundle.Workflows))
			return nil
		},
	}
	addDirFlag(cmd, &dirFlag)
	return cmd
}

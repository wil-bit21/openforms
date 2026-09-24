package cli

import (
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/client"
)

func NewPushCmd(d Deps) *cobra.Command {
	var rf remoteFlags
	var dryRun bool
	cmd := &cobra.Command{
		Use:          "push",
		Short:        "Validate local definitions and apply them to the server",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			pc, err := LoadProjectConfig(d.WorkDir)
			if err != nil {
				return err
			}
			lb, problems, err := LoadLocal(d.resolveDir(rf.dir, pc))
			if err != nil {
				return err
			}
			if len(problems) > 0 {
				printProblems(d.Err, problems)
				return ErrProblems
			}
			server, key, err := d.resolveRemote(rf, pc)
			if err != nil {
				return err
			}
			res, err := d.NewClient(server, key).Apply(cmd.Context(), lb.Bundle, dryRun)
			if err != nil {
				return explainRemoteError(d.Err, err)
			}
			if dryRun {
				fmt.Fprintln(d.Out, "Dry run: nothing was applied.")
			}
			printApplyTable(d.Out, res)
			return nil
		},
	}
	addRemoteFlags(cmd, &rf)
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "show what would change without applying")
	return cmd
}

func applyStatus(it client.ApplyItem) string {
	switch {
	case it.Created:
		return "created"
	case it.Changed:
		return "updated"
	default:
		return "unchanged"
	}
}

func printApplyTable(w io.Writer, res client.ApplyResult) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tSLUG\tVERSION\tSTATUS")
	for _, it := range res.Items {
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n", it.Kind, it.Slug, it.Version, applyStatus(it))
	}
	tw.Flush()
}

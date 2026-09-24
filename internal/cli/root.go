// Package cli implements the openforms command (server and developer CLI in one binary).
package cli

import (
	"os"

	"github.com/spf13/cobra"
)

// Execute runs the root command and exits non-zero on error.
func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

// NewRootCmd builds the command tree. Later plans append subcommands here.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "openforms",
		Short:         "Developer-first forms with submission workflows",
		SilenceUsage:  true,
		SilenceErrors: false,
	}
	root.AddCommand(newServeCmd(), newMigrateCmd())
	root.AddCommand(Commands(DefaultDeps())...)
	return root
}

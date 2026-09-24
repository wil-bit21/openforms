package cli

import "github.com/spf13/cobra"

// Commands returns the developer-facing commands added by Plan 05 (Plan 09 appends seed).
func Commands(d Deps) []*cobra.Command {
	return []*cobra.Command{
		NewAdminCmd(d),
		NewInitCmd(d),
		NewValidateCmd(d),
		NewPushCmd(d),
		NewPullCmd(d),
		NewDiffCmd(d),
	}
}

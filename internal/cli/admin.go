package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
)

func NewAdminCmd(d Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Administrative commands that talk directly to the database (needs OPENFORMS_DATABASE_URL)",
	}
	cmd.AddCommand(newCreateUserCmd(d), newCreateAPIKeyCmd(d))
	return cmd
}

func newCreateUserCmd(d Deps) *cobra.Command {
	var email, name, password, roles string
	cmd := &cobra.Command{
		Use:          "create-user",
		Short:        "Create a user who can sign in to the admin UI",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if email == "" || password == "" {
				return errors.New("--email and --password are required")
			}
			if name == "" {
				name = strings.SplitN(email, "@", 2)[0]
			}
			svc, orgID, closeFn, err := d.OpenAuth(cmd.Context())
			if err != nil {
				return err
			}
			defer closeFn()
			u, err := svc.CreateUser(cmd.Context(), orgID, email, name, password, splitRoles(roles))
			if errors.Is(err, auth.ErrEmailTaken) {
				return fmt.Errorf("a user with email %s already exists", strings.ToLower(email))
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Created user %s (%s) with roles [%s]\n", u.Email, u.ID, strings.Join(u.Roles, ", "))
			return nil
		},
	}
	cmd.Flags().StringVar(&email, "email", "", "email address (required)")
	cmd.Flags().StringVar(&name, "name", "", "display name (default: part of the email before @)")
	cmd.Flags().StringVar(&password, "password", "", "password, at least 8 characters (required)")
	cmd.Flags().StringVar(&roles, "roles", "", "comma-separated roles, e.g. admin,reviewer")
	return cmd
}

func newCreateAPIKeyCmd(d Deps) *cobra.Command {
	var name, roles string
	cmd := &cobra.Command{
		Use:          "create-api-key",
		Short:        "Create an API key (printed once)",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" {
				return errors.New("--name is required")
			}
			svc, orgID, closeFn, err := d.OpenAuth(cmd.Context())
			if err != nil {
				return err
			}
			defer closeFn()
			plaintext, k, err := svc.CreateAPIKey(cmd.Context(), orgID, name, splitRoles(roles))
			if err != nil {
				return err
			}
			fmt.Fprintf(d.Out, "Created API key %q (prefix %s) with roles [%s]\n", k.Name, k.Prefix, strings.Join(k.Roles, ", "))
			fmt.Fprintln(d.Out, plaintext)
			fmt.Fprintln(d.Out, "Store this key now; it will not be shown again.")
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "a label for the key, e.g. ci (required)")
	cmd.Flags().StringVar(&roles, "roles", "", "comma-separated roles; use admin for push/pull")
	return cmd
}

func splitRoles(s string) []string {
	out := []string{}
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

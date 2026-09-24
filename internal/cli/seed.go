package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/seed"
)

func newSeedCmd() *cobra.Command {
	var demo bool
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Load the demo forms, workflows and users (idempotent)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if !demo {
				return errors.New("nothing to seed: pass --demo")
			}
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if cfg.DatabaseURL == "" {
				return errors.New("OPENFORMS_DATABASE_URL is required")
			}
			pool, err := db.Open(ctx, cfg.DatabaseURL)
			if err != nil {
				return err
			}
			defer pool.Close()
			if err := db.Migrate(ctx, pool); err != nil {
				return err
			}
			return runSeedDemo(ctx, cmd.OutOrStdout(), pool)
		},
	}
	cmd.Flags().BoolVar(&demo, "demo", false, "seed the demo bundle and demo users")
	return cmd
}

func runSeedDemo(ctx context.Context, out io.Writer, pool *pgxpool.Pool) error {
	authSvc := auth.NewService(pool)
	orgID, err := authSvc.EnsureDefaultOrg(ctx)
	if err != nil {
		return err
	}
	res, err := seed.Demo(ctx, authSvc, definitions.NewStore(pool), orgID)
	if err != nil {
		return err
	}
	for _, it := range res.Apply.Items {
		status := "unchanged"
		switch {
		case it.Created:
			status = "created"
		case it.Changed:
			status = "updated"
		}
		fmt.Fprintf(out, "%-9s %-20s v%-4d %s\n", it.Kind, it.Slug, it.Version, status)
	}
	if len(res.UsersCreated) == 0 {
		fmt.Fprintln(out, "demo users already exist")
	}
	for _, email := range res.UsersCreated {
		fmt.Fprintf(out, "user      %-20s password %s\n", email, seed.DemoPassword)
	}
	return nil
}

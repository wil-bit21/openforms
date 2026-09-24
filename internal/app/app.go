// Package app wires configuration, database, services and the HTTP router together.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/webui"
)

type App struct {
	Config  config.Config
	Pool    *pgxpool.Pool
	Auth    *auth.Service
	OrgID   uuid.UUID
	Defs    *definitions.Store
	Subs    *submissions.Service
	Handler http.Handler
}

// New opens the pool, migrates, ensures the default org and builds the router.
func New(ctx context.Context, cfg config.Config) (*App, error) {
	if err := cfg.RequireDatabase(); err != nil {
		return nil, err
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	authSvc := auth.NewService(pool)
	orgID, err := authSvc.EnsureDefaultOrg(ctx)
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("ensure default org: %w", err)
	}
	a := &App{Config: cfg, Pool: pool, Auth: authSvc, OrgID: orgID}
	a.Defs = definitions.NewStore(a.Pool)
	a.Subs = submissions.NewService(a.Pool, a.Defs)
	a.Handler = httpapi.NewRouter(httpapi.Deps{
		Config: cfg, Pool: pool, Auth: authSvc, Web: webui.Handler(), OrgID: orgID,
		Defs: a.Defs, Subs: a.Subs,
	})
	return a, nil
}

// Run serves HTTP until ctx is cancelled, then shuts down gracefully (10s).
func (a *App) Run(ctx context.Context) error {
	ln, err := net.Listen("tcp", a.Config.HTTPAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", a.Config.HTTPAddr, err)
	}
	srv := &http.Server{Handler: a.Handler, ReadHeaderTimeout: 10 * time.Second}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("openforms listening", "addr", ln.Addr().String(), "baseURL", a.Config.BaseURL)
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()
	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func (a *App) Close() { a.Pool.Close() }

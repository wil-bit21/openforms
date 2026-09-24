package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"

	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/config"
	"github.com/openforms/openforms/internal/db"
)

// Remote is the subset of the API the CLI needs; *client.Client implements it.
type Remote interface {
	Validate(ctx context.Context, b client.Bundle) (client.ValidateResult, error)
	Apply(ctx context.Context, b client.Bundle, dryRun bool) (client.ApplyResult, error)
	Export(ctx context.Context) (client.Bundle, error)
}

// OpenAuthFunc opens the database and returns an auth service scoped to the default org.
type OpenAuthFunc func(ctx context.Context) (svc *auth.Service, orgID uuid.UUID, closeFn func(), err error)

// Deps carries everything a command touches in the outside world, so tests can swap it.
type Deps struct {
	Out, Err  io.Writer
	WorkDir   string
	Getenv    func(string) string
	NewClient func(server, apiKey string) Remote
	OpenAuth  OpenAuthFunc
}

func DefaultDeps() Deps {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return Deps{
		Out:       os.Stdout,
		Err:       os.Stderr,
		WorkDir:   wd,
		Getenv:    os.Getenv,
		NewClient: func(server, apiKey string) Remote { return client.New(server, apiKey) },
		OpenAuth:  openAuthFromEnv,
	}
}

var (
	// ErrProblems is returned after validation problems have been printed.
	ErrProblems = errors.New("definitions are invalid")
	// ErrDrift is returned by `pull --check` when local files differ from the server.
	ErrDrift = errors.New("local definitions differ from the server")
)

const ConfigFile = "openforms.yaml"

// ProjectConfig is openforms.yaml. API keys are deliberately not supported here.
type ProjectConfig struct {
	Server string `json:"server"`
	Dir    string `json:"dir"`
}

func LoadProjectConfig(workDir string) (ProjectConfig, error) {
	var pc ProjectConfig
	raw, err := os.ReadFile(filepath.Join(workDir, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		return pc, nil
	}
	if err != nil {
		return pc, err
	}
	if err := yaml.UnmarshalStrict(raw, &pc); err != nil {
		return pc, fmt.Errorf("%s: %w", ConfigFile, err)
	}
	return pc, nil
}

type remoteFlags struct {
	server, apiKey, dir string
}

func addDirFlag(cmd *cobra.Command, dir *string) {
	cmd.Flags().StringVar(dir, "dir", "", "definitions directory (default: dir from openforms.yaml, else ./openforms)")
}

func addRemoteFlags(cmd *cobra.Command, f *remoteFlags) {
	addDirFlag(cmd, &f.dir)
	cmd.Flags().StringVar(&f.server, "server", "", "openforms server URL (default: $OPENFORMS_URL, else server from openforms.yaml)")
	cmd.Flags().StringVar(&f.apiKey, "api-key", "", "API key with the admin role (default: $OPENFORMS_API_KEY)")
}

func (d Deps) abs(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(d.WorkDir, p)
}

func (d Deps) resolveDir(flag string, pc ProjectConfig) string {
	dir := flag
	if dir == "" {
		dir = pc.Dir
	}
	if dir == "" {
		dir = "openforms"
	}
	return d.abs(dir)
}

func (d Deps) resolveRemote(f remoteFlags, pc ProjectConfig) (string, string, error) {
	server := f.server
	if server == "" {
		server = d.Getenv("OPENFORMS_URL")
	}
	if server == "" {
		server = pc.Server
	}
	if server == "" {
		return "", "", errors.New("no server configured: pass --server, set OPENFORMS_URL, or add `server:` to openforms.yaml")
	}
	key := f.apiKey
	if key == "" {
		key = d.Getenv("OPENFORMS_API_KEY")
	}
	if key == "" {
		return "", "", errors.New("no API key: pass --api-key or set OPENFORMS_API_KEY (create one with `openforms admin create-api-key --name cli --roles admin`)")
	}
	return server, key, nil
}

// explainRemoteError turns API errors into actionable messages. 422 details are
// printed as problems and ErrProblems is returned.
func explainRemoteError(errOut io.Writer, err error) error {
	var apiErr *client.Error
	if !errors.As(err, &apiErr) {
		return err
	}
	switch {
	case apiErr.Status == http.StatusUnauthorized:
		return fmt.Errorf("authentication failed (%s): check --api-key or OPENFORMS_API_KEY", apiErr.Code)
	case apiErr.Status == http.StatusForbidden:
		return fmt.Errorf("permission denied (%s): the API key needs the %q role", apiErr.Code, auth.RoleAdmin)
	case apiErr.Status == http.StatusUnprocessableEntity && len(apiErr.Details) > 0:
		ps := make([]Problem, 0, len(apiErr.Details))
		for _, p := range apiErr.Details {
			ps = append(ps, Problem{File: "server", Path: p.Path, Message: p.Message})
		}
		printProblems(errOut, ps)
		return ErrProblems
	}
	return err
}

func openAuthFromEnv(ctx context.Context) (*auth.Service, uuid.UUID, func(), error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, uuid.Nil, nil, err
	}
	if cfg.DatabaseURL == "" {
		return nil, uuid.Nil, nil, errors.New("OPENFORMS_DATABASE_URL is required for admin commands")
	}
	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, uuid.Nil, nil, err
	}
	if err := db.Migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, uuid.Nil, nil, err
	}
	svc := auth.NewService(pool)
	orgID, err := svc.EnsureDefaultOrg(ctx)
	if err != nil {
		pool.Close()
		return nil, uuid.Nil, nil, err
	}
	return svc, orgID, pool.Close, nil
}

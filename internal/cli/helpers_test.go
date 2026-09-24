package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
)

type fakeRemote struct {
	applied   []client.Bundle
	dryRuns   []bool
	applyRes  client.ApplyResult
	applyErr  error
	export    client.Bundle
	exportErr error
}

func (f *fakeRemote) Validate(ctx context.Context, b client.Bundle) (client.ValidateResult, error) {
	return client.ValidateResult{Valid: true}, nil
}

func (f *fakeRemote) Apply(ctx context.Context, b client.Bundle, dryRun bool) (client.ApplyResult, error) {
	f.applied = append(f.applied, b)
	f.dryRuns = append(f.dryRuns, dryRun)
	return f.applyRes, f.applyErr
}

func (f *fakeRemote) Export(ctx context.Context) (client.Bundle, error) {
	return f.export, f.exportErr
}

type testIO struct{ out, err bytes.Buffer }

func newTestDeps(workDir string, env map[string]string, r Remote) (Deps, *testIO) {
	tio := &testIO{}
	return Deps{
		Out:     &tio.out,
		Err:     &tio.err,
		WorkDir: workDir,
		Getenv:  func(k string) string { return env[k] },
		NewClient: func(server, apiKey string) Remote {
			return r
		},
		OpenAuth: func(context.Context) (*auth.Service, uuid.UUID, func(), error) {
			return nil, uuid.Nil, nil, errors.New("OpenAuth not configured in this test")
		},
	}, tio
}

// execute runs a freshly constructed command with args; cobra's own output is discarded
// because commands write through Deps.Out / Deps.Err.
func execute(cmd *cobra.Command, args ...string) error {
	cmd.SetArgs(args)
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	return cmd.Execute()
}

func readRel(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// remoteEnv is the env used by tests that talk to a fake remote.
var remoteEnv = map[string]string{"OPENFORMS_URL": "http://fake", "OPENFORMS_API_KEY": "ofk_fake"}

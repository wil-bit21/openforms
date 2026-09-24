package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/cli"
	"github.com/openforms/openforms/internal/db/dbtest"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestMigrateCommand(t *testing.T) {
	t.Setenv("OPENFORMS_DATABASE_URL", dbtest.NewURL(t))
	out, err := run(t, "migrate")
	if err != nil {
		t.Fatalf("migrate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "migrations applied") {
		t.Fatalf("output = %q", out)
	}
}

func TestServeAndMigrateRequireDatabaseURL(t *testing.T) {
	t.Setenv("OPENFORMS_DATABASE_URL", "")
	for _, sub := range []string{"serve", "migrate"} {
		out, err := run(t, sub)
		if err == nil || !strings.Contains(err.Error()+out, "OPENFORMS_DATABASE_URL") {
			t.Errorf("%s: err=%v out=%q", sub, err, out)
		}
	}
}

func TestRootHelpListsCommands(t *testing.T) {
	out, err := run(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"serve", "migrate"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q:\n%s", want, out)
		}
	}
}

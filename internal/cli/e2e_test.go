package cli

import (
	"errors"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/openforms/openforms/internal/auth"
	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definitions"
	"github.com/openforms/openforms/internal/httpapi"
	"github.com/openforms/openforms/internal/submissions"
	"github.com/openforms/openforms/internal/testutil"
)

func TestCommandsRegistersAll(t *testing.T) {
	d, _ := newTestDeps(t.TempDir(), nil, nil)
	var names []string
	for _, c := range Commands(d) {
		names = append(names, c.Name())
	}
	if got := strings.Join(names, ","); got != "admin,init,validate,push,pull,diff" {
		t.Fatalf("commands = %s", got)
	}
}

func TestPushPullRoundTripAgainstRealServer(t *testing.T) {
	env := testutil.NewEnv(t)
	defs := definitions.NewStore(env.Pool)
	subs := submissions.NewService(env.Pool, defs)
	srv := httptest.NewServer(httpapi.NewRouter(httpapi.Deps{
		Config: env.Config, Auth: env.Auth, Defs: defs, Subs: subs, OrgID: env.OrgID,
	}))
	t.Cleanup(srv.Close)
	key := env.APIKey(t, auth.RoleAdmin)

	run := func(workDir, apiKey string, args ...string) (string, string, error) {
		d, tio := newTestDeps(workDir, map[string]string{"OPENFORMS_URL": srv.URL, "OPENFORMS_API_KEY": apiKey}, nil)
		d.NewClient = func(server, k string) Remote { return client.New(server, k) }
		root := &cobra.Command{Use: "openforms", SilenceUsage: true}
		root.AddCommand(Commands(d)...)
		err := execute(root, args...)
		return tio.out.String(), tio.err.String(), err
	}
	mustMatch := func(out, pattern string) {
		t.Helper()
		if !regexp.MustCompile(pattern).MatchString(out) {
			t.Fatalf("output does not match %q:\n%s", pattern, out)
		}
	}

	projA := t.TempDir()
	if _, _, err := run(projA, key, "init"); err != nil {
		t.Fatal(err)
	}

	// Review Focus #4: wrong key gives an actionable message.
	if _, _, err := run(projA, "ofk_wrong", "push"); err == nil || !strings.Contains(err.Error(), "check --api-key or OPENFORMS_API_KEY") {
		t.Fatalf("wrong key err = %v", err)
	}

	out, errOut, err := run(projA, key, "push")
	if err != nil {
		t.Fatalf("push: %v\n%s", err, errOut)
	}
	mustMatch(out, `(?m)^workflow\s+contact-triage\s+1\s+created$`)
	mustMatch(out, `(?m)^form\s+contact\s+1\s+created$`)

	out, _, err = run(projA, key, "push")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+1\s+unchanged$`)

	if out, _, err = run(projA, key, "diff"); err != nil || out != "No differences.\n" {
		t.Fatalf("diff: %q %v", out, err)
	}
	if out, _, err = run(projA, key, "pull", "--check"); err != nil || out != "Up to date.\n" {
		t.Fatalf("pull --check: %q %v", out, err)
	}

	projB := t.TempDir()
	if _, _, err := run(projB, key, "pull"); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{"openforms/forms/contact.yaml", "openforms/workflows/contact-triage.yaml"} {
		if readRel(t, projA, rel) != readRel(t, projB, rel) {
			t.Fatalf("%s differs between pushed and pulled copies", rel)
		}
	}

	// Change the form in A: diff, dry-run, push, then B is out of date.
	formA := readRel(t, projA, "openforms/forms/contact.yaml")
	put(t, projA, "openforms/forms/contact.yaml", strings.Replace(formA, "title: Contact us", "title: Talk to us", 1))
	if out, _, err = run(projA, key, "diff"); err != nil || out != "~ form contact\n" {
		t.Fatalf("diff after edit: %q %v", out, err)
	}
	out, _, err = run(projA, key, "push", "--dry-run")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+2\s+updated$`)
	out, _, err = run(projA, key, "push")
	if err != nil {
		t.Fatal(err)
	}
	mustMatch(out, `(?m)^form\s+contact\s+2\s+updated$`)

	if _, _, err := run(projB, key, "pull", "--check"); !errors.Is(err, ErrDrift) {
		t.Fatalf("pull --check in stale project: err = %v", err)
	}
	if _, _, err := run(projB, key, "pull"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readRel(t, projB, "openforms/forms/contact.yaml"), "title: Talk to us") {
		t.Fatal("pull did not bring the new title")
	}
}

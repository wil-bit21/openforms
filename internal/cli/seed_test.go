package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/db/dbtest"
)

func TestRunSeedDemoIsIdempotent(t *testing.T) {
	pool := dbtest.New(t)
	ctx := context.Background()

	var first bytes.Buffer
	if err := runSeedDemo(ctx, &first, pool); err != nil {
		t.Fatalf("first run: %v", err)
	}
	out := first.String()
	for _, want := range []string{"job-application", "hiring", "contact-triage", "created", "reviewer@demo.local", "demo1234"} {
		if !strings.Contains(out, want) {
			t.Errorf("first output missing %q:\n%s", want, out)
		}
	}

	var second bytes.Buffer
	if err := runSeedDemo(ctx, &second, pool); err != nil {
		t.Fatalf("second run: %v", err)
	}
	out = second.String()
	if strings.Contains(out, "created") || strings.Contains(out, "updated") {
		t.Errorf("second run should report only unchanged items:\n%s", out)
	}
	if !strings.Contains(out, "demo users already exist") {
		t.Errorf("second output missing users line:\n%s", out)
	}
}

func TestSeedCommandRequiresDemoFlag(t *testing.T) {
	cmd := newSeedCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "--demo") {
		t.Fatalf("err = %v, want mention of --demo", err)
	}
}

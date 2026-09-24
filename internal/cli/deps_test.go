package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/openforms/openforms/internal/client"
	"github.com/openforms/openforms/internal/definition"
)

func TestLoadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	pc, err := LoadProjectConfig(dir)
	if err != nil || pc != (ProjectConfig{}) {
		t.Fatalf("missing file: pc=%+v err=%v", pc, err)
	}
	put(t, dir, ConfigFile, "server: http://file:8080\ndir: defs\n")
	pc, err = LoadProjectConfig(dir)
	if err != nil || pc.Server != "http://file:8080" || pc.Dir != "defs" {
		t.Fatalf("pc=%+v err=%v", pc, err)
	}
	put(t, dir, ConfigFile, "servr: typo\n")
	if _, err := LoadProjectConfig(dir); err == nil || !strings.Contains(err.Error(), ConfigFile) {
		t.Fatalf("unknown key must fail and name the file, err=%v", err)
	}
}

func TestResolveDir(t *testing.T) {
	wd := t.TempDir()
	d, _ := newTestDeps(wd, nil, nil)
	if got := d.resolveDir("", ProjectConfig{}); got != filepath.Join(wd, "openforms") {
		t.Fatalf("default = %s", got)
	}
	if got := d.resolveDir("", ProjectConfig{Dir: "defs"}); got != filepath.Join(wd, "defs") {
		t.Fatalf("config = %s", got)
	}
	if got := d.resolveDir("flagdir", ProjectConfig{Dir: "defs"}); got != filepath.Join(wd, "flagdir") {
		t.Fatalf("flag = %s", got)
	}
	abs := filepath.Join(t.TempDir(), "x")
	if got := d.resolveDir(abs, ProjectConfig{}); got != abs {
		t.Fatalf("absolute = %s", got)
	}
}

func TestResolveRemotePrecedence(t *testing.T) {
	pc := ProjectConfig{Server: "http://file"}
	env := map[string]string{"OPENFORMS_URL": "http://env", "OPENFORMS_API_KEY": "ofk_env"}
	d, _ := newTestDeps(t.TempDir(), env, nil)

	s, k, err := d.resolveRemote(remoteFlags{server: "http://flag", apiKey: "ofk_flag"}, pc)
	if err != nil || s != "http://flag" || k != "ofk_flag" {
		t.Fatalf("flags: %s %s %v", s, k, err)
	}
	s, k, err = d.resolveRemote(remoteFlags{}, pc)
	if err != nil || s != "http://env" || k != "ofk_env" {
		t.Fatalf("env: %s %s %v", s, k, err)
	}

	dNoEnv, _ := newTestDeps(t.TempDir(), map[string]string{"OPENFORMS_API_KEY": "ofk_env"}, nil)
	s, _, err = dNoEnv.resolveRemote(remoteFlags{}, pc)
	if err != nil || s != "http://file" {
		t.Fatalf("file: %s %v", s, err)
	}

	_, _, err = dNoEnv.resolveRemote(remoteFlags{}, ProjectConfig{})
	if err == nil || !strings.Contains(err.Error(), "no server configured") {
		t.Fatalf("missing server err = %v", err)
	}
	dNoKey, _ := newTestDeps(t.TempDir(), nil, nil)
	_, _, err = dNoKey.resolveRemote(remoteFlags{}, pc)
	if err == nil || !strings.Contains(err.Error(), "no API key") {
		t.Fatalf("missing key err = %v", err)
	}
}

// Review Focus #4: auth failures must tell the user what to fix.
func TestExplainRemoteError(t *testing.T) {
	var buf bytes.Buffer
	err := explainRemoteError(&buf, &client.Error{Status: 401, Code: "unauthenticated", Message: "missing credentials"})
	if err == nil || !strings.Contains(err.Error(), "check --api-key or OPENFORMS_API_KEY") {
		t.Fatalf("401: %v", err)
	}
	err = explainRemoteError(&buf, &client.Error{Status: 403, Code: "forbidden", Message: "admin only"})
	if err == nil || !strings.Contains(err.Error(), `needs the "admin" role`) {
		t.Fatalf("403: %v", err)
	}
	err = explainRemoteError(&buf, &client.Error{Status: 422, Code: "validation_failed", Message: "invalid",
		Details: []definition.Problem{{Path: "forms[0].title", Message: "required"}}})
	if !errors.Is(err, ErrProblems) || !strings.Contains(buf.String(), "server:forms[0].title: required") {
		t.Fatalf("422: err=%v out=%q", err, buf.String())
	}
	plain := fmt.Errorf("boom")
	if got := explainRemoteError(&buf, plain); got != plain {
		t.Fatalf("passthrough: %v", got)
	}
}

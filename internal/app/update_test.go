package app

import (
	"bytes"
	"context"
	"errors"
	action "github.com/ahillspace/tadx/actions/update"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type updateTestRuntime struct {
	checks, installs int
	targets          []string
}

func (*updateTestRuntime) Current() string                     { return "1.0.0" }
func (*updateTestRuntime) InstallationTarget() (string, error) { return "/opt/tadx/bin/tadx", nil }
func (f *updateTestRuntime) ValidateTargets(targets []string) error {
	f.targets = targets
	for _, target := range targets {
		if target == "invalid" {
			return errors.New("invalid target")
		}
	}
	return nil
}
func (f *updateTestRuntime) Latest(context.Context) (action.Release, error) {
	f.checks++
	return action.Release{Version: "1.1.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.1.0"}, nil
}
func (f *updateTestRuntime) Install(context.Context, action.Release, []string) error {
	f.installs++
	return nil
}

type updateTestRenderer struct{ value any }

func (r *updateTestRenderer) Render(value any) error { r.value = value; return nil }
func TestUpdateAppCommandCheckAndValidation(t *testing.T) {
	for _, invalid := range []bool{false, true} {
		t.Run(map[bool]string{false: "check", true: "invalid"}[invalid], func(t *testing.T) {
			execution := &updateTestRuntime{}
			renderer := &updateTestRenderer{}
			deps := newUpdateCommand(execution)
			deps.Renderer = renderer
			cmd := updatecli.New(*deps)
			args := []string{"--check", "--target", "codex"}
			if invalid {
				args = []string{"--target", "invalid"}
			}
			cmd.SetArgs(args)
			err := cmd.ExecuteContext(t.Context())
			if invalid {
				if err == nil || execution.checks != 0 || execution.installs != 0 {
					t.Fatalf("invalid: %v %+v", err, execution)
				}
				return
			}
			if err != nil || execution.checks != 1 || execution.installs != 0 {
				t.Fatalf("check: %v %+v", err, execution)
			}
			if renderer.value.(action.Output).Status != "checked" {
				t.Fatal(renderer.value)
			}
		})
	}
}
func TestUpdateRootRoutesWithoutReleaseCalls(t *testing.T) {
	for _, args := range [][]string{{"update", "--help"}, {"update", "--target", "invalid"}, {"capability", "get", "update"}} {
		t.Run(strings.Join(args, "-"), func(t *testing.T) {
			temp := t.TempDir()
			var out bytes.Buffer
			code := Run(t.Context(), args, &out, Options{ConfigPath: filepath.Join(temp, "config.yaml"), UserHomeDir: func() (string, error) { return temp, nil }})
			if args[0] == "update" && args[1] == "--target" {
				if code == 0 || !strings.Contains(out.String(), "update.validate.failed") {
					t.Fatalf("%d %s", code, out.String())
				}
				return
			}
			if code != 0 || !strings.Contains(out.String(), "update") {
				t.Fatalf("%d %s", code, out.String())
			}
		})
	}
}

func TestUpdaterWithholdsConfiguredPATVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "version: 1\nenvironments:\n  production:\n    url: https://tableau.example.com\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_UPDATE_PAT_NAME\n      pat_secret_env: PROD_UPDATE_PAT_SECRET\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(configuredPATVariables(path)(), ",")
	if got != "PROD_UPDATE_PAT_NAME,PROD_UPDATE_PAT_SECRET" {
		t.Fatalf("configured PAT variables = %q", got)
	}
	if missing := configuredPATVariables(filepath.Join(t.TempDir(), "missing.yaml"))(); len(missing) != 0 {
		t.Fatalf("missing configuration listed %q", missing)
	}
}

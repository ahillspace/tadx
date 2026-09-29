package app

import (
	"bytes"
	"context"
	"errors"
	action "github.com/ahillspace/tadx/actions/update"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
	"os"
	"path/filepath"
	"runtime"
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
	got := strings.Join((&runtimeDependencies{configPath: path}).configuredPATVariables(), ",")
	if got != "PROD_UPDATE_PAT_NAME,PROD_UPDATE_PAT_SECRET" {
		t.Fatalf("configured PAT variables = %q", got)
	}
	if missing := (&runtimeDependencies{configPath: filepath.Join(t.TempDir(), "missing.yaml")}).configuredPATVariables(); len(missing) != 0 {
		t.Fatalf("missing configuration listed %q", missing)
	}
}

// A config selected by flag is parsed after the command tree is built, so the
// updater must read the selected file when it starts a child, not at wiring time.
func TestUpdaterWithholdsPATVariablesFromFlagSelectedConfig(t *testing.T) {
	for _, flag := range []string{"--config", "--cfg"} {
		t.Run(flag, func(t *testing.T) {
			temp := t.TempDir()
			configuration := func(name, secret string) string {
				return "version: 1\nenvironments:\n  production:\n    url: https://tableau.example.com\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: " + name + "\n      pat_secret_env: " + secret + "\n"
			}
			defaultPath := filepath.Join(temp, "default.yaml")
			selectedPath := filepath.Join(temp, "selected.yaml")
			if err := os.WriteFile(defaultPath, []byte(configuration("DEFAULT_UPDATE_PAT_NAME", "DEFAULT_UPDATE_PAT_SECRET")), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(selectedPath, []byte(configuration("SELECTED_UPDATE_PAT_NAME", "SELECTED_UPDATE_PAT_SECRET")), 0o600); err != nil {
				t.Fatal(err)
			}
			bin := filepath.Join(temp, "bin")
			if err := os.Mkdir(bin, 0o700); err != nil {
				t.Fatal(err)
			}
			captured := filepath.Join(temp, "gh-environment")
			if runtime.GOOS == "windows" {
				if err := os.WriteFile(filepath.Join(bin, "gh.bat"), []byte("@set > \"%UPDATE_FAKE_GH_CAPTURE%\"\r\n"), 0o700); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(filepath.Join(bin, "gh"), []byte("#!/bin/sh\nexport -p > \"$UPDATE_FAKE_GH_CAPTURE\"\n"), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", bin)
			t.Setenv("UPDATE_FAKE_GH_CAPTURE", captured)
			t.Setenv("SELECTED_UPDATE_PAT_NAME", "fixture-name")
			t.Setenv("SELECTED_UPDATE_PAT_SECRET", "fixture-secret")
			t.Setenv("UPDATE_FIXTURE_KEPT", "kept")
			var out bytes.Buffer
			Run(t.Context(), []string{flag, selectedPath, "update", "--check"}, &out, Options{ConfigPath: defaultPath, UserHomeDir: func() (string, error) { return temp, nil }})
			environment, err := os.ReadFile(captured)
			if err != nil {
				t.Fatalf("fake gh was not run: %v\n%s", err, out.String())
			}
			upper := strings.ToUpper(string(environment))
			for _, withheld := range []string{"SELECTED_UPDATE_PAT_NAME", "SELECTED_UPDATE_PAT_SECRET"} {
				if strings.Contains(upper, withheld) {
					t.Errorf("updater child received %s", withheld)
				}
			}
			if !strings.Contains(upper, "UPDATE_FIXTURE_KEPT") {
				t.Fatalf("updater child lost ordinary variables: %s", environment)
			}
		})
	}
}

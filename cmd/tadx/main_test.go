package main_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestCLIProcessExitCodesAndStreams(t *testing.T) {
	binary := buildCLI(t)
	tests := []struct {
		name     string
		args     []string
		wantExit int
		wantKind string
	}{
		{name: "success", args: []string{"capability", "list", "--domain", "capability"}, wantExit: 0},
		{name: "operation error", args: []string{"capability", "get", "missing"}, wantExit: 1, wantKind: "operation"},
		{name: "usage error", args: []string{"capability", "list", "--unknown"}, wantExit: 2, wantKind: "usage"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runCLI(t, binary, test.args, nil)
			if result.exitCode != test.wantExit {
				t.Fatalf("exit code = %d, want %d\nstdout:\n%s\nstderr:\n%s", result.exitCode, test.wantExit, result.stdout, result.stderr)
			}
			if result.stdout == "" {
				t.Fatal("stdout is empty, want one TOON document")
			}
			if result.stderr != "" {
				t.Fatalf("stderr = %q, want empty", result.stderr)
			}
			document := decodeDocument(t, result.stdout)
			if test.wantKind == "" {
				if _, ok := document["page"]; !ok {
					t.Fatalf("success output has no page envelope: %#v", document)
				}
				return
			}
			errorDocument, ok := document["error"].(map[string]any)
			if !ok {
				t.Fatalf("error output has no error envelope: %#v", document)
			}
			if got := errorDocument["kind"]; got != test.wantKind {
				t.Fatalf("error kind = %#v, want %q", got, test.wantKind)
			}
		})
	}
}

func TestCLIProcessRejectsRetiredRoutes(t *testing.T) {
	binary := buildCLI(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "retired content get", args: []string{"content", "get"}},
		{name: "retired content search", args: []string{"content", "search"}},
		{name: "retired workbook get", args: []string{"content", "workbook", "get"}},
		{name: "retired cache search", args: []string{"cache", "search"}},
		{name: "retired admin user get", args: []string{"admin", "user", "get"}},
		{name: "retired Pulse metric get", args: []string{"pulse", "metric", "get"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runCLI(t, binary, test.args, nil)
			if result.exitCode != 2 {
				t.Fatalf("exit code = %d, want 2\nstdout:\n%s\nstderr:\n%s", result.exitCode, result.stdout, result.stderr)
			}
			if result.stderr != "" {
				t.Fatalf("stderr = %q, want empty", result.stderr)
			}
			document := decodeDocument(t, result.stdout)
			errorDocument, ok := document["error"].(map[string]any)
			if !ok || errorDocument["kind"] != "usage" {
				t.Fatalf("error = %#v, want structured usage error", document)
			}
		})
	}
}

func TestCLIProcessBareGroupsKeepHelpBehavior(t *testing.T) {
	binary := buildCLI(t)
	for _, args := range [][]string{
		{"content", "workbook"},
		{"cache"},
		{"admin", "user"},
		{"pulse", "metric"},
	} {
		result := runCLI(t, binary, args, nil)
		inventory := "Commands:"
		if result.exitCode != 0 || result.stderr != "" || !strings.Contains(strings.ToLower(result.stdout), "usage: tadx "+strings.Join(args, " ")) || !strings.Contains(result.stdout, inventory) {
			t.Fatalf("args = %v, exit = %d, stdout = %q, stderr = %q", args, result.exitCode, result.stdout, result.stderr)
		}
		explicit := runCLI(t, binary, append(append([]string{}, args...), "--help"), nil)
		if explicit.exitCode != 0 || explicit.stderr != "" || explicit.stdout != result.stdout {
			t.Fatalf("bare group %v differs from its explicit help", args)
		}
	}
}

func TestCLIProcessProjectIDSelectorsReachOnlyIsolatedSetup(t *testing.T) {
	binary := buildCLI(t)
	for _, flag := range []string{"--id", "--project-id"} {
		result := runCLI(t, binary, []string{"content", "project", "inspect", flag, "project-1"}, nil)
		if result.exitCode == 0 || strings.Contains(result.stdout, "unknown flag") || result.stderr != "" {
			t.Fatalf("flag%s exit%d output%s stderr%s", flag, result.exitCode, result.stdout, result.stderr)
		}
		document := decodeDocument(t, result.stdout)
		failure, ok := document["error"].(map[string]any)
		if !ok || failure["id"] != "project.inspect.setup" {
			t.Fatalf("expected isolated setup failure, got %#v", document)
		}
	}
}

func TestCLIProcessMutationDiscoveryEnvironment(t *testing.T) {
	binary := buildCLI(t)
	for _, consent := range []bool{false, true} {
		for _, legacy := range []string{"0", "1"} {
			t.Run(fmt.Sprintf("saved_%t_legacy_%s", consent, legacy), func(t *testing.T) {
				path := processSiteConfig(t, consent)
				args := []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true", "--environment", "fixture", "--config", path}
				result := runCLI(t, binary, args, map[string]string{"TADX_ENABLE_MUTATIONS": legacy})
				if result.exitCode != 0 || result.stderr != "" {
					t.Fatalf("mutation discovery failed: %+v", result)
				}
				document := decodeDocument(t, result.stdout)
				capabilities, ok := document["capabilities"].([]any)
				if !ok || len(capabilities) != 4 {
					t.Fatalf("expected four workbook mutations: %#v", document)
				}
				for index, want := range []string{"workbook.delete", "workbook.move", "workbook.publish", "workbook.update"} {
					capability, ok := capabilities[index].(map[string]any)
					if !ok || capability["id"] != want || capability["execution_enabled"] != consent {
						t.Fatalf("legacy %s changed saved %t consent: %#v", legacy, consent, capabilities[index])
					}
				}
			})
		}
	}
}

func TestCLIProcessMutationCommandsAreVisibleAndGated(t *testing.T) {
	binary := buildCLI(t)
	help := runCLI(t, binary, []string{"content", "workbook", "--help"}, nil)
	if help.exitCode != 0 || !strings.Contains(help.stdout, "publish") || !strings.Contains(help.stdout, "delete") {
		t.Fatalf("workbook help did not expose mutations: exit = %d, stdout = %s", help.exitCode, help.stdout)
	}

	path := processSiteConfig(t, false)
	result := runCLI(t, binary, []string{"content", "workbook", "delete", "--environment", "fixture", "--id", "workbook-1", "--config", path}, map[string]string{"TADX_ENABLE_MUTATIONS": "1"})
	document := decodeDocument(t, result.stdout)
	errorDocument, ok := document["error"].(map[string]any)
	if result.exitCode != 1 || !ok || errorDocument["id"] != "mutation.disabled" || errorDocument["operation"] != "workbook.delete" || errorDocument["environment"] != "fixture" || errorDocument["outcome"] != "not_attempted" || result.stderr != "" {
		t.Fatalf("mutation gate result = %#v, exit = %d", document, result.exitCode)
	}
}

func TestCLIProcessShorthand(t *testing.T) {
	binary := buildCLI(t)
	t.Run("canonical output", func(t *testing.T) {
		configPath := filepath.Join(t.TempDir(), "missing.yaml")
		canonical := runCLI(t, binary, []string{"capability", "list", "--domain", "capability", "--full", "--config", configPath}, nil)
		short := runCLI(t, binary, []string{"cap", "ls", "--dom", "capability", "-f", "--config", configPath}, nil)
		if canonical.exitCode != 0 || short.exitCode != 0 || canonical.stdout != short.stdout || short.stderr != "" {
			t.Fatalf("canonical=%+v shorthand=%+v", canonical, short)
		}
	})
	t.Run("mutation remains gated", func(t *testing.T) {
		path := processSiteConfig(t, false)
		result := runCLI(t, binary, []string{"con", "wb", "del", "--env", "fixture", "-i", "workbook-1", "--pv=false", "-f", "--config", path}, map[string]string{"TADX_ENABLE_MUTATIONS": "1"})
		document := decodeDocument(t, result.stdout)
		failure, ok := document["error"].(map[string]any)
		if result.exitCode != 1 || !ok || failure["id"] != "mutation.disabled" || failure["operation"] != "workbook.delete" || failure["environment"] != "fixture" || failure["outcome"] != "not_attempted" || result.stderr != "" {
			t.Fatalf("shorthand mutation result=%+v", result)
		}
	})
	t.Run("help advertises flags", func(t *testing.T) {
		result := runCLI(t, binary, []string{"con", "wb", "pub", "-h"}, nil)
		for _, want := range []string{"--full", "--preview", "--environment (--env,-e)"} {
			if result.exitCode != 0 || !strings.Contains(result.stdout, want) {
				t.Fatalf("help missing %q: %+v", want, result)
			}
		}
		canonical := runCLI(t, binary, []string{"content", "workbook", "publish", "--help"}, nil)
		if canonical.exitCode != 0 || result.stdout != canonical.stdout {
			t.Fatalf("shorthand help differs from canonical help: short=%+v canonical=%+v", result, canonical)
		}
	})
}

type processResult struct {
	exitCode int
	stdout   string
	stderr   string
}

func processSiteConfig(t *testing.T, consent bool) string {
	t.Helper()
	environment := config.Environment{URL: "https://tableau.example.invalid", SiteContentURL: "fixture-site", Auth: config.Auth{Type: config.AuthTypePAT}}
	configuration := config.Config{Version: config.CurrentVersion, DefaultEnvironment: "fixture", Environments: map[string]config.Environment{"fixture": environment}}
	if err := configuration.SetMutationSetting(environment, consent); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, configuration); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildCLI(t *testing.T) string {
	t.Helper()
	name := "tadx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	path := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-trimpath", "-o", path, ".")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/tadx: %v\n%s", err, output)
	}
	return path
}

func runCLI(t *testing.T, binary string, args []string, environment map[string]string) processResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// A recognized route must never fall through to the developer's installed
	// profile or its native-store credentials. Each process uses isolated fixture
	// configuration or an intentionally missing isolated path.
	isolation := t.TempDir()
	if !slices.Contains(args, "--config") {
		args = append(append([]string(nil), args...), "--config", filepath.Join(isolation, "missing.yaml"))
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir = isolation
	command.Env = environmentWithout("TADX_ENABLE_MUTATIONS")
	command.Env = append(command.Env, "TADX_GUIDANCE_NOTICE=0")
	for name, value := range environment {
		command.Env = append(command.Env, name+"="+value)
	}
	command.Stdin = strings.NewReader("")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		t.Fatalf("tadx process did not exit within 10 seconds: %v", ctx.Err())
	}
	exitCode := 0
	if err != nil {
		var exitError *exec.ExitError
		if !errors.As(err, &exitError) {
			t.Fatalf("run tadx process: %v", err)
		}
		exitCode = exitError.ExitCode()
	}
	return processResult{exitCode: exitCode, stdout: stdout.String(), stderr: stderr.String()}
}

func environmentWithout(name string) []string {
	result := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if !strings.EqualFold(key, name) && !strings.HasPrefix(upper, "TADX_") && !strings.Contains(upper, "_PAT") && !strings.HasPrefix(upper, "PAT_") && !strings.Contains(upper, "TABLEAU") {
			result = append(result, entry)
		}
	}
	return result
}

func decodeDocument(t *testing.T, value string) map[string]any {
	t.Helper()
	decoded, err := toon.Decode([]byte(value))
	if err != nil {
		t.Fatalf("stdout is not valid TOON: %v\n%s", err, value)
	}
	document, ok := decoded.(map[string]any)
	if !ok {
		t.Fatalf("stdout TOON root = %T, want object", decoded)
	}
	return document
}

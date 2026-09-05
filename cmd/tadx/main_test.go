package main_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

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

func TestCLIProcessRejectsRetiredRoutesAndLegacyProjectSelectors(t *testing.T) {
	binary := buildCLI(t)
	tests := []struct {
		name string
		args []string
	}{
		{name: "retired content get", args: []string{"content", "get"}},
		{name: "retired content search", args: []string{"content", "search"}},
		{name: "retired workbook get", args: []string{"content", "workbook", "get"}},
		{name: "retired catalog search", args: []string{"catalog", "search"}},
		{name: "retired admin user get", args: []string{"admin", "user", "get"}},
		{name: "retired Pulse metric get", args: []string{"pulse", "metric", "get"}},
		{name: "legacy project inspect ID", args: []string{"content", "project", "inspect", "--id", "project-1"}},
		{name: "legacy project update ID", args: []string{"content", "project", "update", "--id", "project-1"}},
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
		{"catalog"},
		{"admin", "user"},
		{"pulse", "metric"},
	} {
		result := runCLI(t, binary, args, nil)
		if result.exitCode != 0 || result.stderr != "" || !strings.Contains(result.stdout, "Usage:") {
			t.Fatalf("args = %v, exit = %d, stdout = %q, stderr = %q", args, result.exitCode, result.stdout, result.stderr)
		}
	}
}

func TestCLIProcessMutationDiscoveryEnvironment(t *testing.T) {
	binary := buildCLI(t)
	args := []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}

	disabled := runCLI(t, binary, args, nil)
	if disabled.exitCode != 0 || disabled.stderr != "" {
		t.Fatalf("disabled gate returned exit %d and stderr %q, want exit 0 and empty stderr\nstdout:\n%s", disabled.exitCode, disabled.stderr, disabled.stdout)
	}
	disabledDocument := decodeDocument(t, disabled.stdout)
	disabledCapabilities, ok := disabledDocument["capabilities"].([]any)
	if !ok || len(disabledCapabilities) != 4 {
		t.Fatalf("disabled mutation discovery = %#v", disabledDocument)
	}
	for _, raw := range disabledCapabilities {
		capability, rowOK := raw.(map[string]any)
		if !rowOK || capability["execution_enabled"] != false {
			t.Fatalf("disabled capability = %#v", raw)
		}
	}

	enabled := runCLI(t, binary, args, map[string]string{"TADX_ENABLE_MUTATIONS": "1"})
	if enabled.exitCode != 0 || enabled.stderr != "" {
		t.Fatalf("enabled gate returned exit %d and stderr %q, want exit 0 and empty stderr\nstdout:\n%s", enabled.exitCode, enabled.stderr, enabled.stdout)
	}
	enabledDocument := decodeDocument(t, enabled.stdout)
	capabilities, ok := enabledDocument["capabilities"].([]any)
	if !ok || len(capabilities) != 4 {
		t.Fatalf("enabled mutation discovery capabilities = %#v, want four workbook mutations", enabledDocument["capabilities"])
	}
	for index, want := range []string{"workbook.delete", "workbook.move", "workbook.publish", "workbook.update"} {
		capability, rowOK := capabilities[index].(map[string]any)
		if !rowOK || capability["id"] != want || capability["execution_enabled"] != true {
			t.Fatalf("enabled capability %d = %#v, want %s enabled", index, capabilities[index], want)
		}
	}
}

func TestCLIProcessMutationCommandsAreVisibleAndGated(t *testing.T) {
	binary := buildCLI(t)
	help := runCLI(t, binary, []string{"content", "workbook", "--help"}, nil)
	if help.exitCode != 0 || !strings.Contains(help.stdout, "publish") || !strings.Contains(help.stdout, "delete") {
		t.Fatalf("workbook help did not expose mutations: exit = %d, stdout = %s", help.exitCode, help.stdout)
	}

	result := runCLI(t, binary, []string{"content", "workbook", "delete", "--environment", "missing", "--id", "workbook-1"}, nil)
	document := decodeDocument(t, result.stdout)
	errorDocument, ok := document["error"].(map[string]any)
	if result.exitCode != 1 || !ok || errorDocument["id"] != "mutation.disabled" || errorDocument["operation"] != "workbook.delete" {
		t.Fatalf("mutation gate result = %#v, exit = %d", document, result.exitCode)
	}
}

type processResult struct {
	exitCode int
	stdout   string
	stderr   string
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
	command := exec.CommandContext(ctx, binary, args...)
	command.Env = environmentWithout("TADX_ENABLE_MUTATIONS")
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
		if !strings.EqualFold(key, name) {
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

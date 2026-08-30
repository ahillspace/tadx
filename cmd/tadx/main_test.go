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

func TestCLIProcessMutationDiscoveryEnvironment(t *testing.T) {
	binary := buildCLI(t)
	args := []string{"capability", "list", "--domain", "content", "--resource", "workbook", "--mutation=true"}

	disabled := runCLI(t, binary, args, nil)
	if disabled.exitCode != 2 || disabled.stderr != "" {
		t.Fatalf("disabled gate returned exit %d and stderr %q, want exit 2 and empty stderr\nstdout:\n%s", disabled.exitCode, disabled.stderr, disabled.stdout)
	}
	disabledDocument := decodeDocument(t, disabled.stdout)
	disabledError, ok := disabledDocument["error"].(map[string]any)
	if !ok || disabledError["kind"] != "usage" {
		t.Fatalf("disabled gate output = %#v, want structured usage error", disabledDocument)
	}
	if strings.Contains(disabled.stdout, "workbook.publish") {
		t.Fatalf("disabled mutation discovery exposed workbook.publish: %s", disabled.stdout)
	}

	enabled := runCLI(t, binary, args, map[string]string{"TADX_ENABLE_MUTATIONS": "1"})
	if enabled.exitCode != 0 || enabled.stderr != "" {
		t.Fatalf("enabled gate returned exit %d and stderr %q, want exit 0 and empty stderr\nstdout:\n%s", enabled.exitCode, enabled.stderr, enabled.stdout)
	}
	enabledDocument := decodeDocument(t, enabled.stdout)
	capabilities, ok := enabledDocument["capabilities"].([]any)
	if !ok || len(capabilities) != 1 {
		t.Fatalf("enabled mutation discovery capabilities = %#v, want one capability", enabledDocument["capabilities"])
	}
	capability, ok := capabilities[0].(map[string]any)
	if !ok || capability["id"] != "workbook.publish" {
		t.Fatalf("enabled mutation discovery result = %#v, want workbook.publish", capabilities[0])
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

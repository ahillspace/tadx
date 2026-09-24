package main_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCLIGuidanceNoticeIsSessionBoundAndLeavesStdoutAlone(t *testing.T) {
	binary := buildNoticeCLI(t)
	directory := t.TempDir()
	if err := os.Mkdir(filepath.Join(directory, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	environment := noticeEnvironment(directory, "session-one")
	first := runNoticeCLI(t, binary, directory, environment, []string{"capability", "list", "--domain", "capability"})
	if !strings.Contains(first.stderr, "Guidance was not detected") {
		t.Fatalf("first stderr = %q, want guidance notice", first.stderr)
	}
	if first.stdout == "" {
		t.Fatal("first stdout is empty")
	}
	second := runNoticeCLI(t, binary, directory, environment, []string{"capability", "list", "--domain", "capability"})
	if second.stderr != "" {
		t.Fatalf("second stderr = %q, want no repeated notice", second.stderr)
	}
	if second.stdout != first.stdout {
		t.Fatal("notice changed stdout")
	}
	newSession := noticeEnvironment(directory, "session-two")
	third := runNoticeCLI(t, binary, directory, newSession, []string{"capability", "list", "--domain", "capability"})
	if !strings.Contains(third.stderr, "Guidance was not detected") {
		t.Fatalf("new session stderr = %q, want guidance notice", third.stderr)
	}
	if err := os.MkdirAll(filepath.Join(directory, "home", ".codex", "skills", "tadx"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "home", ".codex", "skills", "tadx", "SKILL.md"), []byte("installed"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed := runNoticeCLI(t, binary, directory, noticeEnvironment(directory, "session-three"), []string{"capability", "list", "--domain", "capability"})
	if installed.stderr != "" {
		t.Fatalf("installed stderr = %q, want no notice", installed.stderr)
	}
	optedOut := noticeEnvironment(t.TempDir(), "session-four")
	optedOut = append(optedOut, "TADX_GUIDANCE_NOTICE=0")
	result := runNoticeCLI(t, binary, directory, optedOut, []string{"capability", "list", "--domain", "capability"})
	if result.stderr != "" {
		t.Fatalf("opt-out stderr = %q, want no notice", result.stderr)
	}
}

func buildNoticeCLI(t *testing.T) string {
	t.Helper()
	name := "tadx"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	command := exec.Command("go", "build", "-trimpath", "-o", binary, ".")
	command.Dir = "."
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("build cmd/tadx: %v\n%s", err, output)
	}
	return binary
}

func noticeEnvironment(directory, session string) []string {
	environment := environmentWithout("TADX_GUIDANCE_NOTICE")
	for _, name := range []string{"TADX_GUIDANCE_SESSION", "CODEX_HOME", "XDG_CONFIG_HOME", "PI_CODING_AGENT_DIR", "HERMES_HOME", "HOME", "USERPROFILE", "LOCALAPPDATA", "XDG_CACHE_HOME"} {
		environment = environmentWithoutFrom(environment, name)
	}
	home := filepath.Join(directory, "home")
	cache := filepath.Join(directory, "cache")
	return append(environment,
		"TADX_GUIDANCE_SESSION="+session,
		"HOME="+home,
		"USERPROFILE="+home,
		"LOCALAPPDATA="+cache,
		"XDG_CACHE_HOME="+cache,
	)
}

func environmentWithoutFrom(environment []string, name string) []string {
	result := environment[:0]
	for _, entry := range environment {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, name) {
			result = append(result, entry)
		}
	}
	return result
}

func runNoticeCLI(t *testing.T, binary, directory string, environment, args []string) processResult {
	t.Helper()
	args = append(append([]string(nil), args...), "--config", filepath.Join(directory, "missing.yaml"))
	command := exec.Command(binary, args...)
	command.Dir = directory
	command.Env = environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("run CLI: %v\nstdout: %s\nstderr: %s", err, stdout.String(), stderr.String())
	}
	ordinaryStderr, policyWarnings := separateAmbientPolicyWarnings(stderr.String())
	return processResult{stdout: stdout.String(), stderr: ordinaryStderr, policyWarnings: policyWarnings}
}

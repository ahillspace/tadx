package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAgentAutoInstallCLIReportsPortableTargetAndRefreshesEditedPackages(t *testing.T) {
	home := t.TempDir()
	opts := app.Options{ConfigPath: filepath.Join(home, "missing.yaml"), UserHomeDir: func() (string, error) { return home, nil }}
	run := func(args ...string) (int, string) {
		var out bytes.Buffer
		code := app.Run(context.Background(), args, &out, opts)
		return code, out.String()
	}
	if code, out := run("agent", "install", "--preview"); code != 0 || !strings.Contains(out, "generic") {
		t.Fatalf("preview: %d %s", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents")); !os.IsNotExist(err) {
		t.Fatalf("preview writes: %v", err)
	}
	if code, out := run("agent", "install"); code != 0 || !strings.Contains(out, "generic") {
		t.Fatalf("install: %d %s", code, out)
	}
	file := filepath.Join(home, ".agents", "skills", "tadx", "SKILL.md")
	if err := os.WriteFile(file, []byte("edited"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, out := run("agent", "install"); code != 0 || !strings.Contains(out, "replaced") {
		t.Fatalf("refresh: %d %s", code, out)
	}
	data, err := os.ReadFile(file)
	if err != nil || string(data) == "edited" {
		t.Fatal("owned Guidance was not refreshed")
	}
}

func TestAgentAutoInstallCLIPreservesCompletedTargetsOnFailure(t *testing.T) {
	home := t.TempDir()
	for _, target := range []string{".claude", ".codex"} {
		if err := os.MkdirAll(filepath.Join(home, target, "skills"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(home, ".codex", "skills", ".tadx-install.lock"), []byte("busy"), 0600); err != nil {
		t.Fatal(err)
	}
	opts := app.Options{ConfigPath: filepath.Join(home, "missing.yaml"), UserHomeDir: func() (string, error) { return home, nil }}
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"agent", "install"}, &out, opts)
	if code == 0 || !strings.Contains(out.String(), "partial") || !strings.Contains(out.String(), "claude") || !strings.Contains(out.String(), "agent.install.failed") {
		t.Fatalf("lost completion: %d %s", code, out.String())
	}
	if strings.Contains(out.String(), home) {
		t.Fatalf("home leaked: %s", out.String())
	}
	if _, err := os.Stat(filepath.Join(home, ".claude", "skills", "tadx", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}

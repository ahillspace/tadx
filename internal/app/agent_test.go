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

func TestAgentInstallThroughCLI(t *testing.T) {
	for target, directory := range map[string]string{"claude": ".claude", "codex": ".agents", "cursor": ".cursor"} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			options := app.Options{ConfigPath: filepath.Join(home, "config.yaml"), UserHomeDir: func() (string, error) { return home, nil }}
			run := func(extra ...string) (int, string) {
				t.Helper()
				var out bytes.Buffer
				exit := app.Run(context.Background(), append([]string{"agent", "install", "--target", target}, extra...), &out, options)
				if strings.Contains(out.String(), home) {
					t.Fatalf("output leaks runtime home: %s", out.String())
				}
				return exit, out.String()
			}
			if exit, out := run("--preview"); exit != 0 || !strings.Contains(out, "status: preview") {
				t.Fatalf("preview: %d %s", exit, out)
			}
			if _, err := os.Stat(filepath.Join(home, directory)); !os.IsNotExist(err) {
				t.Fatalf("preview wrote files: %v", err)
			}
			if exit, out := run(); exit != 0 || !strings.Contains(out, "status: installed") {
				t.Fatalf("install: %d %s", exit, out)
			}
			for _, skill := range []string{"tadx", "tadx-pulse"} {
				data, err := os.ReadFile(filepath.Join(home, directory, "skills", skill, "SKILL.md"))
				if err != nil || !bytes.Contains(data, []byte("name: "+skill+"\n")) {
					t.Fatalf("skill %s: %v", skill, err)
				}
			}
			if exit, out := run("--full"); exit != 0 || !strings.Contains(out, "status: unchanged") || !strings.Contains(out, "sha256") {
				t.Fatalf("repeat: %d %s", exit, out)
			}
			path := filepath.Join(home, directory, "skills", "tadx-pulse", "SKILL.md")
			if err := os.WriteFile(path, []byte("user edits"), 0o600); err != nil {
				t.Fatal(err)
			}
			if exit, out := run(); exit == 0 {
				t.Fatalf("divergent install succeeds: %s", out)
			}
			if data, _ := os.ReadFile(path); string(data) != "user edits" {
				t.Fatal("divergent file changed")
			}
			if exit, out := run("--force"); exit != 0 || !strings.Contains(out, "backup") {
				t.Fatalf("force: %d %s", exit, out)
			}
			if err := filepath.WalkDir(home, func(path string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.Name() == "AGENTS.md" || entry.Name() == "CLAUDE.md" || entry.Name() == "rules" {
					t.Errorf("unexpected instructions file: %s", entry.Name())
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentInstallRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"agent", "install"}, {"agent", "install", "--target", "../outside"}, {"agent", "install", "--target", "codex", "extra"}} {
		var out bytes.Buffer
		if exit := app.Run(context.Background(), args, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); exit == 0 {
			t.Fatalf("accepted %v: %s", args, out.String())
		}
	}
}

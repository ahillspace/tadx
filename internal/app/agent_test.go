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
	for target, directory := range map[string]string{
		"claude": ".claude", "codex": ".codex", "cursor": ".cursor",
		"opencode": ".config/opencode", "pi": ".pi/agent", "hermes": ".hermes",
		"copilot": ".copilot", "gemini": ".gemini", "cline": ".cline",
	} {
		t.Run(target, func(t *testing.T) {
			home := t.TempDir()
			configPath := filepath.Join(t.TempDir(), "config.yaml")
			options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) { return home, nil }}
			run := func(extra ...string) (int, string) {
				t.Helper()
				var out bytes.Buffer
				exit := app.Run(context.Background(), append([]string{"agent", "install", "--target", target}, extra...), &out, options)
				if agentOutputContainsPath(out.String(), home) {
					t.Fatalf("output leaks runtime home: %s", out.String())
				}
				if !agentOutputContainsPath(out.String(), configPath) {
					t.Fatalf("follow-up lost explicit configuration: %s", out.String())
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
			if exit, out := run(); exit != 0 || !strings.Contains(out, "backup") {
				t.Fatalf("owned replacement without force: %d %s", exit, out)
			}
			if data, _ := os.ReadFile(path); string(data) == "user edits" {
				t.Fatal("owned package was not refreshed")
			}
			for _, preview := range []bool{true, false} {
				args := []string{"agent", "uninstall", "--target", target}
				if preview {
					args = append(args, "--preview")
				}
				var out bytes.Buffer
				if exit := app.Run(context.Background(), args, &out, options); exit != 0 {
					t.Fatalf("uninstall preview=%v: %d %s", preview, exit, out.String())
				}
				if agentOutputContainsPath(out.String(), home) || !agentOutputContainsPath(out.String(), configPath) {
					t.Fatalf("uninstall output must exclude runtime home and preserve explicit config: %s", out.String())
				}
				for _, skill := range []string{"tadx", "tadx-pulse"} {
					_, err := os.Stat(filepath.Join(home, directory, "skills", skill, "SKILL.md"))
					if preview && err != nil || !preview && !os.IsNotExist(err) {
						t.Fatalf("uninstall preview=%v skill %s: %v", preview, skill, err)
					}
				}
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

// TOON and JSON escape Windows separators. Check the logical path rather than
// letting escaping hide the same disclosure that Linux reports directly.
func agentOutputContainsPath(output, location string) bool {
	normalize := func(value string) string {
		value = strings.ReplaceAll(value, `\\`, `\`)
		return strings.ReplaceAll(value, `\`, `/`)
	}
	return strings.Contains(normalize(output), normalize(location))
}

func TestAgentOutputPathCheckRecognizesPlatformAndEscapedSpellings(t *testing.T) {
	for _, example := range []struct{ output, location string }{
		{`path: /tmp/agent-home/skills`, `/tmp/agent-home`},
		{`path: C:\agent-home\skills`, `C:\agent-home`},
		{`"path":"C:\\agent-home\\skills"`, `C:\agent-home`},
		{`path: C:/agent-home/skills`, `C:\agent-home`},
	} {
		if !agentOutputContainsPath(example.output, example.location) {
			t.Fatalf("path detector missed %q in %q", example.location, example.output)
		}
	}
}

func TestAgentInstallRejectsInvalidArguments(t *testing.T) {
	for _, args := range [][]string{{"agent", "install", "--target", "../outside"}, {"agent", "install", "--target", "codex", "extra"}} {
		var out bytes.Buffer
		if exit := app.Run(context.Background(), args, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); exit == 0 {
			t.Fatalf("accepted %v: %s", args, out.String())
		}
	}
}

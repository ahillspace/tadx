package app_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/app"
)

func TestAgentAutoUpgradeKeepsFourteenBackupsWithoutRoutineWarningsThroughCLI(t *testing.T) {
	home := t.TempDir()
	targets := []string{"claude", "cline", "codex", "copilot", "cursor", "gemini", "hermes"}
	for _, target := range targets {
		base, ok := agenttarget.TargetPath(target)
		if !ok {
			t.Fatalf("unknown fixture target %q", target)
		}
		receipt := struct {
			Version  int               `json:"version"`
			Packages map[string]string `json:"packages"`
		}{Version: 1, Packages: make(map[string]string)}
		for _, name := range []string{"tadx", "tadx-pulse"} {
			content := []byte("name: " + name + "\nolder bundled release\n")
			location := filepath.Join(home, filepath.FromSlash(base), name, "SKILL.md")
			if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(location, content, 0o600); err != nil {
				t.Fatal(err)
			}
			record := fmt.Sprintf("SKILL.md\x00%x", sha256.Sum256(content))
			receipt.Packages[name] = fmt.Sprintf("%x", sha256.Sum256([]byte(record)))
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			t.Fatal(err)
		}
		location := filepath.Join(home, filepath.Dir(filepath.FromSlash(base)), ".tadx-skill-receipt.json")
		if err := os.WriteFile(location, encoded, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	configPath := filepath.Join(t.TempDir(), "missing.yaml")
	options := app.Options{ConfigPath: configPath, UserHomeDir: func() (string, error) { return home, nil }}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"agent", "install", "--json", "--full"}, &out, options)
	var result struct {
		Status   string   `json:"status"`
		Targets  []string `json:"targets"`
		Warnings []string `json:"warnings"`
		Skills   []struct {
			Target string `json:"target"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Backup string `json:"backup"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("CLI result: %s, error=%v", out.String(), err)
	}
	t.Logf("auto upgrade: targets=%d packages=%d warnings=%d", len(result.Targets), len(result.Skills), len(result.Warnings))
	if code != 0 || result.Status != "installed" || len(result.Targets) != 7 || len(result.Skills) != 14 || len(result.Warnings) != 0 {
		t.Errorf("upgrade receipt: code=%d status=%q targets=%d packages=%d warnings=%v", code, result.Status, len(result.Targets), len(result.Skills), result.Warnings)
	}
	for _, skill := range result.Skills {
		if skill.Status != "replaced" || !strings.Contains(skill.Backup, ".tadx-skill-backups/") || filepath.IsAbs(skill.Backup) || agentOutputContainsPath(skill.Backup, home) {
			t.Errorf("backup metadata: %#v", skill)
			continue
		}
		content, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(skill.Backup), "SKILL.md"))
		if err != nil || string(content) != "name: "+skill.Name+"\nolder bundled release\n" {
			t.Errorf("backup %s: content=%q err=%v", skill.Backup, content, err)
		}
	}
	if agentOutputContainsPath(out.String(), home) {
		t.Errorf("CLI output leaked home: %s", out.String())
	}
}

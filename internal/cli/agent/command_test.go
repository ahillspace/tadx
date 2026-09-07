package agent_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestCodexCLIMigratesAndUninstallsLegacyGuidance(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			options := app.Options{UserHomeDir: func() (string, error) { return home, nil }, ConfigPath: filepath.Join(home, "config.yaml")}
			run := func(args ...string) {
				t.Helper()
				var output bytes.Buffer
				if code := app.Run(context.Background(), args, &output, options); code != 0 {
					t.Fatalf("%v: exit %d: %s", args, code, output.String())
				}
			}
			run("agent", "install", "--target", "codex")
			if err := os.MkdirAll(filepath.Join(home, ".agents"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(home, ".codex", "skills"), filepath.Join(home, ".agents", "skills")); err != nil {
				t.Fatal(err)
			}
			run("agent", operation, "--target", "codex")
			for _, name := range []string{"tadx", "tadx-pulse"} {
				if _, err := os.Stat(filepath.Join(home, ".agents", "skills", name)); !os.IsNotExist(err) {
					t.Fatalf("legacy %s remains discoverable: %v", name, err)
				}
				_, err := os.Stat(filepath.Join(home, ".codex", "skills", name, "SKILL.md"))
				if operation == "install" && err != nil || operation == "uninstall" && !os.IsNotExist(err) {
					t.Fatalf("canonical %s: %v", name, err)
				}
			}
		})
	}
}

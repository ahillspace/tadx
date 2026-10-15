package app_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestConfigFlagIsolatesSavedResultAndReplay(t *testing.T) {
	ordinary := t.TempDir()
	selected := t.TempDir()
	path := filepath.Join(selected, "config.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nenvironments:\n  isolated:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := app.Options{ConfigPath: filepath.Join(ordinary, "config.yaml"), Stderr: io.Discard}
	var out strings.Builder
	if code := app.Run(t.Context(), []string{"--config", path, "auth", "status", "--env", "isolated", "--json"}, &out, options); code != 0 {
		t.Fatalf("isolated status code=%d: %s", code, out.String())
	}
	if _, err := os.Stat(filepath.Join(ordinary, "last-result.json")); !os.IsNotExist(err) {
		t.Fatal("explicit --config wrote a saved result into the initial config location")
	}
	if _, err := os.Stat(filepath.Join(selected, "last-result.json")); err != nil {
		t.Fatal("explicit --config did not save its result in the selected location", err)
	}
	out.Reset()
	if code := app.Run(t.Context(), []string{"--config", path, "last", "--full", "--json"}, &out, options); code != 0 || !strings.Contains(out.String(), "isolated") {
		t.Fatalf("replay did not use the selected config location: code=%d %s", code, out.String())
	}
}

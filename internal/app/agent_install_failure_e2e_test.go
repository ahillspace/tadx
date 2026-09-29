package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAgentInstallFailureRendersOnlyTheErrorThroughCLI(t *testing.T) {
	home := t.TempDir()
	// A regular file where the agent configuration directory belongs fails
	// before any package is staged.
	if err := os.WriteFile(filepath.Join(home, ".claude"), []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), UserHomeDir: func() (string, error) { return home, nil }}

	var compact bytes.Buffer
	if code := app.Run(t.Context(), []string{"agent", "install", "--target", "claude"}, &compact, options); code == 0 {
		t.Fatalf("install succeeded: %s", compact.String())
	}
	if strings.Contains(compact.String(), "output:") || strings.Contains(compact.String(), `status: ""`) || !strings.Contains(compact.String(), "id: agent.install.failed") {
		t.Fatalf("compact failure:\n%s", compact.String())
	}

	var encoded bytes.Buffer
	if code := app.Run(t.Context(), []string{"agent", "install", "--target", "claude", "--json"}, &encoded, options); code == 0 {
		t.Fatalf("install succeeded: %s", encoded.String())
	}
	var document map[string]json.RawMessage
	if err := json.Unmarshal(encoded.Bytes(), &document); err != nil {
		t.Fatalf("decode %q: %v", encoded.String(), err)
	}
	if _, ok := document["output"]; ok || len(document) != 1 {
		t.Fatalf("JSON failure carries more than the error: %s", encoded.String())
	}
	var failure struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(document["error"], &failure); err != nil || failure.ID != "agent.install.failed" {
		t.Fatalf("error = %s, %v", document["error"], err)
	}
	for _, rendered := range []string{compact.String(), encoded.String()} {
		if agentOutputContainsPath(rendered, home) {
			t.Fatalf("output leaks runtime home: %s", rendered)
		}
	}
}

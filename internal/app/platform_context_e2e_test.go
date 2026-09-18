package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestEnvironmentGetRetainsRequestedProfileFacts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.yaml")
	data := "version: 1\nenvironments:\n  dev:\n    url: https://example.test\n    api_version: '3.29'\n    auth:\n      type: pat\n      pat_name_env: DEV_NAME\n      pat_secret_env: DEV_SECRET\n"
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"env", "get", "dev", "--json"}, &out, app.Options{ConfigPath: path})
	var result struct{ Environment map[string]any }
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 0 || result.Environment["api_version"] != "3.29" || result.Environment["auth_type"] != "pat" || result.Environment["pat_name_env"] != "DEV_NAME" || result.Environment["cache_max_concurrency"] != float64(tableaucache.DefaultMaxConcurrency) {
		t.Fatalf("code=%d output=%s", code, out.Bytes())
	}
	if !strings.Contains(out.String(), "last --full --json") {
		t.Fatalf("missing saved expansion: %s", out.Bytes())
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != data {
		t.Fatalf("read changed saved configuration: %v", err)
	}
}

func TestWorkspaceManifestAsConfigExplainsCorrectRoute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tadx.yaml")
	if err := os.WriteFile(path, []byte("version: 1\nworkspace:\n  name: test-workspace\n  id: ws_0123456789abcdef0123456789abcdef\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"env", "list", "--json"}, &out, app.Options{ConfigPath: path})
	if code == 0 || !strings.Contains(out.String(), "workspace manifest") || !strings.Contains(out.String(), "--workspace") || !strings.Contains(out.String(), "CLI settings") {
		t.Fatalf("code=%d output=%s", code, out.Bytes())
	}
}

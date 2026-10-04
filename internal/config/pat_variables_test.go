package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredPATVariables(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := "version: 1\nenvironments:\n  production:\n    url: https://tableau.example.com\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_UPDATE_PAT_NAME\n      pat_secret_env: PROD_UPDATE_PAT_SECRET\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(ConfiguredPATVariables(path), ",")
	if got != "PROD_UPDATE_PAT_NAME,PROD_UPDATE_PAT_SECRET" {
		t.Fatalf("configured PAT variables = %q", got)
	}
	if missing := ConfiguredPATVariables(filepath.Join(t.TempDir(), "missing.yaml")); len(missing) != 0 {
		t.Fatalf("missing configuration listed %q", missing)
	}
}

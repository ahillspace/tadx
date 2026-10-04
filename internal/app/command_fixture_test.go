package app_test

import (
	"context"
	"fmt"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
	"os"
	"path/filepath"
	"testing"
)

func writeCLIConfig(t *testing.T, serverURL string) string {
	t.Helper()
	return writeCLIConfigWithSite(t, serverURL, "")
}

func writeCLIConfigWithSite(t *testing.T, serverURL, site string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: %q\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", serverURL, site)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func createNamedWorkspace(t *testing.T, configPath, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if _, err := workspacecore.NewManager(configPath, nil).Create(context.Background(), name, root); err != nil {
		t.Fatal(err)
	}
	return root
}

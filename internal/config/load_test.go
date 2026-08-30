package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestLoadReadsNonSecretEnvironmentProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
default_environment: production
environments:
  production:
    url: https://example.test
    site_content_url: marketing
    api_version: "3.29"
    auth:
      type: pat
      pat_name_env: PROD_PAT_NAME
      pat_secret_env: PROD_PAT_SECRET
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := loaded.ResolveEnvironment("")
	if err != nil {
		t.Fatal(err)
	}
	if environment.Alias != "production" || environment.APIVersion != "3.29" || environment.Auth.PATSecretEnv != "PROD_PAT_SECRET" {
		t.Fatalf("environment = %#v", environment)
	}
}

func TestLoadRejectsPersistedPATValues(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
environments:
  production:
    url: https://example.test
    auth:
      type: pat
      pat_name: forbidden
      pat_secret: forbidden
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() accepted persisted PAT values")
	}
}

func TestLoadRejectsTrailingYAMLDocument(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := `version: 1
environments: {}
---
version: 1
default_environment: production
environments:
  production:
    url: https://unexpected.example.test
    auth:
      type: pat
`
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err == nil {
		t.Fatal("Load() accepted multiple YAML documents")
	}
}

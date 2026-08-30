package config_test

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func TestDefaultPATVariableNames(t *testing.T) {
	t.Parallel()

	name, secret := config.DefaultPATVariableNames("production-us.east")
	if name != "TADX_PRODUCTION_US_EAST_PAT_NAME" {
		t.Fatalf("name = %q", name)
	}
	if secret != "TADX_PRODUCTION_US_EAST_PAT_SECRET" {
		t.Fatalf("secret = %q", secret)
	}
}

func TestConfigValidateAcceptsNonSecretPATReferences(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version:            config.CurrentVersion,
		DefaultEnvironment: "production",
		DefaultWorkspace:   "./workspaces/default",
		Environments: map[string]config.Environment{
			"production": {
				URL:            "https://example.tableau.com",
				SiteContentURL: "example-site",
				Auth: config.Auth{
					Type:         config.AuthTypePAT,
					PATNameEnv:   "TADX_PRODUCTION_PAT_NAME",
					PATSecretEnv: "TADX_PRODUCTION_PAT_SECRET",
				},
				DefaultWorkspace: "./workspaces/prod",
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsInvalidModels(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  config.Config
	}{
		{name: "unsupported version", cfg: config.Config{Version: 2}},
		{name: "missing default", cfg: config.Config{Version: 1, DefaultEnvironment: "missing", Environments: map[string]config.Environment{}}},
		{name: "non PAT auth", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://example.com", Auth: config.Auth{Type: "oauth"}}}}},
		{name: "invalid URL", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "URL has credentials", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://user:secret@example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "empty alias", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "same PAT variable", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "PAT", PATSecretEnv: "PAT"}}}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := tt.cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil")
			}
		})
	}
}

func TestResolveEnvironmentAppliesDefaultPATReferencesWithoutMutatingConfig(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version:            config.CurrentVersion,
		DefaultEnvironment: "production-us",
		Environments: map[string]config.Environment{
			"production-us": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT}},
		},
	}

	got, err := cfg.ResolveEnvironment("")
	if err != nil {
		t.Fatalf("ResolveEnvironment() error = %v", err)
	}
	if got.Alias != "production-us" || got.Auth.PATNameEnv != "TADX_PRODUCTION_US_PAT_NAME" || got.Auth.PATSecretEnv != "TADX_PRODUCTION_US_PAT_SECRET" {
		t.Fatalf("ResolveEnvironment() = %#v", got)
	}
	if cfg.Environments["production-us"].Auth.PATNameEnv != "" {
		t.Fatal("ResolveEnvironment() mutated the source configuration")
	}
}

func TestConfigModelContainsNoSecretValueField(t *testing.T) {
	t.Parallel()

	for _, typ := range []reflect.Type{reflect.TypeFor[config.Config](), reflect.TypeFor[config.Environment](), reflect.TypeFor[config.Auth]()} {
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.Name == "PATName" || field.Name == "PATSecret" || field.Name == "Token" {
				t.Fatalf("%s contains secret-bearing field %s", typ, field.Name)
			}
		}
	}
}

func TestPathContracts(t *testing.T) {
	t.Parallel()

	got := config.UserConfigPathFrom("root")
	want := filepath.Join("root", "tadx", "config.yaml")
	if got != want {
		t.Fatalf("UserConfigPathFrom() = %q, want %q", got, want)
	}
	if config.WorkspaceConfigName != "tadx.yaml" {
		t.Fatalf("WorkspaceConfigName = %q", config.WorkspaceConfigName)
	}
}

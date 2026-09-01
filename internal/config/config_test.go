package config_test

import (
	"path/filepath"
	"reflect"
	"strings"
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
		DefaultWorkspace:   "default",
		Workspaces: map[string]config.WorkspaceRegistration{
			"default": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join("workspaces", "default")},
			"prod":    {ID: "ws_22222222222222222222222222222222", Path: filepath.Join("workspaces", "prod")},
		},
		Environments: map[string]config.Environment{
			"production": {
				URL:            "https://example.tableau.com",
				SiteContentURL: "example-site",
				Auth: config.Auth{
					Type:         config.AuthTypePAT,
					PATNameEnv:   "TADX_PRODUCTION_PAT_NAME",
					PATSecretEnv: "TADX_PRODUCTION_PAT_SECRET",
				},
				DefaultWorkspace: "prod",
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
		{name: "plaintext URL", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "http://example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "URL has credentials", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://user:secret@example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "invalid API version", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://example.com", APIVersion: "3.29?x", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "empty alias", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT}}}}},
		{name: "same PAT variable", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "PAT", PATSecretEnv: "PAT"}}}}},
		{name: "case-only same PAT variable", cfg: config.Config{Version: 1, Environments: map[string]config.Environment{"x": {URL: "https://example.com", Auth: config.Auth{Type: config.AuthTypePAT, PATNameEnv: "PAT", PATSecretEnv: "pat"}}}}},
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

func TestConfigValidateRejectsCollidingDefaultPATVariables(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version: config.CurrentVersion,
		Environments: map[string]config.Environment{
			"prod-us": {URL: "https://prod-us.example.com", Auth: config.Auth{Type: config.AuthTypePAT}},
			"prod_us": {URL: "https://prod-us-2.example.com", Auth: config.Auth{Type: config.AuthTypePAT}},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("Validate() error = nil")
	}
	for _, value := range []string{"prod-us", "prod_us", "TADX_PROD_US_PAT_NAME", "TADX_PROD_US_PAT_SECRET"} {
		if !strings.Contains(err.Error(), value) {
			t.Fatalf("Validate() error = %q, want collision context %q", err, value)
		}
	}
}

func TestConfigValidateAllowsExplicitNonCollidingPATVariables(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version: config.CurrentVersion,
		Environments: map[string]config.Environment{
			"prod-us": {URL: "https://prod-us.example.com", Auth: config.Auth{Type: config.AuthTypePAT}},
			"prod_us": {
				URL: "https://prod-us-2.example.com",
				Auth: config.Auth{
					Type:         config.AuthTypePAT,
					PATNameEnv:   "SECOND_PROD_PAT_NAME",
					PATSecretEnv: "SECOND_PROD_PAT_SECRET",
				},
			},
		},
	}

	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsCaseOnlyCrossEnvironmentCollision(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version: config.CurrentVersion,
		Environments: map[string]config.Environment{
			"prod": {URL: "https://prod.example.com", Auth: config.Auth{Type: config.AuthTypePAT}},
			"other": {
				URL: "https://other.example.com",
				Auth: config.Auth{
					Type:         config.AuthTypePAT,
					PATNameEnv:   "tadx_prod_pat_name",
					PATSecretEnv: "OTHER_SECRET",
				},
			},
		},
	}

	if err := cfg.Validate(); err == nil {
		t.Fatal("Validate() error = nil")
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

func TestConfigValidateRejectsCaseInsensitiveWorkspaceCollisions(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version: config.CurrentVersion,
		Workspaces: map[string]config.WorkspaceRegistration{
			"Development": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join("root", "one")},
			"development": {ID: "ws_22222222222222222222222222222222", Path: filepath.Join("root", "two")},
		},
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "case-insensitive") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsDuplicateWorkspaceIdentityAndCanonicalRoot(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "workspace")
	cfg := config.Config{
		Version: config.CurrentVersion,
		Workspaces: map[string]config.WorkspaceRegistration{
			"one": {ID: "ws_11111111111111111111111111111111", Path: root},
			"two": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join(root, ".")},
		},
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "workspace ID") || !strings.Contains(err.Error(), "canonical root") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestConfigValidateRejectsCaseOnlyCanonicalRootCollisionOnEveryPlatform(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	cfg := config.Config{
		Version: config.CurrentVersion,
		Workspaces: map[string]config.WorkspaceRegistration{
			"one": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join(root, "Development")},
			"two": {ID: "ws_22222222222222222222222222222222", Path: filepath.Join(root, "development")},
		},
	}

	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "canonical root") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResolveWorkspaceUsesLogicalDefaults(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		Version:          config.CurrentVersion,
		DefaultWorkspace: "Development",
		Workspaces: map[string]config.WorkspaceRegistration{
			"development": {ID: "ws_11111111111111111111111111111111", Path: filepath.Join("root", "workspace")},
		},
	}

	name, registration, err := cfg.ResolveWorkspace("")
	if err != nil {
		t.Fatal(err)
	}
	if name != "development" || registration.ID != "ws_11111111111111111111111111111111" {
		t.Fatalf("ResolveWorkspace() = %q, %#v", name, registration)
	}
}

package env_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	profileupdate "github.com/ahillspace/tadx/actions/env"
	"github.com/ahillspace/tadx/internal/config"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestEnvironmentUpdateRejectsCredentialTargetChange(t *testing.T) {
	t.Parallel()
	path := credentialEnvironmentConfig(t)
	store := profileupdate.NewConfigStore(func() string { return path }, tableaucache.DefaultMaxConcurrency)

	for _, test := range []struct {
		name  string
		patch profileupdate.Patch
	}{
		{name: "server URL", patch: profileupdate.Patch{ServerURL: profileupdate.StringField{Set: true, Value: "https://other.example.test"}}},
		{name: "site", patch: profileupdate.Patch{SiteContentURL: profileupdate.StringField{Set: true, Value: "other-site"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := store.Update(context.Background(), "dev", test.patch)
			if err == nil || !strings.Contains(err.Error(), "auth logout") {
				t.Fatalf("Update() error = %v, want auth logout guidance", err)
			}
		})
	}
}

func TestEnvironmentRemoveRejectsStoredCredential(t *testing.T) {
	t.Parallel()
	path := credentialEnvironmentConfig(t)
	store := profileupdate.NewConfigStore(func() string { return path }, tableaucache.DefaultMaxConcurrency)

	err := store.Remove(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "auth logout") {
		t.Fatalf("Remove() error = %v, want auth logout guidance", err)
	}
}

func TestEnvironmentGetAndListPreserveEffectiveAndStoredDefaults(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := config.Save(path, config.Config{
		Version:            config.CurrentVersion,
		DefaultEnvironment: "dev",
		Environments: map[string]config.Environment{
			"dev": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
		},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	store := profileupdate.NewConfigStore(func() string { return path }, tableaucache.DefaultMaxConcurrency)
	got, err := store.Get(t.Context(), "dev")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	listed, err := store.List(t.Context())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(listed) != 1 || !got.Default || !listed[0].Default {
		t.Fatalf("Get() = %+v, List() = %+v, want one default profile", got, listed)
	}
	if got.DefaultWorkspace != "" || got.CacheMaxConcurrency != 32 {
		t.Fatalf("Get() defaults = %+v, want effective workspace and cache concurrency", got)
	}
	if listed[0].DefaultWorkspace != "" || listed[0].CacheMaxConcurrency != 0 {
		t.Fatalf("List() defaults = %+v, want stored environment fields", listed[0])
	}
}

func credentialEnvironmentConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := config.Save(path, config.Config{
		Version: config.CurrentVersion,
		Environments: map[string]config.Environment{
			"dev": {
				URL:            "https://tableau.example.test",
				SiteContentURL: "test-site",
				Auth: config.Auth{
					Type:          config.AuthTypePAT,
					CredentialRef: "cred_0123456789abcdef0123456789abcdef",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return path
}

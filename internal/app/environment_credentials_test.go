package app

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	"github.com/ahillspace/tadx/internal/config"
)

func TestEnvironmentUpdateRejectsCredentialTargetChange(t *testing.T) {
	t.Parallel()
	path := credentialEnvironmentConfig(t)
	store := configProfileStore{path: &path}

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
	store := configProfileStore{path: &path}

	err := store.Remove(context.Background(), "dev")
	if err == nil || !strings.Contains(err.Error(), "auth logout") {
		t.Fatalf("Remove() error = %v, want auth logout guidance", err)
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

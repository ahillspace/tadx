//go:build unix

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	authlogin "github.com/ahillspace/tadx/actions/auth"
	authlogout "github.com/ahillspace/tadx/actions/auth"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

// unreadableConfigDirectory lets the configuration rename succeed while the
// following directory sync cannot open the directory.
func unreadableConfigDirectory(t *testing.T, path string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory read permission")
	}
	directory := filepath.Dir(path)
	if err := os.Chmod(directory, 0o300); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(directory, 0o700) })
}

func loadAfterDurabilityFailure(t *testing.T, path string) config.Config {
	t.Helper()
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	return loaded
}

func TestAuthCredentialStoreLoginKeepsStoresConsistentWhenDirectorySyncFails(t *testing.T) {
	reference := coreauth.CredentialReference("cred_66666666666666666666666666666666")
	path := authConfig(t, "")
	store := &fakePATStore{next: reference}
	service := authlogin.NewCredentialPersistence(path, store, processEnvironment{})
	unreadableConfigDirectory(t, path)

	_, err := service.Store(context.Background(), authlogin.LoginTarget{Environment: "dev", ServerURL: "https://tableau.example.test", SiteContentURL: "test-site"}, authlogin.LoginCredential{PATName: "name", PATSecret: "secret"})
	if err == nil {
		t.Fatal("Store() error = nil after the configuration directory could not be synced")
	}
	loaded := loadAfterDurabilityFailure(t, path)
	got := loaded.Environments["dev"].Auth.CredentialRef
	_, credentialExists := store.records[reference]
	if got != "" || credentialExists {
		t.Fatalf("login failure left CredentialRef=%q with stored credential present=%t; want the prior configuration and no credential", got, credentialExists)
	}
}

func TestAuthCredentialStoreLogoutKeepsStoresConsistentWhenDirectorySyncFails(t *testing.T) {
	reference := coreauth.CredentialReference("cred_77777777777777777777777777777777")
	path := authConfig(t, string(reference))
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{
		reference: {Name: "name", Secret: "secret", Source: coreauth.CredentialSourceOSKeyring},
	}}
	service := authlogin.NewCredentialPersistence(path, store, processEnvironment{})
	unreadableConfigDirectory(t, path)

	if _, err := service.Remove(context.Background(), authlogout.LogoutTarget{Environment: "dev"}); err == nil {
		t.Fatal("Remove() error = nil after the configuration directory could not be synced")
	}
	loaded := loadAfterDurabilityFailure(t, path)
	got := loaded.Environments["dev"].Auth.CredentialRef
	_, credentialExists := store.records[reference]
	if got != string(reference) || !credentialExists {
		t.Fatalf("logout failure left CredentialRef=%q with stored credential present=%t; want the prior reference and credential", got, credentialExists)
	}
}

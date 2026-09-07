package app

import (
	"context"
	"path/filepath"
	"testing"

	authlogin "github.com/ahillspace/tadx/actions/auth/login"
	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

type fakePATStore struct {
	next      coreauth.CredentialReference
	records   map[coreauth.CredentialReference]coreauth.PATCredentials
	deleted   []coreauth.CredentialReference
	deleteErr error
}

func (s *fakePATStore) StorePAT(_ context.Context, _ coreauth.CredentialTarget, name, secret string) (coreauth.CredentialReference, error) {
	if s.records == nil {
		s.records = make(map[coreauth.CredentialReference]coreauth.PATCredentials)
	}
	s.records[s.next] = coreauth.PATCredentials{Name: name, Secret: secret, Source: coreauth.CredentialSourceOSKeyring}
	return s.next, nil
}

func (s *fakePATStore) ReplacePAT(_ context.Context, reference coreauth.CredentialReference, _ coreauth.CredentialTarget, name, secret string) error {
	if s.records == nil {
		s.records = make(map[coreauth.CredentialReference]coreauth.PATCredentials)
	}
	s.records[reference] = coreauth.PATCredentials{Name: name, Secret: secret, Source: coreauth.CredentialSourceOSKeyring}
	return nil
}

func (s *fakePATStore) LoadPAT(_ context.Context, reference coreauth.CredentialReference, _ coreauth.CredentialTarget) (coreauth.PATCredentials, error) {
	credential, ok := s.records[reference]
	if !ok {
		return coreauth.PATCredentials{}, &coreauth.CredentialStoreError{Kind: coreauth.CredentialStoreNotFound, Operation: "load"}
	}
	return credential, nil
}

func (s *fakePATStore) DeletePAT(_ context.Context, reference coreauth.CredentialReference) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	if _, ok := s.records[reference]; !ok {
		return &coreauth.CredentialStoreError{Kind: coreauth.CredentialStoreNotFound, Operation: "delete"}
	}
	delete(s.records, reference)
	s.deleted = append(s.deleted, reference)
	return nil
}

func TestAuthCredentialStoreLogoutRestoresReferenceWhenDeletionFails(t *testing.T) {
	reference := coreauth.CredentialReference("cred_55555555555555555555555555555555")
	path := authConfig(t, string(reference))
	store := &fakePATStore{
		records: map[coreauth.CredentialReference]coreauth.PATCredentials{
			reference: {Name: "name", Secret: "secret", Source: coreauth.CredentialSourceOSKeyring},
		},
		deleteErr: &coreauth.CredentialStoreError{Kind: coreauth.CredentialStoreDenied, Operation: "delete"},
	}
	service := authCredentialStore{runtime: &runtimeDependencies{configPath: path, patStore: store}}

	if _, err := service.Remove(context.Background(), authlogout.Target{Environment: "dev"}); err == nil {
		t.Fatal("Remove() error = nil")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.Environments["dev"].Auth.CredentialRef; got != string(reference) {
		t.Fatalf("CredentialRef = %q, want restored %q", got, reference)
	}
}

func TestAuthCredentialStorePersistsOpaqueReferenceAndReplacesOldPAT(t *testing.T) {
	oldReference := coreauth.CredentialReference("cred_00000000000000000000000000000000")
	path := authConfig(t, string(oldReference))
	store := &fakePATStore{
		records: map[coreauth.CredentialReference]coreauth.PATCredentials{
			oldReference: {Name: "old", Secret: "old-secret", Source: coreauth.CredentialSourceOSKeyring},
		},
	}
	service := authCredentialStore{runtime: &runtimeDependencies{configPath: path, patStore: store}}

	_, err := service.Store(context.Background(), authlogin.Target{Environment: "dev", ServerURL: "https://tableau.example.test", SiteContentURL: "test-site"}, authlogin.Credential{PATName: "new", PATSecret: "new-secret"})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.Environments["dev"].Auth.CredentialRef; got != string(oldReference) {
		t.Fatalf("CredentialRef = %q, want stable %q", got, oldReference)
	}
	credential := store.records[oldReference]
	if credential.Name != "new" || credential.Secret != "new-secret" {
		t.Fatal("stored credential was not replaced in place")
	}
}

func TestAuthCredentialStoreLogoutDeletesPATAndReference(t *testing.T) {
	reference := coreauth.CredentialReference("cred_22222222222222222222222222222222")
	path := authConfig(t, string(reference))
	store := &fakePATStore{
		records: map[coreauth.CredentialReference]coreauth.PATCredentials{
			reference: {Name: "name", Secret: "secret", Source: coreauth.CredentialSourceOSKeyring},
		},
	}
	service := authCredentialStore{runtime: &runtimeDependencies{configPath: path, patStore: store}}

	result, err := service.Remove(context.Background(), authlogout.Target{Environment: "dev"})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !result.Removed {
		t.Fatal("Remove() reported unchanged")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.Environments["dev"].Auth.CredentialRef; got != "" {
		t.Fatalf("CredentialRef = %q, want empty", got)
	}
	if _, exists := store.records[reference]; exists {
		t.Fatal("stored credential was not deleted")
	}
}

func TestAuthCredentialStoreLogoutClearsStaleReference(t *testing.T) {
	reference := coreauth.CredentialReference("cred_44444444444444444444444444444444")
	path := authConfig(t, string(reference))
	store := &fakePATStore{records: make(map[coreauth.CredentialReference]coreauth.PATCredentials)}
	service := authCredentialStore{runtime: &runtimeDependencies{configPath: path, patStore: store}}

	result, err := service.Remove(context.Background(), authlogout.Target{Environment: "dev"})
	if err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if !result.Removed {
		t.Fatal("Remove() reported unchanged")
	}
	loaded, err := config.Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := loaded.Environments["dev"].Auth.CredentialRef; got != "" {
		t.Fatalf("CredentialRef = %q, want empty", got)
	}
}

func authConfig(t *testing.T, credentialReference string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	err := config.Save(path, config.Config{
		Version: config.CurrentVersion,
		Environments: map[string]config.Environment{
			"dev": {
				URL:            "https://tableau.example.test",
				SiteContentURL: "test-site",
				Auth:           config.Auth{Type: config.AuthTypePAT, CredentialRef: credentialReference},
			},
		},
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	return path
}

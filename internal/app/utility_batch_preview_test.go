package app

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

func TestCacheRefreshPreviewPlansScopesWithoutAuthenticationOrStorage(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(500) }))
	defer server.Close()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(path, config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"alpha": {URL: server.URL, SiteContentURL: "a", Auth: config.Auth{Type: config.AuthTypePAT}},
		"beta":  {URL: server.URL, SiteContentURL: "b", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := Run(context.Background(), []string{"cache", "refresh", "--environment", "alpha", "--scope", "views", "--preview", "--json"}, &output, Options{ConfigPath: path, HTTPClient: server.Client()})
	if code != 0 || requests != 0 || !strings.Contains(output.String(), `"requested_scopes":["views"]`) || !strings.Contains(output.String(), `"implicit_scopes":["projects","workbooks"]`) {
		t.Fatalf("code=%d requests=%d output=%s", code, requests, &output)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("config changed err=%v", err)
	}
	err = filepath.WalkDir(filepath.Dir(path), func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (strings.HasSuffix(path, ".sqlite") || strings.HasSuffix(path, ".db")) {
			t.Errorf("preview created cache database %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAuthLogoutResolverObservesRemovalAfterConfigurationSnapshot(t *testing.T) {
	reference := coreauth.CredentialReference("cred_88888888888888888888888888888888")
	path := authConfig(t, string(reference))
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
	runtime, err := newRuntime(Options{ConfigPath: path, PATStore: store})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if _, err := runtime.configuration(); err != nil {
		t.Fatal(err)
	}
	resolver := authLogoutResolver{runtime: runtime}
	before, err := resolver.Resolve(context.Background(), "dev")
	if err != nil || !before.StoredCredentialReferencePresent {
		t.Fatalf("before=%#v err=%v", before, err)
	}
	if _, err := (authCredentialStore{runtime: runtime}).Remove(context.Background(), authlogout.Target{Environment: "dev"}); err != nil {
		t.Fatal(err)
	}
	after, err := resolver.Resolve(context.Background(), "dev")
	if err != nil || after.StoredCredentialReferencePresent {
		t.Fatalf("after=%#v err=%v", after, err)
	}
}

func TestAuthLogoutPreviewPreservesCredentialsThenExecutionRemovesOnlySelectedReference(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	reference := coreauth.CredentialReference("cred_66666666666666666666666666666666")
	otherReference := coreauth.CredentialReference("cred_77777777777777777777777777777777")
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}, otherReference: {Name: "fixture-name", Secret: "fixture-secret"}}}
	if err := config.Save(path, config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"alpha": {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT, CredentialRef: string(reference)}},
		"beta":  {URL: "https://tableau.example.test", Auth: config.Auth{Type: config.AuthTypePAT, CredentialRef: string(otherReference)}},
	}}); err != nil {
		t.Fatal(err)
	}
	options := Options{ConfigPath: path, PATStore: store}
	args := []string{"auth", "logout", "--environment", "alpha", "--json"}
	var output bytes.Buffer
	file := filepath.Join(t.TempDir(), "logout.json")
	if err := os.WriteFile(file, []byte(`{"items":[{"environment":"alpha"},{"environment":"beta"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	code := Run(context.Background(), append(append([]string(nil), args...), "--batch-file", file), &output, options)
	if code == 0 || len(store.deleted) != 0 || !strings.Contains(output.String(), "unknown flag: --batch-file") {
		t.Fatalf("batch rejection code=%d deleted=%v output=%s", code, store.deleted, &output)
	}
	output.Reset()
	code = Run(context.Background(), append(append([]string(nil), args...), "--preview"), &output, options)
	if code != 0 || len(store.deleted) != 0 || !bytes.Contains(output.Bytes(), []byte(`"status":"preview"`)) || !bytes.Contains(output.Bytes(), []byte(`"stored_credential_reference_present":true`)) {
		t.Fatalf("preview code=%d deleted=%v output=%s", code, store.deleted, &output)
	}
	configuration, err := config.Load(path)
	if err != nil || configuration.Environments["alpha"].Auth.CredentialRef != string(reference) || configuration.Environments["beta"].Auth.CredentialRef != string(otherReference) {
		t.Fatalf("preview changed config: %#v err=%v", configuration, err)
	}
	output.Reset()
	code = Run(context.Background(), args, &output, options)
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code != 0 || len(store.deleted) != 1 || store.deleted[0] != reference || result.Status != "removed" {
		t.Fatalf("execute code=%d deleted=%v output=%s", code, store.deleted, &output)
	}
	configuration, err = config.Load(path)
	if err != nil || configuration.Environments["alpha"].Auth.CredentialRef != "" || configuration.Environments["beta"].Auth.CredentialRef != string(otherReference) {
		t.Fatalf("execute config=%#v err=%v", configuration, err)
	}
	if strings.Contains(output.String(), "fixture-secret") || strings.Contains(output.String(), string(reference)) {
		t.Fatal("credential data leaked")
	}
}

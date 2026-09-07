//go:build livekeyring

package auth_test

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
)

func TestLiveOSPATStoreRoundTrip(t *testing.T) {
	store := auth.NewOSPATStore()
	target := auth.CredentialTarget{ServerURL: "https://example.test", SiteContentURL: "credential-store-smoke"}
	reference, err := store.StorePAT(context.Background(), target, "synthetic-name", "synthetic-secret")
	if err != nil {
		t.Fatalf("StorePAT() error = %v", err)
	}
	t.Cleanup(func() { _ = store.DeletePAT(context.Background(), reference) })

	credential, err := store.LoadPAT(context.Background(), reference, target)
	if err != nil {
		t.Fatalf("LoadPAT() error = %v", err)
	}
	if credential.Name != "synthetic-name" || credential.Secret != "synthetic-secret" || credential.Source != auth.CredentialSourceOSKeyring {
		t.Fatal("LoadPAT() returned an unexpected credential")
	}
	if err := store.ReplacePAT(context.Background(), reference, target, "replacement-name", "replacement-secret"); err != nil {
		t.Fatalf("ReplacePAT() error = %v", err)
	}
	credential, err = store.LoadPAT(context.Background(), reference, target)
	if err != nil || credential.Name != "replacement-name" || credential.Secret != "replacement-secret" {
		t.Fatal("LoadPAT() did not return the replacement credential")
	}
	if err := store.DeletePAT(context.Background(), reference); err != nil {
		t.Fatalf("DeletePAT() error = %v", err)
	}
	if _, err := store.LoadPAT(context.Background(), reference, target); err == nil {
		t.Fatal("LoadPAT() succeeded after deletion")
	}
}

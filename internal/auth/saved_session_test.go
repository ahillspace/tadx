package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

// mapKeyring keeps every entry so PAT and session records can coexist.
type mapKeyring struct {
	entries   map[string]string
	deleteErr map[string]error
	setErr    error
}

func newMapKeyring() *mapKeyring {
	return &mapKeyring{entries: map[string]string{}, deleteErr: map[string]error{}}
}

func (k *mapKeyring) Set(service, account, value string) error {
	if k.setErr != nil {
		return k.setErr
	}
	k.entries[service+"/"+account] = value
	return nil
}

func (k *mapKeyring) Get(service, account string) (string, error) {
	value, ok := k.entries[service+"/"+account]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}

func (k *mapKeyring) Delete(service, account string) error {
	if err := k.deleteErr[account]; err != nil {
		return err
	}
	if _, ok := k.entries[service+"/"+account]; !ok {
		return keyring.ErrNotFound
	}
	delete(k.entries, service+"/"+account)
	return nil
}

var savedSessionTarget = CredentialTarget{ServerURL: "https://example.com", SiteContentURL: "Sales"}

func testSavedSession() SavedSession {
	return SavedSession{Credential: credentialIdentity(savedSessionTarget.ServerURL, PATCredentials{Name: "agent-name", Secret: "highly-secret"}), Epoch: 7, Token: "session-token-value", SiteLUID: "site-luid", UserLUID: "user-luid"}
}

func storeWithPAT(t *testing.T) (*patStore, *mapKeyring, CredentialReference) {
	t.Helper()
	backend := newMapKeyring()
	store := newPATStore(backend, fixedReader(0xab), t.TempDir())
	reference, err := store.StorePAT(t.Context(), savedSessionTarget, "agent-name", "highly-secret")
	if err != nil {
		t.Fatal(err)
	}
	return store, backend, reference
}

func TestSavedSessionRoundTripsInItsOwnTargetBoundEntry(t *testing.T) {
	store, backend, reference := storeWithPAT(t)
	if err := store.SaveSession(t.Context(), reference, savedSessionTarget, testSavedSession()); err != nil {
		t.Fatal(err)
	}
	encoded, ok := backend.entries[credentialService+"/session:v1:abababababababababababababababab"]
	if !ok {
		t.Fatalf("session entry missing: %v", backend.entries)
	}
	if strings.Contains(encoded, "highly-secret") || strings.Contains(encoded, "agent-name") {
		t.Fatal("session record contains PAT values")
	}
	got, err := store.LoadSession(t.Context(), reference, savedSessionTarget)
	if err != nil {
		t.Fatal(err)
	}
	if got != testSavedSession() {
		t.Fatalf("LoadSession() = %#v", got)
	}
	if _, err := store.LoadPAT(t.Context(), reference, savedSessionTarget); err != nil {
		t.Fatalf("PAT record disturbed: %v", err)
	}
	_, err = store.LoadSession(t.Context(), reference, CredentialTarget{ServerURL: "https://example.com", SiteContentURL: "Finance"})
	assertStoreErrorKind(t, err, CredentialStoreTargetMismatch)
}

func TestSavedSessionMissingEntryIsNotFound(t *testing.T) {
	store, _, reference := storeWithPAT(t)
	_, err := store.LoadSession(t.Context(), reference, savedSessionTarget)
	assertStoreErrorKind(t, err, CredentialStoreNotFound)
}

func TestSavedSessionRejectsMalformedRecords(t *testing.T) {
	store, backend, reference := storeWithPAT(t)
	if err := store.SaveSession(t.Context(), reference, savedSessionTarget, testSavedSession()); err != nil {
		t.Fatal(err)
	}
	account := credentialService + "/session:v1:abababababababababababababababab"
	valid := backend.entries[account]
	var fields map[string]any
	if err := json.Unmarshal([]byte(valid), &fields); err != nil {
		t.Fatal(err)
	}
	reencode := func(change func(map[string]any)) string {
		copied := map[string]any{}
		for key, value := range fields {
			copied[key] = value
		}
		change(copied)
		data, _ := json.Marshal(copied)
		return string(data)
	}
	for name, record := range map[string]string{
		"unknown field":   reencode(func(m map[string]any) { m["extra"] = true }),
		"duplicate field": strings.Replace(valid, `{"version":1,`, `{"version":1,"version":1,`, 1),
		"wrong version":   reencode(func(m map[string]any) { m["version"] = 2 }),
		"empty token":     reencode(func(m map[string]any) { m["token"] = "" }),
		"empty site":      reencode(func(m map[string]any) { m["site_luid"] = "" }),
		"empty user":      reencode(func(m map[string]any) { m["user_luid"] = "" }),
		"bad credential":  reencode(func(m map[string]any) { m["credential"] = "not-a-digest" }),
		"oversized":       reencode(func(m map[string]any) { m["token"] = strings.Repeat("t", maxSessionRecordBytes) }),
		"trailing value":  valid + "{}",
	} {
		t.Run(name, func(t *testing.T) {
			if !strings.HasPrefix(valid, `{"version":1,`) {
				t.Fatalf("record layout changed: %s", valid)
			}
			backend.entries[account] = record
			_, err := store.LoadSession(t.Context(), reference, savedSessionTarget)
			assertStoreErrorKind(t, err, CredentialStoreCorrupt)
		})
	}
}

func TestReplacingOrDeletingPATRemovesItsSavedSession(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(context.Context, *patStore, CredentialReference) error
	}{
		{"replace", func(ctx context.Context, store *patStore, reference CredentialReference) error {
			return store.ReplacePAT(ctx, reference, savedSessionTarget, "other-name", "other-secret")
		}},
		{"delete", func(ctx context.Context, store *patStore, reference CredentialReference) error {
			return store.DeletePAT(ctx, reference)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, _, reference := storeWithPAT(t)
			if err := store.SaveSession(t.Context(), reference, savedSessionTarget, testSavedSession()); err != nil {
				t.Fatal(err)
			}
			if err := test.change(t.Context(), store, reference); err != nil {
				t.Fatal(err)
			}
			_, err := store.LoadSession(t.Context(), reference, savedSessionTarget)
			assertStoreErrorKind(t, err, CredentialStoreNotFound)
		})
	}
}

func TestPATChangesSucceedWithoutASavedSession(t *testing.T) {
	store, _, reference := storeWithPAT(t)
	if err := store.ReplacePAT(t.Context(), reference, savedSessionTarget, "other-name", "other-secret"); err != nil {
		t.Fatalf("ReplacePAT() without a saved session = %v", err)
	}
	if err := store.DeletePAT(t.Context(), reference); err != nil {
		t.Fatalf("DeletePAT() without a saved session = %v", err)
	}
}

func TestReplacePATSucceedsWhenSessionRemovalFails(t *testing.T) {
	store, backend, reference := storeWithPAT(t)
	if err := store.SaveSession(t.Context(), reference, savedSessionTarget, testSavedSession()); err != nil {
		t.Fatal(err)
	}
	backend.deleteErr["session:v1:abababababababababababababababab"] = errors.New("backend refused")
	if err := store.ReplacePAT(t.Context(), reference, savedSessionTarget, "other-name", "other-secret"); err != nil {
		t.Fatalf("ReplacePAT() = %v; a leftover session is never reused for a different PAT", err)
	}
	credentials, err := store.LoadPAT(t.Context(), reference, savedSessionTarget)
	if err != nil || credentials.Name != "other-name" {
		t.Fatalf("LoadPAT() = %v, %v", credentials, err)
	}
}

func TestDeletePATKeepsThePATWhenSessionRemovalFails(t *testing.T) {
	store, backend, reference := storeWithPAT(t)
	if err := store.SaveSession(t.Context(), reference, savedSessionTarget, testSavedSession()); err != nil {
		t.Fatal(err)
	}
	backend.deleteErr["session:v1:abababababababababababababababab"] = errors.New("backend refused")
	if err := store.DeletePAT(t.Context(), reference); err == nil {
		t.Fatal("DeletePAT() succeeded while its saved session remained")
	}
	if _, err := store.LoadPAT(t.Context(), reference, savedSessionTarget); err != nil {
		t.Fatalf("PAT removed although logout must be retried: %v", err)
	}
}

func TestSavedSessionFormattingRedactsToken(t *testing.T) {
	saved := testSavedSession()
	data, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{saved.String(), fmt.Sprintf("%v", saved), fmt.Sprintf("%+v", saved), fmt.Sprintf("%#v", saved), string(data)} {
		if strings.Contains(rendered, saved.Token) || strings.Contains(rendered, saved.Credential) {
			t.Fatalf("formatting exposed session material: %s", rendered)
		}
	}
}

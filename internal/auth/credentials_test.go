package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	keyring "github.com/zalando/go-keyring"
)

type recordingKeyring struct {
	service     string
	account     string
	value       string
	setErr      error
	getErr      error
	deleteErr   error
	setCalls    int
	getCalls    int
	deleteCalls int
}

func (r *recordingKeyring) Set(service, account, value string) error {
	r.setCalls++
	r.service, r.account, r.value = service, account, value
	return r.setErr
}

func (r *recordingKeyring) Get(service, account string) (string, error) {
	r.getCalls++
	r.service, r.account = service, account
	return r.value, r.getErr
}

func (r *recordingKeyring) Delete(service, account string) error {
	r.deleteCalls++
	r.service, r.account = service, account
	return r.deleteErr
}

type fixedReader byte

func (r fixedReader) Read(output []byte) (int, error) {
	for index := range output {
		output[index] = byte(r)
	}
	return len(output), nil
}

func TestPATStoreUsesOpaqueReferenceAndBoundedTargetBoundRecord(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(0xab))
	reference, err := store.StorePAT(context.Background(), CredentialTarget{
		ServerURL:      "HTTPS://Example.COM:443/",
		SiteContentURL: "Sales",
	}, "agent-name", "highly-secret")
	if err != nil {
		t.Fatalf("StorePAT() error = %v", err)
	}
	if got, want := reference, CredentialReference("cred_abababababababababababababababab"); got != want {
		t.Fatalf("reference = %q, want %q", got, want)
	}
	if backend.service != credentialService || backend.account != "pat:v1:abababababababababababababababab" {
		t.Fatalf("keyring address = %q/%q", backend.service, backend.account)
	}
	for _, forbidden := range []string{"Example.COM", "example.com", "Sales"} {
		if strings.Contains(backend.account, forbidden) {
			t.Fatalf("account contains target value %q", forbidden)
		}
	}
	if len(backend.value) > maxPATRecordBytes {
		t.Fatalf("record size = %d", len(backend.value))
	}

	credentials, err := store.LoadPAT(context.Background(), reference, CredentialTarget{
		ServerURL:      "https://example.com",
		SiteContentURL: "Sales",
	})
	if err != nil {
		t.Fatalf("LoadPAT() error = %v", err)
	}
	if credentials.Name != "agent-name" || credentials.Secret != "highly-secret" || credentials.Source != CredentialSourceOSKeyring {
		t.Fatalf("credentials = %#v", credentials)
	}
	formatted := fmt.Sprintf("%v %+v %#v", credentials, credentials, credentials)
	encoded, err := json.Marshal(credentials)
	if err != nil {
		t.Fatal(err)
	}
	formatted += string(encoded)
	for _, secret := range []string{"agent-name", "highly-secret"} {
		if strings.Contains(formatted, secret) {
			t.Fatalf("formatted credentials contain %q", secret)
		}
	}
}

func TestPATStoreBindsCredentialsToNormalizedOriginAndExactSite(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(1))
	reference, err := store.StorePAT(context.Background(), CredentialTarget{
		ServerURL: "https://example.com:443/", SiteContentURL: "Sales",
	}, "name", "secret")
	if err != nil {
		t.Fatal(err)
	}

	for _, target := range []CredentialTarget{
		{ServerURL: "https://example.com", SiteContentURL: "sales"},
		{ServerURL: "https://example.com:8443", SiteContentURL: "Sales"},
	} {
		_, err := store.LoadPAT(context.Background(), reference, target)
		var storeErr *CredentialStoreError
		if !errors.As(err, &storeErr) || storeErr.Kind != CredentialStoreTargetMismatch {
			t.Fatalf("LoadPAT(%#v) error = %v", target, err)
		}
	}
}

func TestPATStoreBindsCredentialsToNormalizedServerBasePath(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(3))
	reference, err := store.StorePAT(context.Background(), CredentialTarget{ServerURL: "https://example.com/tableau/"}, "name", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPAT(context.Background(), reference, CredentialTarget{ServerURL: "https://example.com/tableau"}); err != nil {
		t.Fatalf("equivalent base path failed: %v", err)
	}
	_, err = store.LoadPAT(context.Background(), reference, CredentialTarget{ServerURL: "https://example.com/other"})
	assertStoreErrorKind(t, err, CredentialStoreTargetMismatch)
}

func TestPATStoreSupportsTheDefaultSiteAndDeletesOnlyItsOpaqueReference(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(2))
	target := CredentialTarget{ServerURL: "https://example.com", SiteContentURL: ""}
	reference, err := store.StorePAT(context.Background(), target, "name", "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadPAT(context.Background(), reference, target); err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePAT(context.Background(), reference); err != nil {
		t.Fatal(err)
	}
	if backend.deleteCalls != 1 || backend.service != credentialService || backend.account != credentialAccount(reference) {
		t.Fatalf("delete call = %d, address = %q/%q", backend.deleteCalls, backend.service, backend.account)
	}
}

func TestPATStoreReplacesCredentialAtTheSameOpaqueReference(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(4))
	target := CredentialTarget{ServerURL: "https://example.com", SiteContentURL: "Sales"}
	reference, err := store.StorePAT(context.Background(), target, "old-name", "old-secret")
	if err != nil {
		t.Fatal(err)
	}
	account := backend.account
	if err := store.ReplacePAT(context.Background(), reference, target, "new-name", "new-secret"); err != nil {
		t.Fatal(err)
	}
	if backend.account != account || backend.setCalls != 2 {
		t.Fatalf("replacement address = %q, calls = %d", backend.account, backend.setCalls)
	}
	credentials, err := store.LoadPAT(context.Background(), reference, target)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Name != "new-name" || credentials.Secret != "new-secret" {
		t.Fatal("replacement credential was not loaded")
	}
}

func TestPATStoreRejectsInvalidTargetsBeforeKeyringAccess(t *testing.T) {
	t.Parallel()

	for _, target := range []CredentialTarget{
		{ServerURL: "http://example.com"},
		{ServerURL: "https://user:secret@example.com"},
		{ServerURL: "https://example.com?query=value"},
		{ServerURL: "https://example.com#fragment"},
		{ServerURL: "https://example.com", SiteContentURL: " Sales "},
	} {
		backend := &recordingKeyring{}
		store := newPATStore(backend, fixedReader(1))
		_, err := store.StorePAT(context.Background(), target, "name", "secret")
		var storeErr *CredentialStoreError
		if !errors.As(err, &storeErr) || storeErr.Kind != CredentialStoreInvalid {
			t.Fatalf("StorePAT(%#v) error = %v", target, err)
		}
		if backend.setCalls != 0 {
			t.Fatalf("StorePAT(%#v) keyring calls = %d", target, backend.setCalls)
		}
	}
}

func TestPATStoreRejectsOversizeAndMalformedRecords(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(1))
	_, err := store.StorePAT(context.Background(), CredentialTarget{ServerURL: "https://example.com"}, "name", strings.Repeat("s", maxPATRecordBytes))
	assertStoreErrorKind(t, err, CredentialStoreTooLarge)
	if backend.setCalls != 0 {
		t.Fatalf("oversize keyring calls = %d", backend.setCalls)
	}

	reference := CredentialReference("cred_01010101010101010101010101010101")
	fingerprint, err := credentialTargetFingerprint(CredentialTarget{ServerURL: "https://example.com"})
	if err != nil {
		t.Fatal(err)
	}
	valid := fmt.Sprintf(`{"version":1,"pat_name":"name","pat_secret":"secret","target_fingerprint":%q}`, fingerprint)
	for _, record := range []string{
		`{`,
		`{"version":2,"pat_name":"name","pat_secret":"secret","target_fingerprint":"sha256:x"}`,
		`{"version":1,"pat_name":"","pat_secret":"secret","target_fingerprint":"sha256:x"}`,
		`{"version":1,"pat_name":"name","pat_secret":"secret","target_fingerprint":"sha256:x"}`,
		valid + `{}`,
		strings.Replace(valid, `"pat_name":"name"`, `"pat_name":"name","pat_name":"other"`, 1),
		strings.Replace(valid, `"pat_name":"name"`, `"pat_name":"name","extra":true`, 1),
		strings.Repeat("x", maxPATRecordBytes+1),
	} {
		backend.value = record
		_, err := store.LoadPAT(context.Background(), reference, CredentialTarget{ServerURL: "https://example.com"})
		assertStoreErrorKind(t, err, CredentialStoreCorrupt)
	}
}

func TestCredentialStoreErrorsProvideBoundedRetryGuidance(t *testing.T) {
	t.Parallel()

	for _, kind := range []CredentialStoreErrorKind{
		CredentialStoreInvalid,
		CredentialStoreNotFound,
		CredentialStoreUnavailable,
		CredentialStoreLocked,
		CredentialStoreDenied,
		CredentialStoreUnsupported,
		CredentialStoreCorrupt,
		CredentialStoreTargetMismatch,
		CredentialStoreTooLarge,
		CredentialStoreCanceled,
		CredentialStoreOperationFailed,
	} {
		err := &CredentialStoreError{Kind: kind}
		if err.Error() == "" || err.CorrectiveAction() == "" {
			t.Fatalf("kind %q has incomplete diagnostics", kind)
		}
		wantRetryable := kind == CredentialStoreUnavailable || kind == CredentialStoreOperationFailed
		if err.Retryable() != wantRetryable {
			t.Fatalf("kind %q retryable = %t", kind, err.Retryable())
		}
	}
}

func TestPATStoreClassifiesBackendFailuresWithoutLeakingRawErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		kind CredentialStoreErrorKind
	}{
		{name: "not found", err: keyring.ErrNotFound, kind: CredentialStoreNotFound},
		{name: "too large", err: keyring.ErrSetDataTooBig, kind: CredentialStoreTooLarge},
		{name: "locked", err: errors.New("collection is locked highly-secret"), kind: CredentialStoreLocked},
		{name: "denied", err: errors.New("access denied highly-secret"), kind: CredentialStoreDenied},
		{name: "unavailable", err: errors.New("D-Bus session bus unavailable highly-secret"), kind: CredentialStoreUnavailable},
		{name: "unsupported", err: errors.New("keyring provider not implemented highly-secret"), kind: CredentialStoreUnsupported},
		{name: "unknown", err: errors.New("unexpected highly-secret"), kind: CredentialStoreOperationFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := &recordingKeyring{getErr: test.err}
			store := newPATStore(backend, fixedReader(1))
			_, err := store.LoadPAT(context.Background(), CredentialReference("cred_01010101010101010101010101010101"), CredentialTarget{ServerURL: "https://example.com"})
			assertStoreErrorKind(t, err, test.kind)
			if strings.Contains(fmt.Sprintf("%v %#v", err, err), "highly-secret") {
				t.Fatalf("classified error leaked backend details: %v", err)
			}
		})
	}
}

func TestPATStoreRejectsInvalidReferenceAndCanceledContextBeforeKeyringAccess(t *testing.T) {
	t.Parallel()

	backend := &recordingKeyring{}
	store := newPATStore(backend, fixedReader(1))
	_, err := store.LoadPAT(context.Background(), "dev", CredentialTarget{ServerURL: "https://example.com"})
	assertStoreErrorKind(t, err, CredentialStoreInvalid)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.StorePAT(ctx, CredentialTarget{ServerURL: "https://example.com"}, "name", "secret")
	assertStoreErrorKind(t, err, CredentialStoreCanceled)
	if backend.getCalls != 0 || backend.setCalls != 0 {
		t.Fatalf("keyring calls after rejected inputs = get %d, set %d", backend.getCalls, backend.setCalls)
	}
}

func TestPATSourceResolverPrefersOnlyCompleteEnvironmentPair(t *testing.T) {
	t.Parallel()

	store := &recordingPATStore{credentials: PATCredentials{Name: "stored", Secret: "stored-secret"}}
	lookup := LookupEnvFunc(func(key string) (string, bool) {
		value, ok := map[string]string{"PAT_NAME": "environment", "PAT_SECRET": "environment-secret"}[key]
		return value, ok
	})
	resolver := NewPATSourceResolver(lookup, store)
	credentials, err := resolver.Resolve(context.Background(), Target{
		PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET", CredentialReference: "cred_01010101010101010101010101010101",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Name != "environment" || credentials.Secret != "environment-secret" || credentials.Source != CredentialSourceEnvironment {
		t.Fatalf("credentials = %#v", credentials)
	}
	if store.loadCalls != 0 {
		t.Fatalf("store load calls = %d", store.loadCalls)
	}
}

func TestPATSourceResolverRejectsPartialEnvironmentPairWithoutStoreFallback(t *testing.T) {
	t.Parallel()

	for _, values := range []map[string]string{
		{"PAT_NAME": "name"},
		{"PAT_SECRET": "secret"},
		{"PAT_NAME": "", "PAT_SECRET": "secret"},
		{"PAT_NAME": "   ", "PAT_SECRET": "secret"},
	} {
		store := &recordingPATStore{}
		resolver := NewPATSourceResolver(LookupEnvFunc(func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		}), store)
		_, err := resolver.Resolve(context.Background(), Target{
			Environment: "dev", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET", CredentialReference: "cred_01010101010101010101010101010101",
		})
		var partial *PartialEnvironmentCredentialsError
		if !errors.As(err, &partial) {
			t.Fatalf("Resolve(%#v) error = %v", values, err)
		}
		if store.loadCalls != 0 {
			t.Fatalf("Resolve(%#v) store calls = %d", values, store.loadCalls)
		}
	}
}

func TestPATSourceResolverTreatsWhitespaceOnlyPairAsAbsent(t *testing.T) {
	t.Parallel()

	store := &recordingPATStore{credentials: PATCredentials{Name: "stored", Secret: "stored-secret", Source: CredentialSourceOSKeyring}}
	resolver := NewPATSourceResolver(LookupEnvFunc(func(string) (string, bool) { return "   ", true }), store)
	credentials, err := resolver.Resolve(context.Background(), Target{
		ServerURL: "https://example.com", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET",
		CredentialReference: "cred_01010101010101010101010101010101",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Source != CredentialSourceOSKeyring || store.loadCalls != 1 {
		t.Fatalf("credentials = %#v, store calls = %d", credentials, store.loadCalls)
	}
}

func TestPATSourceResolverFallsBackToBoundStoreOnlyWhenEnvironmentPairIsAbsent(t *testing.T) {
	t.Parallel()

	store := &recordingPATStore{credentials: PATCredentials{Name: "stored", Secret: "stored-secret", Source: CredentialSourceOSKeyring}}
	resolver := NewPATSourceResolver(LookupEnvFunc(func(string) (string, bool) { return "", false }), store)
	target := Target{
		ServerURL: "https://example.com", SiteContentURL: "Sales", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET",
		CredentialReference: "cred_01010101010101010101010101010101",
	}
	credentials, err := resolver.Resolve(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if credentials.Name != "stored" || store.loadCalls != 1 || store.target.ServerURL != target.ServerURL || store.reference != CredentialReference(target.CredentialReference) {
		t.Fatalf("credentials/store call = %#v/%#v", credentials, store)
	}
}

func TestPATSourceResolverPreservesMissingVariableDiagnosticsWithoutAStoredReference(t *testing.T) {
	t.Parallel()

	store := &recordingPATStore{}
	resolver := NewPATSourceResolver(LookupEnvFunc(func(string) (string, bool) { return "", false }), store)
	_, err := resolver.Resolve(context.Background(), Target{
		Environment: "dev", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET",
	})
	var missing *MissingVariablesError
	if !errors.As(err, &missing) {
		t.Fatalf("Resolve() error = %v", err)
	}
	if store.loadCalls != 0 {
		t.Fatalf("store load calls = %d", store.loadCalls)
	}
}

func TestPATProviderCanAuthenticateWithStoredCredentials(t *testing.T) {
	t.Parallel()

	store := &recordingPATStore{credentials: PATCredentials{Name: "stored", Secret: "stored-secret", Source: CredentialSourceOSKeyring}}
	signer := &storedCredentialSigner{response: SignInResponse{Token: "token", SiteLUID: "site", UserLUID: "user"}}
	provider := NewPATProviderWithStore(LookupEnvFunc(func(string) (string, bool) { return "", false }), signer, store)
	_, err := provider.Authenticate(context.Background(), Target{
		ServerURL: "https://example.com", PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET",
		CredentialReference: "cred_01010101010101010101010101010101",
	})
	if err != nil {
		t.Fatal(err)
	}
	if signer.request.PATName != "stored" || signer.request.PATSecret != "stored-secret" {
		t.Fatalf("sign-in request = %#v", signer.request)
	}
}

type recordingPATStore struct {
	credentials PATCredentials
	err         error
	reference   CredentialReference
	target      CredentialTarget
	loadCalls   int
}

type storedCredentialSigner struct {
	request  SignInRequest
	response SignInResponse
}

func (s *storedCredentialSigner) SignIn(_ context.Context, request SignInRequest) (SignInResponse, error) {
	s.request = request
	return s.response, nil
}

func (*recordingPATStore) StorePAT(context.Context, CredentialTarget, string, string) (CredentialReference, error) {
	return "", errors.New("unexpected StorePAT call")
}

func (*recordingPATStore) ReplacePAT(context.Context, CredentialReference, CredentialTarget, string, string) error {
	return errors.New("unexpected ReplacePAT call")
}

func (s *recordingPATStore) LoadPAT(_ context.Context, reference CredentialReference, target CredentialTarget) (PATCredentials, error) {
	s.loadCalls++
	s.reference, s.target = reference, target
	return s.credentials, s.err
}

func (*recordingPATStore) DeletePAT(context.Context, CredentialReference) error {
	return errors.New("unexpected DeletePAT call")
}

func assertStoreErrorKind(t *testing.T, err error, want CredentialStoreErrorKind) {
	t.Helper()
	var storeErr *CredentialStoreError
	if !errors.As(err, &storeErr) || storeErr.Kind != want {
		t.Fatalf("error = %v (%T), want kind %q", err, err, want)
	}
}

var _ io.Reader = fixedReader(0)

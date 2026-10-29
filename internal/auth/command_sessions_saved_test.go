package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

// countingSigner issues a new token for every sign-in.
type countingSigner struct {
	mu    sync.Mutex
	calls int
	user  string
}

func (s *countingSigner) SignIn(context.Context, SignInRequest) (SignInResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	user := s.user
	if user == "" {
		user = "user-luid"
	}
	return SignInResponse{Token: fmt.Sprintf("token-%d", s.calls), SiteLUID: "site-luid", UserLUID: user}, nil
}

func (s *countingSigner) signIns() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type storedPATFixture struct {
	store     *patStore
	backend   *mapKeyring
	directory string
	target    Target
}

func newStoredPATFixture(t *testing.T) storedPATFixture {
	t.Helper()
	fixture := storedPATFixture{store: newPATStore(newMapKeyring(), fixedReader(0xab)), directory: t.TempDir()}
	fixture.backend = fixture.store.backend.(*mapKeyring)
	reference, err := fixture.store.StorePAT(t.Context(), savedSessionTarget, "agent-name", "highly-secret")
	if err != nil {
		t.Fatal(err)
	}
	fixture.target = Target{Environment: "sales", ServerURL: savedSessionTarget.ServerURL, SiteContentURL: savedSessionTarget.SiteContentURL, CredentialReference: string(reference)}
	return fixture
}

// command runs one command's authentication and returns the token it would send.
func (f storedPATFixture) command(t *testing.T, signer Signer, target Target) string {
	t.Helper()
	sessions := NewCommandSessions(nil, f.store, f.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), target, signer)
	if err != nil {
		t.Fatal(err)
	}
	return sentToken(current)
}

func sentToken(current Session) string {
	request, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	current.Authorize(request)
	return request.Header.Get(TableauAuthHeader)
}

func rejectedWith(current Session) http.Header {
	header := http.Header{}
	header.Set(TableauAuthHeader, sentToken(current))
	return header
}

func TestCommandsWithTheSameStoredPATReuseTheSavedSession(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	first := fixture.command(t, signer, fixture.target)
	second := fixture.command(t, signer, fixture.target)
	if signer.signIns() != 1 || second != first {
		t.Fatalf("sign-ins = %d, tokens %q then %q; want one sign-in reused by the next command", signer.signIns(), first, second)
	}
}

func TestReplacedPATNeverReusesTheOldPATSession(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	first := fixture.command(t, signer, fixture.target)
	// Leave the old session behind, as a failed cleanup would.
	fixture.backend.deleteErr[sessionAccount(CredentialReference(fixture.target.CredentialReference))] = fmt.Errorf("backend refused")
	if err := fixture.store.ReplacePAT(t.Context(), CredentialReference(fixture.target.CredentialReference), savedSessionTarget, "agent-name", "replacement-secret"); err != nil {
		t.Fatal(err)
	}
	second := fixture.command(t, signer, fixture.target)
	if signer.signIns() != 2 || second == first {
		t.Fatalf("sign-ins = %d, tokens %q then %q; want the replaced PAT to sign in", signer.signIns(), first, second)
	}
}

func TestEnvironmentPATSavesNoSession(t *testing.T) {
	fixture := newStoredPATFixture(t)
	target := fixture.target
	target.PATNameVariable, target.PATSecretVariable = "PAT_NAME", "PAT_SECRET"
	sessions := NewCommandSessions(LookupEnvFunc(func(name string) (string, bool) { return "environment-" + name, true }), fixture.store, fixture.directory)
	defer sessions.Close()
	if _, err := sessions.Authenticate(t.Context(), target, &countingSigner{}); err != nil {
		t.Fatal(err)
	}
	for account := range fixture.backend.entries {
		if strings.Contains(account, "session:") {
			t.Fatalf("environment PAT saved a session: %s", account)
		}
	}
}

func TestSavedSessionSupersededByAnotherSiteSignInIsNotReused(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	first := fixture.command(t, signer, fixture.target)
	// The same PAT stored for another site shares the sign-in epoch.
	fixture.store.random = fixedReader(0xcd)
	otherSite := CredentialTarget{ServerURL: savedSessionTarget.ServerURL, SiteContentURL: "Finance"}
	reference, err := fixture.store.StorePAT(t.Context(), otherSite, "agent-name", "highly-secret")
	if err != nil {
		t.Fatal(err)
	}
	fixture.command(t, signer, Target{ServerURL: otherSite.ServerURL, SiteContentURL: otherSite.SiteContentURL, CredentialReference: string(reference)})
	third := fixture.command(t, signer, fixture.target)
	if signer.signIns() != 3 || third == first {
		t.Fatalf("sign-ins = %d, tokens %q then %q; want the superseded session to sign in", signer.signIns(), first, third)
	}
}

func TestSessionStoreFailuresStillAuthenticate(t *testing.T) {
	t.Run("save", func(t *testing.T) {
		fixture := newStoredPATFixture(t)
		fixture.backend.setErr = fmt.Errorf("backend refused")
		signer := &countingSigner{}
		fixture.command(t, signer, fixture.target)
		fixture.command(t, signer, fixture.target)
		if signer.signIns() != 2 {
			t.Fatalf("sign-ins = %d, want each command to sign in without a saved session", signer.signIns())
		}
	})
	t.Run("load", func(t *testing.T) {
		fixture := newStoredPATFixture(t)
		signer := &countingSigner{}
		fixture.command(t, signer, fixture.target)
		fixture.backend.entries[credentialService+"/"+sessionAccount(CredentialReference(fixture.target.CredentialReference))] = "{corrupt"
		fixture.command(t, signer, fixture.target)
		if signer.signIns() != 2 {
			t.Fatalf("sign-ins = %d, want a corrupt saved session to sign in", signer.signIns())
		}
	})
}

func TestSignInAlwaysSignsInAndSavesTheNewSession(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	fixture.command(t, signer, fixture.target)
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	current, source, err := sessions.SignIn(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	checked := sentToken(current)
	if err := sessions.Close(); err != nil {
		t.Fatal(err)
	}
	if source != CredentialSourceOSKeyring || signer.signIns() != 2 {
		t.Fatalf("source = %q, sign-ins = %d; want a fresh sign-in", source, signer.signIns())
	}
	if next := fixture.command(t, signer, fixture.target); next != checked || signer.signIns() != 2 {
		t.Fatalf("next command sent %q after %d sign-ins, want the checked session %q", next, signer.signIns(), checked)
	}
}

func TestRenewSignsInOnceAndUpdatesTheSharedSession(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	rejected := rejectedWith(current)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() {
			if err := current.Renew(t.Context(), rejected); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	renewed := sentToken(current)
	if signer.signIns() != 2 || renewed == rejected.Get(TableauAuthHeader) {
		t.Fatalf("sign-ins = %d, token %q; want one renewal for concurrent rejections", signer.signIns(), renewed)
	}
	again, err := sessions.Authenticate(t.Context(), fixture.target, signer)
	if err != nil || again != current {
		t.Fatalf("Authenticate() after renewal = %v, %v; want the same renewed session", again, err)
	}
	if err := sessions.Close(); err != nil {
		t.Fatal(err)
	}
	if next := fixture.command(t, signer, fixture.target); next != renewed {
		t.Fatalf("next command sent %q, want the renewed session %q", next, renewed)
	}
}

func TestRenewTakesTheCredentialLockAgainAfterSuspend(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), rejectedWith(current)); err != nil {
		t.Fatal(err)
	}
	other := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer other.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	if _, err := other.Authenticate(ctx, fixture.target, signer); err == nil || !strings.Contains(err.Error(), "in use by another command") {
		t.Fatalf("other command Authenticate() = %v, want the renewing command to hold the PAT", err)
	}
}

func TestRenewWithAnAlreadyReplacedTokenDoesNotSignIn(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	stale := rejectedWith(current)
	if err := current.Renew(t.Context(), stale); err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), stale); err != nil {
		t.Fatal(err)
	}
	if signer.signIns() != 2 {
		t.Fatalf("sign-ins = %d, want the second stale rejection to reuse the renewed token", signer.signIns())
	}
}

func TestRenewRejectsADifferentIdentity(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	before := sentToken(current)
	signer.user = "other-user"
	if err := current.Renew(t.Context(), rejectedWith(current)); err == nil {
		t.Fatal("Renew() accepted a sign-in for a different user")
	}
	if sentToken(current) != before || current.UserLUID() != "user-luid" {
		t.Fatal("rejected renewal changed the session")
	}
}

func TestRenewAfterCloseFails(t *testing.T) {
	fixture := newStoredPATFixture(t)
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	current, err := sessions.Authenticate(t.Context(), fixture.target, &countingSigner{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sessions.Close(); err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), rejectedWith(current)); err == nil {
		t.Fatal("Renew() signed in after the command closed")
	}
}

func TestProviderSessionRenewsWithItsPAT(t *testing.T) {
	signer := &countingSigner{}
	provider := NewPATProvider(LookupEnvFunc(func(name string) (string, bool) { return "value-" + name, true }), signer)
	current, err := provider.Authenticate(t.Context(), Target{ServerURL: "https://example.com", SiteContentURL: "Sales", PATNameVariable: "NAME", PATSecretVariable: "SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), rejectedWith(current)); err != nil {
		t.Fatal(err)
	}
	if sentToken(current) != "token-2" {
		t.Fatalf("renewed token = %q", sentToken(current))
	}
}

package auth

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestRemovedCredentialCannotBeRestoredByRenewal(t *testing.T) {
	fixture := newStoredPATFixture(t)
	sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer sessions.Close()
	current, err := sessions.Authenticate(t.Context(), fixture.target, &countingSigner{})
	if err != nil {
		t.Fatal(err)
	}
	ref := CredentialReference(fixture.target.CredentialReference)
	if err := fixture.store.DeletePAT(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), rejectedWith(current)); err != nil {
		t.Fatal(err)
	}
	_, err = fixture.store.LoadSession(t.Context(), ref, savedSessionTarget)
	assertStoreErrorKind(t, err, CredentialStoreNotFound)
}

func TestCanonicalServerIdentityAllowsSessionReuseAndRejectsStalePAT(t *testing.T) {
	fixture := newStoredPATFixture(t)
	fixture.target.ServerURL = "https://EXAMPLE.COM:443/"
	signer := &countingSigner{}
	first := fixture.command(t, signer, fixture.target)
	second := fixture.command(t, signer, fixture.target)
	if first != second || signer.signIns() != 1 {
		t.Fatalf("tokens %q and %q after %d sign-ins; want one reused session", first, second, signer.signIns())
	}
	ref := CredentialReference(fixture.target.CredentialReference)
	if err := fixture.store.ReplacePAT(t.Context(), ref, savedSessionTarget, "new-name", "new-secret"); err != nil {
		t.Fatal(err)
	}
	stale := testSavedSession()
	err := fixture.store.SaveSession(t.Context(), ref, CredentialTarget{ServerURL: fixture.target.ServerURL, SiteContentURL: fixture.target.SiteContentURL}, stale)
	assertStoreErrorKind(t, err, CredentialStoreInvalid)
	_, err = fixture.store.LoadSession(t.Context(), ref, savedSessionTarget)
	assertStoreErrorKind(t, err, CredentialStoreNotFound)
}

func TestReferenceLockCoordinatesAcrossProcesses(t *testing.T) {
	if directory := os.Getenv("TADX_TEST_REFERENCE_LOCK_DIR"); directory != "" {
		store := newPATStore(newMapKeyring(), fixedReader(0xab), directory)
		held, err := store.lockReference(t.Context(), "cred_abababababababababababababababab", "test")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("locked")
		_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
		if err := held.Release(); err != nil {
			t.Fatal(err)
		}
		return
	}
	store, _, ref := storeWithPAT(t)
	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestReferenceLockCoordinatesAcrossProcesses$")
	cmd.Env = append(os.Environ(), "TADX_TEST_REFERENCE_LOCK_DIR="+store.lockDirectory)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "locked\n" && line != "locked\r\n" {
		t.Fatalf("child lock: %q, %v", line, err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	assertStoreErrorKind(t, store.SaveSession(ctx, ref, savedSessionTarget, testSavedSession()), CredentialStoreCanceled)
	if _, err := fmt.Fprintln(stdin, "release"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(t.Context(), ref, savedSessionTarget, testSavedSession()); err != nil {
		t.Fatal(err)
	}
}

type delayedLifecycleSigner struct{ entered, proceed chan struct{} }

func (s *delayedLifecycleSigner) SignIn(ctx context.Context, _ SignInRequest) (SignInResponse, error) {
	close(s.entered)
	select {
	case <-s.proceed:
		return SignInResponse{Token: "late-token", SiteLUID: "site-luid", UserLUID: "user-luid"}, nil
	case <-ctx.Done():
		return SignInResponse{}, ctx.Err()
	}
}

func TestCredentialChangeDuringSignInCannotPersistOldSession(t *testing.T) {
	for _, action := range []string{"delete", "replace"} {
		t.Run(action, func(t *testing.T) {
			fixture := newStoredPATFixture(t)
			sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
			defer sessions.Close()
			signer := &delayedLifecycleSigner{entered: make(chan struct{}), proceed: make(chan struct{})}
			done := make(chan error, 1)
			go func() { _, err := sessions.Authenticate(t.Context(), fixture.target, signer); done <- err }()
			<-signer.entered
			ref := CredentialReference(fixture.target.CredentialReference)
			var err error
			if action == "delete" {
				err = fixture.store.DeletePAT(t.Context(), ref)
			} else {
				err = fixture.store.ReplacePAT(t.Context(), ref, savedSessionTarget, "new-name", "new-secret")
			}
			if err != nil {
				t.Fatal(err)
			}
			close(signer.proceed)
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			_, err = fixture.store.LoadSession(t.Context(), ref, savedSessionTarget)
			assertStoreErrorKind(t, err, CredentialStoreNotFound)
		})
	}
}

type delayedSessionWrite struct {
	keyringBackend
	entered, proceed chan struct{}
}

func (b *delayedSessionWrite) Set(service, account, value string) error {
	if len(account) >= len("session:") && account[:len("session:")] == "session:" {
		close(b.entered)
		<-b.proceed
	}
	return b.keyringBackend.Set(service, account, value)
}

func TestDeleteWaitsForInFlightSessionWrite(t *testing.T) {
	store, backend, ref := storeWithPAT(t)
	delayed := &delayedSessionWrite{keyringBackend: backend, entered: make(chan struct{}), proceed: make(chan struct{})}
	store.backend = delayed
	saved := make(chan error, 1)
	go func() { saved <- store.SaveSession(t.Context(), ref, savedSessionTarget, testSavedSession()) }()
	<-delayed.entered
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()
	assertStoreErrorKind(t, store.DeletePAT(ctx, ref), CredentialStoreCanceled)
	close(delayed.proceed)
	if err := <-saved; err != nil {
		t.Fatal(err)
	}
	if err := store.DeletePAT(t.Context(), ref); err != nil {
		t.Fatal(err)
	}
	_, err := store.LoadSession(t.Context(), ref, savedSessionTarget)
	assertStoreErrorKind(t, err, CredentialStoreNotFound)
}

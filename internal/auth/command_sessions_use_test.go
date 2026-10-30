package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// overlapWindow is how long a command must keep waiting to show it waits for
// another command rather than signing in.
const overlapWindow = 250 * time.Millisecond

// startCommand authenticates one command in the background, so a test can
// observe whether it waits for another command.
func startCommand(ctx context.Context, sessions *CommandSessions, target Target, signer Signer) <-chan error {
	done := make(chan error, 1)
	go func() {
		_, err := sessions.Authenticate(ctx, target, signer)
		done <- err
	}()
	return done
}

// requireWaiting fails unless the command is still waiting after the overlap
// window, without another sign-in.
func requireWaiting(t *testing.T, done <-chan error, signer *countingSigner, signIns int) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("second command returned %v after %d sign-ins; want it to wait for the first command", err, signer.signIns())
	case <-time.After(overlapWindow):
	}
	if signer.signIns() != signIns {
		t.Fatalf("sign-ins = %d while the first command was running, want %d", signer.signIns(), signIns)
	}
}

func requireFinished(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second command did not proceed after the first command finished")
	}
}

// TestLoginHoldsNoPATAfterItsSignIn keeps a login command, which signs in
// only to validate the PAT, from blocking later commands with the same PAT.
func TestLoginHoldsNoPATAfterItsSignIn(t *testing.T) {
	directory := t.TempDir()
	target := Target{ServerURL: savedSessionTarget.ServerURL, SiteContentURL: savedSessionTarget.SiteContentURL}
	credentials := PATCredentials{Name: "agent-name", Secret: "highly-secret"}
	signer := &countingSigner{}
	login := NewCommandSessions(nil, nil, directory)
	defer login.Close()
	if _, err := login.AuthenticateCredentials(t.Context(), target, credentials, signer); err != nil {
		t.Fatal(err)
	}
	next := NewCommandSessions(nil, nil, directory)
	defer next.Close()
	ctx, cancel := context.WithTimeout(t.Context(), overlapWindow)
	defer cancel()
	if _, err := next.AuthenticateCredentials(ctx, target, credentials, signer); err != nil || signer.signIns() != 2 {
		t.Fatalf("next command AuthenticateCredentials() = %v after %d sign-ins; want the login to hold nothing", err, signer.signIns())
	}
}

// storeSamePAT stores the fixture's PAT again under a new reference.
func (f storedPATFixture) storeSamePAT(t *testing.T, site string) Target {
	t.Helper()
	f.store.random = fixedReader(0xcd)
	target := CredentialTarget{ServerURL: savedSessionTarget.ServerURL, SiteContentURL: site}
	reference, err := f.store.StorePAT(t.Context(), target, "agent-name", "highly-secret")
	if err != nil {
		t.Fatal(err)
	}
	return Target{ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, CredentialReference: string(reference)}
}

// TestIncompatibleStoredPATCommandsTakeTurns runs two commands with one stored
// PAT whose sessions they cannot share. Signing in for the second would end
// the first command's session, so the second waits for the first to finish.
func TestIncompatibleStoredPATCommandsTakeTurns(t *testing.T) {
	for _, test := range []struct {
		name  string
		other func(storedPATFixture, *testing.T) Target
	}{
		{"another site", func(f storedPATFixture, t *testing.T) Target { return f.storeSamePAT(t, "Finance") }},
		{"another reference", func(f storedPATFixture, t *testing.T) Target {
			return f.storeSamePAT(t, savedSessionTarget.SiteContentURL)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newStoredPATFixture(t)
			other := test.other(fixture, t)
			signer := &countingSigner{}
			first := NewCommandSessions(nil, fixture.store, fixture.directory)
			defer first.Close()
			if _, err := first.Authenticate(t.Context(), fixture.target, signer); err != nil {
				t.Fatal(err)
			}
			second := NewCommandSessions(nil, fixture.store, fixture.directory)
			defer second.Close()
			done := startCommand(t.Context(), second, other, signer)
			requireWaiting(t, done, signer, 1)
			if err := first.Suspend(t.Context()); err != nil {
				t.Fatal(err)
			}
			requireFinished(t, done)
			if signer.signIns() != 2 {
				t.Fatalf("sign-ins = %d, want the second command to sign in after the first", signer.signIns())
			}
		})
	}
}

// TestUnsavedStoredPATSessionIsNotEndedByAnotherCommand fails to save a new
// session, so another command cannot adopt it and waits instead of signing in.
func TestUnsavedStoredPATSessionIsNotEndedByAnotherCommand(t *testing.T) {
	fixture := newStoredPATFixture(t)
	fixture.backend.setErr = fmt.Errorf("backend refused")
	signer := &countingSigner{}
	first := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer first.Close()
	if _, err := first.Authenticate(t.Context(), fixture.target, signer); err != nil {
		t.Fatal(err)
	}
	second := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer second.Close()
	done := startCommand(t.Context(), second, fixture.target, signer)
	requireWaiting(t, done, signer, 1)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	requireFinished(t, done)
}

// TestUnsavedRenewalMakesCompatibleCommandsWait has a renewal fail to save
// while another command shares the PAT's session. That command cannot adopt
// the renewal, so its own renewal waits instead of ending the first one.
func TestUnsavedRenewalMakesCompatibleCommandsWait(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	first := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer first.Close()
	second := NewCommandSessions(nil, fixture.store, fixture.directory)
	defer second.Close()
	firstSession, err := first.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	secondSession, err := second.Authenticate(t.Context(), fixture.target, signer)
	if err != nil {
		t.Fatal(err)
	}
	rejected := rejectedWith(firstSession)
	fixture.backend.mu.Lock()
	fixture.backend.setErr = fmt.Errorf("backend refused")
	fixture.backend.mu.Unlock()
	if err := firstSession.Renew(t.Context(), rejected); err != nil || signer.signIns() != 2 {
		t.Fatalf("Renew() = %v after %d sign-ins", err, signer.signIns())
	}
	done := make(chan error, 1)
	go func() { done <- secondSession.Renew(t.Context(), rejected) }()
	requireWaiting(t, done, signer, 2)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	requireFinished(t, done)
}

// TestParallelStoredPATCommandsSignInOnce starts several commands with one
// stored PAT at once; they all share the first command's sign-in.
func TestParallelStoredPATCommandsSignInOnce(t *testing.T) {
	fixture := newStoredPATFixture(t)
	signer := &countingSigner{}
	const commands = 6
	var running, finished sync.WaitGroup
	running.Add(commands)
	release := make(chan struct{})
	for range commands {
		finished.Go(func() {
			sessions := NewCommandSessions(nil, fixture.store, fixture.directory)
			defer sessions.Close()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			_, err := sessions.Authenticate(ctx, fixture.target, signer)
			running.Done()
			if err != nil {
				t.Error(err)
				return
			}
			// Every command stays open until all have authenticated.
			<-release
		})
	}
	running.Wait()
	close(release)
	finished.Wait()
	if signer.signIns() != 1 {
		t.Fatalf("sign-ins = %d, want parallel commands to share one sign-in", signer.signIns())
	}
}

// TestWaitingForOnePATDoesNotDelayAnother has one command wait for a PAT that
// another command holds while it authenticates with an unrelated PAT.
func TestWaitingForOnePATDoesNotDelayAnother(t *testing.T) {
	directory := t.TempDir()
	lookup := LookupEnvFunc(func(name string) (string, bool) { return "environment-" + name, true })
	held := Target{ServerURL: savedSessionTarget.ServerURL, SiteContentURL: savedSessionTarget.SiteContentURL, PATNameVariable: "PAT_NAME", PATSecretVariable: "PAT_SECRET"}
	unrelated := held
	unrelated.PATSecretVariable = "OTHER_PAT_SECRET"
	signer := &countingSigner{}
	holder := NewCommandSessions(lookup, nil, directory)
	defer holder.Close()
	if _, err := holder.Authenticate(t.Context(), held, signer); err != nil {
		t.Fatal(err)
	}
	waiter := NewCommandSessions(lookup, nil, directory)
	defer waiter.Close()
	waitCtx, stopWaiting := context.WithCancel(t.Context())
	waiting := startCommand(waitCtx, waiter, held, signer)
	defer func() {
		stopWaiting()
		<-waiting
	}()
	select {
	case err := <-waiting:
		t.Fatalf("waiting command returned %v; want it to wait for the holder", err)
	case <-time.After(overlapWindow):
	}
	ctx, cancel := context.WithTimeout(t.Context(), overlapWindow)
	defer cancel()
	if _, err := waiter.Authenticate(ctx, unrelated, signer); err != nil {
		t.Fatalf("Authenticate() for an unrelated PAT = %v; want it to proceed while the command waits for another PAT", err)
	}
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatal("unrelated PAT authenticated only at the deadline")
	}
}

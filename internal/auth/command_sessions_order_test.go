package auth

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

type orderedSigner struct {
	mu    sync.Mutex
	sites []string
}

func (s *orderedSigner) SignIn(_ context.Context, request SignInRequest) (SignInResponse, error) {
	s.mu.Lock()
	s.sites = append(s.sites, request.SiteContentURL)
	s.mu.Unlock()
	return SignInResponse{Token: "test-session-" + request.SiteContentURL, SiteLUID: request.SiteContentURL, UserLUID: "user"}, nil
}

func (s *orderedSigner) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sites...)
}

func TestCommandSessionForegroundAuthenticationPrecedesWaitingMonitor(t *testing.T) {
	for _, sameManager := range []bool{false, true} {
		name := "independent_managers"
		if sameManager {
			name = "same_manager"
		}
		t.Run(name, func(t *testing.T) {
			directory := t.TempDir()
			credentials := PATCredentials{Name: "test-name", Secret: "test-secret"}
			target := Target{ServerURL: "https://tableau.example.com", SiteContentURL: "holder-site"}
			holder := NewCommandSessions(nil, nil, directory)
			defer holder.Close()
			if _, err := holder.AuthenticateCredentials(t.Context(), target, credentials, &orderedSigner{}); err != nil {
				t.Fatal(err)
			}
			var lookups atomic.Int32
			manager := NewCommandSessions(LookupEnvFunc(func(name string) (string, bool) {
				lookups.Add(1)
				if name == "PAT_NAME" {
					return credentials.Name, true
				}
				return credentials.Secret, true
			}), nil, directory)
			defer manager.Close()
			foreground := manager
			if !sameManager {
				foreground = NewCommandSessions(nil, nil, directory)
				defer foreground.Close()
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			signer := &orderedSigner{}
			monitorDone := make(chan error, 1)
			go func() {
				monitorTarget := target
				monitorTarget.SiteContentURL = "monitor-site"
				monitorTarget.PATNameVariable, monitorTarget.PATSecretVariable = "PAT_NAME", "PAT_SECRET"
				_, err := manager.AuthenticateMonitor(ctx, monitorTarget, signer)
				if err == nil {
					err = manager.Suspend(ctx)
				}
				monitorDone <- err
			}()
			for lookups.Load() < 2 && ctx.Err() == nil {
				runtime.Gosched()
			}
			if lookups.Load() < 2 {
				t.Fatal("monitor did not resolve its PAT before foreground request")
			}

			// Own the monitor's existing command gate after its first attempt.
			// Its transient exclusive admission is now released, and another
			// monitor attempt cannot impersonate foreground registration.
			select {
			case manager.gate <- struct{}{}:
			case <-ctx.Done():
				t.Fatal("monitor did not yield its command gate")
			}
			paused := true
			defer func() {
				if paused {
					manager.leave()
				}
			}()
			foregroundDone := make(chan error, 1)
			go func() {
				foregroundTarget := target
				foregroundTarget.SiteContentURL = "foreground-site"
				_, err := foreground.AuthenticateCredentials(ctx, foregroundTarget, credentials, signer)
				if err == nil {
					err = foreground.Suspend(ctx)
				}
				foregroundDone <- err
			}()

			admissionPath := filepath.Join(directory, credentialIdentity(target.ServerURL, credentials)+".admission.lock")
			registered := false
			for !registered && ctx.Err() == nil {
				if sameManager {
					manager.priorityMu.Lock()
					registered = manager.foregroundWaiters > 0
					manager.priorityMu.Unlock()
				} else {
					probe, err := lock.TryAcquire(admissionPath)
					if errors.Is(err, lock.ErrLocked) {
						probe, err = lock.TryAcquireShared(admissionPath)
						registered = err == nil
					}
					if err != nil {
						t.Fatalf("foreground shared admission: %v", err)
					}
					if err := probe.Release(); err != nil {
						t.Fatal(err)
					}
				}
				if !registered {
					runtime.Gosched()
				}
			}
			if !registered {
				t.Fatal("foreground request did not register priority")
			}
			manager.leave()
			paused = false
			if err := holder.Close(); err != nil {
				t.Fatal(err)
			}
			for _, done := range []<-chan error{foregroundDone, monitorDone} {
				select {
				case err := <-done:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal("foreground and monitor authentication did not complete")
				}
			}
			if got := signer.snapshot(); len(got) != 2 || got[0] != "foreground-site" || got[1] != "monitor-site" {
				t.Fatalf("sign-in order=%v, want foreground before monitor", got)
			}
		})
	}
}

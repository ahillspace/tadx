package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

type renewalFailureSigner struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (s *renewalFailureSigner) SignIn(ctx context.Context, _ SignInRequest) (SignInResponse, error) {
	s.mu.Lock()
	s.calls++
	call := s.calls
	s.mu.Unlock()
	if call == 2 {
		close(s.started)
		select {
		case <-s.release:
			return SignInResponse{}, errors.New("renewal refused")
		case <-ctx.Done():
			return SignInResponse{}, ctx.Err()
		}
	}
	return SignInResponse{Token: fmt.Sprintf("token-%d", call), SiteLUID: "site", UserLUID: "user"}, nil
}
func (s *renewalFailureSigner) count() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

func TestConcurrentFailedCommandRenewalsShareFailureAndLaterRetry(t *testing.T) {
	signer := &renewalFailureSigner{started: make(chan struct{}), release: make(chan struct{})}
	manager := NewCommandSessions(LookupEnvFunc(func(string) (string, bool) { return "value", true }), nil, t.TempDir())
	defer manager.Close()
	current, err := manager.Authenticate(t.Context(), Target{ServerURL: "https://example.com", PATNameVariable: "NAME", PATSecretVariable: "SECRET"}, signer)
	if err != nil {
		t.Fatal(err)
	}
	rejected := rejectedWith(current)
	results := make(chan error, 2)
	go func() { results <- current.Renew(t.Context(), rejected) }()
	<-signer.started
	go func() { results <- current.Renew(t.Context(), rejected) }()
	deadline := time.After(time.Second)
	for {
		shared := current.(*session)
		shared.renewMu.Lock()
		waiting := shared.flight != nil && shared.flight.waiters == 2
		shared.renewMu.Unlock()
		if waiting {
			break
		}
		select {
		case <-deadline:
			t.Fatal("second renewal did not join the attempt")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	close(signer.release)
	for range 2 {
		if err := <-results; err == nil || err.Error() != "sign in: renewal refused" {
			t.Errorf("renewal error = %v", err)
		}
	}
	if got := signer.count(); got != 2 {
		t.Fatalf("sign-ins = %d, want initial sign-in and one failed renewal", got)
	}
	if err := current.Renew(t.Context(), rejected); err != nil {
		t.Fatal(err)
	}
	if got := signer.count(); got != 3 {
		t.Fatalf("sign-ins after retry = %d, want 3", got)
	}
}

func waitRenewalWaiters(t *testing.T, current Session, want int) {
	t.Helper()
	deadline := time.After(2 * time.Second)
	shared := current.(*session)
	for {
		shared.renewMu.Lock()
		ready := shared.flight != nil && shared.flight.waiters == want
		shared.renewMu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("renewal did not admit %d waiters", want)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

func TestProviderRenewalSharesFailureAndAllowsRetry(t *testing.T) {
	signer := &renewalFailureSigner{started: make(chan struct{}), release: make(chan struct{})}
	provider := NewPATProvider(LookupEnvFunc(func(string) (string, bool) { return "value", true }), signer)
	current, err := provider.Authenticate(t.Context(), Target{ServerURL: "https://example.com", PATNameVariable: "NAME", PATSecretVariable: "SECRET"})
	if err != nil {
		t.Fatal(err)
	}
	rejected := rejectedWith(current)
	results := make(chan error, 2)
	go func() { results <- current.Renew(t.Context(), rejected) }()
	<-signer.started
	go func() { results <- current.Renew(t.Context(), rejected) }()
	waitRenewalWaiters(t, current, 2)
	close(signer.release)
	for range 2 {
		if err := <-results; err == nil || err.Error() != "sign in: renewal refused" {
			t.Errorf("renewal error = %v", err)
		}
	}
	if signer.count() != 2 {
		t.Fatalf("sign-ins = %d, want 2", signer.count())
	}
	if err := current.Renew(t.Context(), rejected); err != nil {
		t.Fatal(err)
	}
	if signer.count() != 3 {
		t.Fatalf("sign-ins = %d, want 3", signer.count())
	}
}

func TestRenewalWaitersCancelIndependently(t *testing.T) {
	for _, command := range []bool{false, true} {
		t.Run(fmt.Sprintf("command=%v", command), func(t *testing.T) {
			signer := &renewalFailureSigner{started: make(chan struct{}), release: make(chan struct{})}
			target := Target{ServerURL: "https://example.com", PATNameVariable: "NAME", PATSecretVariable: "SECRET"}
			lookup := LookupEnvFunc(func(string) (string, bool) { return "value", true })
			var current Session
			var err error
			if command {
				manager := NewCommandSessions(lookup, nil, t.TempDir())
				defer manager.Close()
				current, err = manager.Authenticate(t.Context(), target, signer)
			} else {
				current, err = NewPATProvider(lookup, signer).Authenticate(t.Context(), target)
			}
			if err != nil {
				t.Fatal(err)
			}
			rejected := rejectedWith(current)
			first, cancelFirst := context.WithCancel(t.Context())
			second, cancelSecond := context.WithCancel(t.Context())
			defer cancelFirst()
			defer cancelSecond()
			results := make(chan error, 3)
			go func() { results <- current.Renew(first, rejected) }()
			<-signer.started
			go func() { results <- current.Renew(second, rejected) }()
			go func() { results <- current.Renew(t.Context(), rejected) }()
			waitRenewalWaiters(t, current, 3)
			cancelFirst()
			cancelSecond()
			for range 2 {
				if err := <-results; !errors.Is(err, context.Canceled) {
					t.Fatalf("canceled waiter = %v", err)
				}
			}
			close(signer.release)
			if err := <-results; err == nil || err.Error() != "sign in: renewal refused" {
				t.Fatalf("live waiter = %v", err)
			}
			if signer.count() != 2 {
				t.Fatalf("sign-ins = %d, want 2", signer.count())
			}
			if err := current.Renew(t.Context(), rejected); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAbandonedCommandRenewalStopsAndCanRetry(t *testing.T) {
	signer := &renewalFailureSigner{started: make(chan struct{}), release: make(chan struct{})}
	manager := NewCommandSessions(LookupEnvFunc(func(string) (string, bool) { return "value", true }), nil, t.TempDir())
	defer manager.Close()
	current, err := manager.Authenticate(t.Context(), Target{ServerURL: "https://example.com", PATNameVariable: "NAME", PATSecretVariable: "SECRET"}, signer)
	if err != nil {
		t.Fatal(err)
	}
	rejected := rejectedWith(current)
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() { result <- current.Renew(ctx, rejected) }()
	<-signer.started
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled renewal = %v", err)
	}
	if err := current.Renew(t.Context(), rejected); err != nil {
		t.Fatalf("retry after cancellation: %v", err)
	}
	if signer.count() != 3 {
		t.Fatalf("sign-ins = %d, want 3", signer.count())
	}
	if err := manager.Close(); err != nil {
		t.Fatal(err)
	}
	if err := current.Renew(t.Context(), rejectedWith(current)); err == nil {
		t.Fatal("renewal after close succeeded")
	}
}

package tableau

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
)

type heldFailureSigner struct {
	calls            atomic.Int32
	started, release chan struct{}
}

func (s *heldFailureSigner) SignIn(ctx context.Context, _ auth.SignInRequest) (auth.SignInResponse, error) {
	n := s.calls.Add(1)
	if n == 2 {
		close(s.started)
		select {
		case <-s.release:
			return auth.SignInResponse{}, errors.New("renewal refused")
		case <-ctx.Done():
			return auth.SignInResponse{}, ctx.Err()
		}
	}
	token := "new"
	if n == 1 {
		token = "old"
	}
	return auth.SignInResponse{Token: token, SiteLUID: "site", UserLUID: "user"}, nil
}

type admittedContext struct {
	context.Context
	once     sync.Once
	admitted chan<- struct{}
}

func (c *admittedContext) Done() <-chan struct{} {
	c.once.Do(func() { c.admitted <- struct{}{} })
	return c.Context.Done()
}

type admissionSession struct {
	auth.Session
	admitted chan<- struct{}
}

func (s admissionSession) Renew(ctx context.Context, rejected http.Header) error {
	return s.Session.Renew(&admittedContext{Context: ctx, admitted: s.admitted}, rejected)
}

func TestTransportConcurrentRejectedRequestsShareFailedRenewal(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Tableau-Auth") == "old" {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, restSessionRejection)
			return
		}
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	signer := &heldFailureSigner{started: make(chan struct{}), release: make(chan struct{})}
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(func(string) (string, bool) { return "value", true }), nil, t.TempDir())
	defer manager.Close()
	session, err := manager.Authenticate(t.Context(), auth.Target{ServerURL: server.URL, PATNameVariable: "NAME", PATSecretVariable: "SECRET"}, signer)
	if err != nil {
		t.Fatal(err)
	}
	admitted := make(chan struct{}, 2)
	wrapped := admissionSession{Session: session, admitted: admitted}
	transport := NewTransport(server.Client(), "3.29", nil)
	request := Request{Method: http.MethodGet, ServerURL: server.URL, Path: "/api/3.29/sites", Operation: "site.list"}
	results := make(chan error, 2)
	go func() { _, err := transport.Do(t.Context(), wrapped, request); results <- err }()
	<-signer.started
	go func() { _, err := transport.Do(t.Context(), wrapped, request); results <- err }()
	for range 2 {
		select {
		case <-admitted:
		case <-time.After(5 * time.Second):
			t.Fatal("renewal was not admitted")
		}
	}
	close(signer.release)
	for range 2 {
		if err := <-results; err == nil || !strings.Contains(err.Error(), "renewal refused") {
			t.Errorf("request error = %v", err)
		}
	}
	if got := signer.calls.Load(); got != 2 {
		t.Fatalf("sign-ins = %d, want 2", got)
	}
	if _, err := transport.Do(t.Context(), wrapped, request); err != nil {
		t.Fatalf("later request: %v", err)
	}
	if got := signer.calls.Load(); got != 3 {
		t.Fatalf("sign-ins after retry = %d, want 3", got)
	}
}

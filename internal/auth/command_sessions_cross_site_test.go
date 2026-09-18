package auth_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
)

type crossSiteSigner struct {
	mu           sync.Mutex
	calls        int
	currentToken string
}

func (s *crossSiteSigner) SignIn(_ context.Context, request auth.SignInRequest) (auth.SignInResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.currentToken = fmt.Sprintf("cross-site-session-%d", s.calls)
	return auth.SignInResponse{
		Token:    s.currentToken,
		SiteLUID: request.SiteContentURL,
		UserLUID: "user",
	}, nil
}

func (s *crossSiteSigner) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *crossSiteSigner) token() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.currentToken
}

func crossSiteTarget(serverURL, site string) auth.Target {
	target := commandEnvironmentTarget()
	target.ServerURL = serverURL
	target.SiteContentURL = site
	return target
}

func crossSiteRequestStatus(t *testing.T, client *http.Client, serverURL string, session auth.Session) int {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, serverURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	session.Authorize(request)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	return response.StatusCode
}

func TestCommandSessionCrossSiteMonitorReauthenticatesAfterSharedPATSignIn(t *testing.T) {
	signer := &crossSiteSigner{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get(auth.TableauAuthHeader) != signer.token() {
			http.Error(response, "session invalidated", http.StatusUnauthorized)
			return
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	directory := t.TempDir()
	firstManager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer firstManager.Close()
	secondManager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer secondManager.Close()

	firstTarget := crossSiteTarget(server.URL, "site-a")
	secondTarget := crossSiteTarget(server.URL, "site-b")
	firstKey, err := firstManager.CoordinationKey(t.Context(), firstTarget)
	if err != nil {
		t.Fatal(err)
	}
	secondKey, err := firstManager.CoordinationKey(t.Context(), secondTarget)
	if err != nil {
		t.Fatal(err)
	}
	if firstKey == secondKey {
		t.Fatalf("site-specific coordination keys are equal: %q", firstKey)
	}

	first, err := firstManager.Authenticate(t.Context(), firstTarget, signer)
	if err != nil {
		t.Fatal(err)
	}
	if got := crossSiteRequestStatus(t, server.Client(), server.URL, first); got != http.StatusNoContent {
		t.Fatalf("first session status=%d, want %d", got, http.StatusNoContent)
	}
	if err := firstManager.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}

	second, err := secondManager.Authenticate(t.Context(), secondTarget, signer)
	if err != nil {
		t.Fatal(err)
	}
	if got := crossSiteRequestStatus(t, server.Client(), server.URL, second); got != http.StatusNoContent {
		t.Fatalf("second session status=%d, want %d", got, http.StatusNoContent)
	}
	if got := crossSiteRequestStatus(t, server.Client(), server.URL, first); got != http.StatusUnauthorized {
		t.Fatalf("invalidated first session status=%d, want %d", got, http.StatusUnauthorized)
	}
	if err := secondManager.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}

	resumed, err := firstManager.AuthenticateMonitor(t.Context(), firstTarget, signer)
	if err != nil {
		t.Fatal(err)
	}
	if resumed == first {
		t.Fatalf("monitor resumed invalidated site-a session after %d sign-ins", signer.callCount())
	}
	if got := crossSiteRequestStatus(t, server.Client(), server.URL, resumed); got != http.StatusNoContent {
		t.Fatalf("resumed session status=%d, want %d", got, http.StatusNoContent)
	}
	if got := signer.callCount(); got != 3 {
		t.Fatalf("sign-ins=%d, want 3 for site-a, site-b, and resumed site-a", got)
	}
}

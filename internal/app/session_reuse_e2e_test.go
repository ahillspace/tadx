package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
)

const sessionRejectedXML = `<tsResponse><error code="401002"><summary>Unauthorized Access</summary><detail>Invalid authentication credentials were provided.</detail></error></tsResponse>`

// sessionTableau issues a new token for every sign-in and accepts only the
// latest one, as Tableau does for one PAT.
type sessionTableau struct {
	mu      sync.Mutex
	signIns int
	current string
	lists   int
	patName string
	// beforeList, when set, runs before each list is answered, outside the
	// lock; false fails the list as if Tableau were unavailable.
	beforeList func() bool
}

func (s *sessionTableau) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if s.beforeList != nil && request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/users" && !s.beforeList() {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			s.signIns++
			var body struct {
				Credentials struct {
					Name string `json:"personalAccessTokenName"`
				} `json:"credentials"`
			}
			_ = json.NewDecoder(request.Body).Decode(&body)
			s.patName = body.Credentials.Name
			s.current = fmt.Sprintf("session-token-%d", s.signIns)
			writer.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(writer, `{"credentials":{"token":%q,"site":{"id":"site-1"},"user":{"id":"user-1"}}}`, s.current)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/users":
			s.lists++
			if request.Header.Get("X-Tableau-Auth") != s.current {
				writer.Header().Set("Content-Type", "application/xml")
				writer.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(writer, sessionRejectedXML)
				return
			}
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="1" pageSize=%q totalAvailable="1"/><users><user id="user-1" name="fixture-user" siteRole="Viewer"/></users></tsResponse>`, request.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			writer.WriteHeader(http.StatusNotFound)
		}
	})
}

// expire makes Tableau reject the session it issued last, as an idle timeout
// or a sign-in from outside TADX does.
func (s *sessionTableau) expire() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = "expired"
}

func (s *sessionTableau) counts() (signIns, lists int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.signIns, s.lists
}

func storedPATEnvironment(t *testing.T, tableau *sessionTableau) (Options, *fakePATStore) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("TADX_DEV_PAT_NAME", "")
	t.Setenv("TADX_DEV_PAT_SECRET", "")
	server := tableauFixtureServer(t, tableau.handler(t))
	t.Cleanup(server.Close)
	reference := coreauth.CredentialReference("cred_44444444444444444444444444444444")
	path := authConfig(t, string(reference))
	configuration, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	environment := configuration.Environments["dev"]
	environment.URL = server.URL
	configuration.Environments["dev"] = environment
	if err := config.Save(path, configuration); err != nil {
		t.Fatal(err)
	}
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret", Source: coreauth.CredentialSourceOSKeyring}}}
	return Options{ConfigPath: path, HTTPClient: server.Client(), PATStore: store}, store
}

func runCommand(t *testing.T, options Options, args ...string) string {
	t.Helper()
	var output bytes.Buffer
	if code := Run(t.Context(), args, &output, options); code != 0 {
		t.Fatalf("%v exit = %d, output = %s", args, code, output.String())
	}
	if strings.Contains(output.String(), "session-token") || strings.Contains(output.String(), "fixture-secret") {
		t.Fatalf("%v exposed credential material: %s", args, output.String())
	}
	return output.String()
}

func TestCommandsWithAStoredPATSignInOnce(t *testing.T) {
	tableau := &sessionTableau{}
	options, _ := storedPATEnvironment(t, tableau)
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	if signIns, lists := tableau.counts(); signIns != 1 || lists != 2 {
		t.Fatalf("sign-ins = %d, lists = %d; want the second command to reuse the saved session", signIns, lists)
	}
}

func TestCommandRenewsARejectedSavedSessionAndSucceeds(t *testing.T) {
	tableau := &sessionTableau{}
	options, store := storedPATEnvironment(t, tableau)
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	tableau.expire()
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	if signIns, lists := tableau.counts(); signIns != 2 || lists != 3 {
		t.Fatalf("sign-ins = %d, lists = %d; want one renewal and one resend", signIns, lists)
	}
	for _, saved := range store.sessions {
		if saved.Token != "session-token-2" {
			t.Fatal("the renewed session was not saved for the next command")
		}
	}
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	if signIns, _ := tableau.counts(); signIns != 2 {
		t.Fatalf("sign-ins = %d; want the next command to reuse the renewed session", signIns)
	}
}

func TestAuthCheckSignsInEveryTime(t *testing.T) {
	tableau := &sessionTableau{}
	options, _ := storedPATEnvironment(t, tableau)
	for range 2 {
		if output := runCommand(t, options, "auth", "check", "--environment", "dev"); !strings.Contains(output, "status: authenticated") {
			t.Fatalf("check output = %s", output)
		}
	}
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	if signIns, lists := tableau.counts(); signIns != 2 || lists != 1 {
		t.Fatalf("sign-ins = %d, lists = %d; want each check to sign in and the next command to reuse the checked session", signIns, lists)
	}
}

func TestCommandAfterReplacingThePATSignsInWithTheNewPAT(t *testing.T) {
	tableau := &sessionTableau{}
	options, _ := storedPATEnvironment(t, tableau)
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	options.AuthPrompter = fixedCredentialPrompter{name: "replacement-name", secret: "replacement-secret"}
	runCommand(t, options, "auth", "login", "--environment", "dev")
	runCommand(t, options, "admin", "user", "list", "--environment", "dev")
	tableau.mu.Lock()
	defer tableau.mu.Unlock()
	if tableau.signIns != 3 || tableau.patName != "replacement-name" {
		t.Fatalf("sign-ins = %d, last PAT %q; want the command after login to sign in with the replacement PAT", tableau.signIns, tableau.patName)
	}
}

// TestParallelCommandsWithAStoredPATShareOneSession runs two separate commands
// on one PAT store at the same time. Tableau answers neither list until both
// are in flight, so a command holding the PAT for its whole run blocks the other.
func TestParallelCommandsWithAStoredPATShareOneSession(t *testing.T) {
	tableau := &sessionTableau{}
	both := make(chan struct{})
	var arrived sync.WaitGroup
	arrived.Add(2)
	go func() { arrived.Wait(); close(both) }()
	tableau.beforeList = func() bool {
		arrived.Done()
		select {
		case <-both:
			return true
		case <-time.After(10 * time.Second):
			return false
		}
	}
	options, _ := storedPATEnvironment(t, tableau)
	var commands sync.WaitGroup
	for range 2 {
		commands.Go(func() {
			var output bytes.Buffer
			if code := Run(t.Context(), []string{"admin", "user", "list", "--environment", "dev"}, &output, options); code != 0 {
				t.Errorf("parallel command exit = %d, output = %s", code, output.String())
			}
		})
	}
	commands.Wait()
	if signIns, lists := tableau.counts(); signIns != 1 || lists != 2 {
		t.Fatalf("sign-ins = %d, lists = %d; want both commands to share one sign-in", signIns, lists)
	}
}

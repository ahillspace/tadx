package auth_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
)

func TestCommandSessionFailedSignInInvalidatesSuspendedMonitorBeforeRequest(t *testing.T) {
	var mu sync.Mutex
	signIns := 0
	currentToken := ""
	directory := t.TempDir()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin" {
			signIns++
			currentToken = fmt.Sprintf("fixture-session-%d", signIns)
			if signIns == 2 {
				// The server accepts the PAT and invalidates its old session, but
				// the caller cannot establish whether sign-in completed.
				http.Error(w, "sign-in response unavailable", http.StatusServiceUnavailable)
				return
			}
			fmt.Fprintf(w, `{"credentials":{"token":%q,"site":{"id":"site"},"user":{"id":"user"}}}`, currentToken)
			return
		}
		if r.Method != http.MethodGet || r.URL.Path != "/probe" {
			t.Errorf("unexpected fixture request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get(auth.TableauAuthHeader) != currentToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	signer := tableauauth.NewClient(tableau.NewTransport(server.Client(), "3.29", nil))
	firstManager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer firstManager.Close()
	secondManager := auth.NewCommandSessions(nil, nil, directory)
	defer secondManager.Close()
	target := commandEnvironmentTarget()
	target.ServerURL = server.URL
	first, err := firstManager.AuthenticateCredentials(t.Context(), target, commandCredential(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := firstManager.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	second, err := secondManager.AuthenticateCredentials(t.Context(), target, commandCredential(), signer)
	if err == nil || second != nil {
		t.Fatal("ambiguous sign-in returned an authenticated session")
	}
	if err := secondManager.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readCommandEpoch(t, commandEpochPath(t, directory)); got != 2 {
		t.Errorf("failed sign-in rolled invalidation back to epoch %d, want 2", got)
	}
	resumed, err := firstManager.AuthenticateMonitor(t.Context(), target, signer)
	if err != nil {
		t.Fatal(err)
	}
	status := crossSiteRequestStatus(t, server.Client(), server.URL+"/probe", resumed)
	mu.Lock()
	calls := signIns
	mu.Unlock()
	if resumed == first || status != http.StatusNoContent || calls != 3 {
		t.Fatalf("resumed invalidated session: cached=%v status=%d sign-ins=%d", resumed == first, status, calls)
	}
}

func TestCommandSessionEpochWriteFailurePreventsSignInAndPreservesPriorEpoch(t *testing.T) {
	directory := t.TempDir()
	signer := &commandSigner{}
	first := auth.NewCommandSessions(nil, nil, directory)
	defer first.Close()
	if _, err := first.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if err := first.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	path := commandEpochPath(t, directory)
	blockEpochReplacement(t, directory, path)
	second := auth.NewCommandSessions(nil, nil, directory)
	defer second.Close()
	session, err := second.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer)
	if err == nil || session != nil || signer.calls.Load() != 1 {
		t.Fatalf("failed epoch publication contacted signer: session=%v error=%v sign-ins=%d", session != nil, err, signer.calls.Load())
	}
	if got := readCommandEpoch(t, path); got != 1 {
		t.Fatalf("failed publication changed prior epoch to %d", got)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".epoch-") {
			t.Fatal("failed publication retained a staging file")
		}
	}
}

func TestCommandSessionExhaustedEpochPreventsSignIn(t *testing.T) {
	directory := t.TempDir()
	signer := &commandSigner{}
	manager := auth.NewCommandSessions(nil, nil, directory)
	defer manager.Close()
	if _, err := manager.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if err := manager.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	path := commandEpochPath(t, directory)
	if err := os.WriteFile(path, []byte(strconv.FormatUint(^uint64(0), 10)), 0600); err != nil {
		t.Fatal(err)
	}
	fresh := auth.NewCommandSessions(nil, nil, directory)
	defer fresh.Close()
	session, err := fresh.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer)
	if err == nil || !strings.Contains(err.Error(), "exhausted") || session != nil || signer.calls.Load() != 1 {
		t.Fatalf("exhausted epoch contacted signer: session=%v error=%v sign-ins=%d", session != nil, err, signer.calls.Load())
	}
	if got := readCommandEpoch(t, path); got != ^uint64(0) {
		t.Fatal("exhausted epoch changed")
	}
}

func TestCommandSessionLastEpochRemainsReadableAndCannotAdvance(t *testing.T) {
	directory := t.TempDir()
	signer := &commandSigner{}
	manager := auth.NewCommandSessions(nil, nil, directory)
	defer manager.Close()
	if _, err := manager.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if err := manager.Suspend(t.Context()); err != nil {
		t.Fatal(err)
	}
	path := commandEpochPath(t, directory)
	if err := os.WriteFile(path, []byte(strconv.FormatUint(^uint64(0)-1, 10)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	last := auth.NewCommandSessions(nil, nil, directory)
	defer last.Close()
	if _, err := last.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if err := last.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readCommandEpoch(t, path); got != ^uint64(0) {
		t.Fatal("last sign-in did not publish the final epoch")
	}
	fresh := auth.NewCommandSessions(nil, nil, directory)
	defer fresh.Close()
	_, err := fresh.AuthenticateCredentials(t.Context(), commandTarget(), commandCredential(), signer)
	if err == nil || !strings.Contains(err.Error(), "exhausted") || signer.calls.Load() != 2 {
		t.Fatalf("final persisted epoch was not readable or contacted signer: error=%v sign-ins=%d", err, signer.calls.Load())
	}
}

func commandEpochPath(t *testing.T, directory string) string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var path string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "epoch-") {
			if path != "" {
				t.Fatal("fixture has multiple credential epochs")
			}
			path = filepath.Join(directory, entry.Name())
		}
	}
	if path == "" {
		t.Fatal("fixture credential epoch is missing")
	}
	return path
}

func readCommandEpoch(t *testing.T, path string) uint64 {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	epoch, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
	if err != nil {
		t.Fatalf("epoch is malformed: %v", err)
	}
	return epoch
}

func blockEpochReplacement(t *testing.T, directory, path string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		// Go's Windows file handles share reads and writes but not deletion.
		// Existing contents remain readable while rename replacement fails.
		file, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { file.Close() })
		return
	}
	if err := os.Chmod(directory, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(directory, 0700) })
	probe, err := os.CreateTemp(directory, "write-probe-")
	if err == nil {
		probe.Close()
		os.Remove(probe.Name())
		t.Skip("current account bypasses fixture directory write permissions")
	}
}

package auth_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/auth"
)

type commandSigner struct{ calls atomic.Int32 }

type failedCommandSigner struct{}

func (failedCommandSigner) SignIn(context.Context, auth.SignInRequest) (auth.SignInResponse, error) {
	return auth.SignInResponse{}, errors.New("sign-in failed")
}

func TestCommandFailedSignInCloseReleasesCredential(t *testing.T) {
	directory := t.TempDir()
	first := auth.NewCommandSessions(nil, nil, directory)
	if _, err := first.AuthenticateCredentials(context.Background(), commandTarget(), commandCredential(), failedCommandSigner{}); err == nil {
		t.Fatal("expected failed sign-in")
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := auth.NewCommandSessions(nil, nil, directory)
	defer second.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := second.AuthenticateCredentials(ctx, commandTarget(), commandCredential(), &commandSigner{}); err != nil {
		t.Fatal(err)
	}
}

func (s *commandSigner) SignIn(_ context.Context, _ auth.SignInRequest) (auth.SignInResponse, error) {
	s.calls.Add(1)
	return auth.SignInResponse{Token: "test-session", SiteLUID: "site", UserLUID: "user"}, nil
}
func commandCredential() auth.PATCredentials {
	return auth.PATCredentials{Name: "test-name", Secret: "test-secret"}
}
func commandTarget() auth.Target {
	return auth.Target{ServerURL: "https://tableau.example.com", SiteContentURL: "site"}
}

func commandEnvironmentLookup(name string) (string, bool) {
	if name == "PAT_NAME" {
		return "test-name", true
	}
	if name == "PAT_SECRET" {
		return "test-secret", true
	}
	return "", false
}

func commandEnvironmentTarget() auth.Target {
	target := commandTarget()
	target.PATNameVariable, target.PATSecretVariable = "PAT_NAME", "PAT_SECRET"
	return target
}

func TestCommandSessionResolvesCredentialsAndSignsInOnceForConcurrentReaders(t *testing.T) {
	var lookups atomic.Int32
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(func(name string) (string, bool) { lookups.Add(1); return "test-" + name, true }), nil, t.TempDir())
	defer manager.Close()
	target := commandTarget()
	target.PATNameVariable, target.PATSecretVariable = "NAME", "SECRET"
	signer := &commandSigner{}
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := manager.Authenticate(context.Background(), target, signer); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if signer.calls.Load() != 1 || lookups.Load() != 2 {
		t.Fatalf("signins=%d credential lookups=%d", signer.calls.Load(), lookups.Load())
	}
}

func TestCommandSessionSuspendReusesSessionWhenEpochIsUnchanged(t *testing.T) {
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, t.TempDir())
	defer manager.Close()
	signer := &commandSigner{}
	target := commandEnvironmentTarget()
	first, err := manager.AuthenticateCredentials(context.Background(), target, commandCredential(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Suspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	monitor, err := manager.AuthenticateMonitor(context.Background(), target, signer)
	if err != nil {
		t.Fatal(err)
	}
	if monitor != first || signer.calls.Load() != 1 {
		t.Fatalf("monitor session=%v first=%v signins=%d, want cached session and one sign-in", monitor, first, signer.calls.Load())
	}
}

func TestCommandSessionMonitorDropsSessionAfterSharedEpochChanges(t *testing.T) {
	directory := t.TempDir()
	firstManager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer firstManager.Close()
	secondManager := auth.NewCommandSessions(auth.LookupEnvFunc(commandEnvironmentLookup), nil, directory)
	defer secondManager.Close()
	signer := &commandSigner{}
	target := commandEnvironmentTarget()
	first, err := firstManager.AuthenticateCredentials(context.Background(), target, commandCredential(), signer)
	if err != nil {
		t.Fatal(err)
	}
	if err := firstManager.Suspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := secondManager.AuthenticateCredentials(context.Background(), target, commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if err := secondManager.Suspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	monitor, err := firstManager.AuthenticateMonitor(context.Background(), target, signer)
	if err != nil {
		t.Fatal(err)
	}
	if monitor == first || signer.calls.Load() != 3 {
		t.Fatalf("monitor session=%v first=%v signins=%d, want re-sign-in after shared epoch change", monitor, first, signer.calls.Load())
	}
}

func TestCommandSessionCoordinationKeyIsTargetBoundAndOpaque(t *testing.T) {
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(func(name string) (string, bool) {
		if name == "PAT_NAME" {
			return "test-name", true
		}
		return "test-secret", true
	}), nil, t.TempDir())
	defer manager.Close()
	target := commandTarget()
	target.PATNameVariable, target.PATSecretVariable = "PAT_NAME", "PAT_SECRET"
	first, err := manager.CoordinationKey(context.Background(), target)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 64 || strings.Contains(first, "test-") {
		t.Fatalf("coordination key=%q is not opaque", first)
	}
	target.ServerURL = "https://TABLEAU.EXAMPLE.COM:443/"
	same, err := manager.CoordinationKey(context.Background(), target)
	if err != nil || same != first {
		t.Fatalf("normalized target key=%q first=%q error=%v", same, first, err)
	}
	target.SiteContentURL = "other-site"
	other, err := manager.CoordinationKey(context.Background(), target)
	if err != nil || other == first {
		t.Fatalf("site-specific key=%q first=%q error=%v", other, first, err)
	}
}

type orderedSigner struct {
	mu    sync.Mutex
	sites []string
}

func (s *orderedSigner) SignIn(_ context.Context, request auth.SignInRequest) (auth.SignInResponse, error) {
	s.mu.Lock()
	s.sites = append(s.sites, request.SiteContentURL)
	s.mu.Unlock()
	return auth.SignInResponse{Token: "test-session-" + request.SiteContentURL, SiteLUID: request.SiteContentURL, UserLUID: "user"}, nil
}

func (s *orderedSigner) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.sites...)
}

func TestCommandSessionForegroundAuthenticationPrecedesWaitingMonitor(t *testing.T) {
	t.Run("independent_managers", func(t *testing.T) {
		testForegroundAuthenticationPrecedesWaitingMonitor(t, false)
	})
	t.Run("same_manager", func(t *testing.T) {
		testForegroundAuthenticationPrecedesWaitingMonitor(t, true)
	})
}

func testForegroundAuthenticationPrecedesWaitingMonitor(t *testing.T, sameManager bool) {
	t.Helper()
	directory := t.TempDir()
	holder := auth.NewCommandSessions(nil, nil, directory)
	defer holder.Close()
	target := commandTarget()
	target.SiteContentURL = "holder-site"
	if _, err := holder.AuthenticateCredentials(context.Background(), target, commandCredential(), &commandSigner{}); err != nil {
		t.Fatal(err)
	}
	var lookups atomic.Int32
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(func(name string) (string, bool) {
		lookups.Add(1)
		if name == "PAT_NAME" {
			return "test-name", true
		}
		return "test-secret", true
	}), nil, directory)
	defer manager.Close()
	foregroundManager := manager
	if !sameManager {
		foregroundManager = auth.NewCommandSessions(nil, nil, directory)
		defer foregroundManager.Close()
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	monitorTarget := commandTarget()
	monitorTarget.SiteContentURL = "monitor-site"
	monitorTarget.PATNameVariable, monitorTarget.PATSecretVariable = "PAT_NAME", "PAT_SECRET"
	signer := &orderedSigner{}
	monitorDone := make(chan error, 1)
	go func() {
		_, err := manager.AuthenticateMonitor(ctx, monitorTarget, signer)
		if err == nil {
			err = manager.Suspend(ctx)
		}
		monitorDone <- err
	}()
	deadline := time.Now().Add(time.Second)
	for lookups.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if lookups.Load() < 2 {
		t.Fatal("monitor did not resolve its PAT before foreground request")
	}
	foregroundDone := make(chan error, 1)
	go func() {
		foregroundTarget := commandTarget()
		foregroundTarget.SiteContentURL = "foreground-site"
		_, err := foregroundManager.AuthenticateCredentials(ctx, foregroundTarget, commandCredential(), signer)
		if err == nil {
			err = foregroundManager.Suspend(ctx)
		}
		foregroundDone <- err
	}()
	time.Sleep(75 * time.Millisecond)
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-foregroundDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("foreground authentication did not complete")
	}
	select {
	case err := <-monitorDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("monitor authentication did not complete")
	}
	if got := signer.snapshot(); len(got) != 2 || got[0] != "foreground-site" || got[1] != "monitor-site" {
		t.Fatalf("sign-in order=%v, want foreground before monitor", got)
	}
}

func TestCommandSessionIdentitySeparatesCredentialsServerAndSite(t *testing.T) {
	manager := auth.NewCommandSessions(nil, nil, t.TempDir())
	defer manager.Close()
	signer := &commandSigner{}
	target, credential := commandTarget(), commandCredential()
	first, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer)
	if err != nil {
		t.Fatal(err)
	}
	target.ServerURL = "https://TABLEAU.EXAMPLE.COM:443/"
	target.Environment = "other-alias"
	same, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer)
	if err != nil || same != first || signer.calls.Load() != 1 {
		t.Fatalf("normalized identity did not reuse: %v", err)
	}
	target.SiteContentURL = "other-site"
	other, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer)
	if err != nil || other == first {
		t.Fatalf("site reused session: %v", err)
	}
	target.SiteContentURL = "site"
	if _, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer); err != nil {
		t.Fatal(err)
	}
	credential.Secret = "different-secret"
	if _, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer); err != nil {
		t.Fatal(err)
	}
	credential.Name = "different-name"
	if _, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer); err != nil {
		t.Fatal(err)
	}
	target.ServerURL = "https://different.example.com"
	if _, err := manager.AuthenticateCredentials(context.Background(), target, credential, signer); err != nil {
		t.Fatal(err)
	}
	if signer.calls.Load() != 6 {
		t.Fatalf("signins=%d", signer.calls.Load())
	}
}

func TestCommandEnvironmentCredentialSnapshotSpansTargets(t *testing.T) {
	var lookups int
	manager := auth.NewCommandSessions(auth.LookupEnvFunc(func(name string) (string, bool) { lookups++; return "test-" + name, true }), nil, t.TempDir())
	defer manager.Close()
	target := commandTarget()
	target.PATNameVariable, target.PATSecretVariable = "NAME", "SECRET"
	signer := &commandSigner{}
	if _, err := manager.Authenticate(context.Background(), target, signer); err != nil {
		t.Fatal(err)
	}
	target.SiteContentURL = "another-site"
	if _, err := manager.Authenticate(context.Background(), target, signer); err != nil {
		t.Fatal(err)
	}
	if lookups != 2 || signer.calls.Load() != 2 {
		t.Fatalf("environment reads=%d signins=%d", lookups, signer.calls.Load())
	}
}

func TestCommandCredentialLockWaitIsCancelableAndDifferentPATIndependent(t *testing.T) {
	directory := t.TempDir()
	first := auth.NewCommandSessions(nil, nil, directory)
	defer first.Close()
	second := auth.NewCommandSessions(nil, nil, directory)
	defer second.Close()
	signer := &commandSigner{}
	if _, err := first.AuthenticateCredentials(context.Background(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	_, err := second.AuthenticateCredentials(ctx, commandTarget(), commandCredential(), signer)
	if !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "PAT is in use") || signer.calls.Load() != 1 {
		t.Fatalf("signins=%d error=%v", signer.calls.Load(), err)
	}
	different := commandCredential()
	different.Secret = "other-secret"
	if _, err := second.AuthenticateCredentials(context.Background(), commandTarget(), different, signer); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := second.AuthenticateCredentials(context.Background(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
	if signer.calls.Load() != 3 {
		t.Fatalf("signins=%d", signer.calls.Load())
	}
	files, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "epoch-") {
			if len(file.Name()) != 74 || !strings.HasSuffix(file.Name(), ".txt") {
				t.Fatalf("invalid epoch name=%q", file.Name())
			}
			data, err := os.ReadFile(filepath.Join(directory, file.Name()))
			_, parseErr := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 64)
			if err != nil || parseErr != nil {
				t.Fatalf("invalid epoch contents name=%q value=%q error=%v", file.Name(), data, err)
			}
			continue
		}
		identity := strings.TrimSuffix(strings.TrimSuffix(file.Name(), ".lock"), ".admission")
		if len(identity) != 64 || !strings.HasSuffix(file.Name(), ".lock") || strings.Contains(file.Name(), "test-") {
			t.Fatalf("nonopaque lock name=%q", file.Name())
		}
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() != 0 {
			t.Fatalf("lock persisted credential/session data: length=%d", info.Size())
		}
	}
}

func TestCommandCredentialLockAcrossProcesses(t *testing.T) {
	if os.Getenv("TADX_COMMAND_LOCK_HELPER") == "1" {
		manager := auth.NewCommandSessions(nil, nil, os.Getenv("TADX_COMMAND_LOCK_DIRECTORY"))
		defer manager.Close()
		if _, err := manager.AuthenticateCredentials(context.Background(), commandTarget(), commandCredential(), &commandSigner{}); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(os.Getenv("TADX_COMMAND_LOCK_READY"), nil, 0600); err != nil {
			t.Fatal(err)
		}
		_, _ = io.CopyN(io.Discard, os.Stdin, 1)
		return
	}
	directory := t.TempDir()
	ready := filepath.Join(directory, "ready")
	child := exec.Command(os.Args[0], "-test.run=^TestCommandCredentialLockAcrossProcesses$")
	child.Env = append(os.Environ(), "TADX_COMMAND_LOCK_HELPER=1", "TADX_COMMAND_LOCK_DIRECTORY="+directory, "TADX_COMMAND_LOCK_READY="+ready)
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = child.Wait() })
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("child did not acquire credential lock")
		}
		time.Sleep(5 * time.Millisecond)
	}
	manager := auth.NewCommandSessions(nil, nil, directory)
	defer manager.Close()
	signer := &commandSigner{}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := manager.AuthenticateCredentials(ctx, commandTarget(), commandCredential(), signer); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cross-process contention error=%v", err)
	}
	if signer.calls.Load() != 0 {
		t.Fatal("sign-in ran before acquiring credential lock")
	}
	different := commandCredential()
	different.Secret = "other-secret"
	if _, err := manager.AuthenticateCredentials(context.Background(), commandTarget(), different, signer); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	if err := child.Wait(); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AuthenticateCredentials(context.Background(), commandTarget(), commandCredential(), signer); err != nil {
		t.Fatal(err)
	}
}

func TestCommandAdditionalCredentialContentionDoesNotDeadlock(t *testing.T) {
	directory := t.TempDir()
	first, second := auth.NewCommandSessions(nil, nil, directory), auth.NewCommandSessions(nil, nil, directory)
	defer first.Close()
	defer second.Close()
	a, b := commandCredential(), commandCredential()
	b.Secret = "independent-secret"
	signer := &commandSigner{}
	for _, item := range []struct {
		manager    *auth.CommandSessions
		credential auth.PATCredentials
	}{{first, a}, {second, b}} {
		if _, err := item.manager.AuthenticateCredentials(context.Background(), commandTarget(), item.credential, signer); err != nil {
			t.Fatal(err)
		}
	}
	for _, item := range []struct {
		manager    *auth.CommandSessions
		credential auth.PATCredentials
	}{{first, b}, {second, a}} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_, err := item.manager.AuthenticateCredentials(ctx, commandTarget(), item.credential, signer)
		cancel()
		if err == nil || errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "PAT is in use") {
			t.Fatalf("additional credential should fail promptly, got %v", err)
		}
	}
	if signer.calls.Load() != 2 {
		t.Fatalf("contended sign-in ran: %d", signer.calls.Load())
	}
}

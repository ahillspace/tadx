package update

import (
	"context"
	"errors"
	"fmt"
	action "github.com/ahillspace/tadx/actions/update"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestInstallationTargetAcceptsArbitraryExecutableBasename(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	path, err := (Runtime{}).InstallationTarget()
	if err != nil || path != filepath.ToSlash(want) {
		t.Fatalf("path=%q error=%v want=%q", path, err, filepath.ToSlash(want))
	}
}

func TestUpdaterCancellationStopsDescendants(t *testing.T) {
	for _, startupDelay := range []time.Duration{0, 4 * time.Second} {
		t.Run(startupDelay.String(), func(t *testing.T) {
			testUpdaterCancellationStopsDescendants(t, startupDelay)
		})
	}
}

func testUpdaterCancellationStopsDescendants(t *testing.T, startupDelay time.Duration) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	startupDeadline := time.Now().Add(15 * time.Second)
	if err := listener.SetDeadline(startupDeadline); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type processResult struct {
		output []byte
		err    error
	}
	completed := make(chan processResult, 1)
	go func() {
		output, err := runProcess(ctx, 30*time.Second, nil, executable, "-test.run=^TestUpdaterProcessTreeHelper$", "--", "--update-process-fixture", "parent", listener.Addr().String(), startupDelay.String())
		completed <- processResult{output: output, err: err}
		close(completed)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-completed:
		case <-time.After(5 * time.Second):
			t.Error("installer fixture did not stop during cleanup")
		}
	})
	connection, err := listener.AcceptTCP()
	if err != nil {
		t.Fatalf("fixture child did not connect before the startup deadline: %v", err)
	}
	defer connection.Close()
	if err := connection.SetReadDeadline(startupDeadline); err != nil {
		t.Fatal(err)
	}
	var ready [1]byte
	if _, err := io.ReadFull(connection, ready[:]); err != nil || ready[0] != 'r' {
		t.Fatalf("fixture child did not acknowledge readiness: ready=%q err=%v", ready, err)
	}
	cancel()
	select {
	case result := <-completed:
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("expected cancellation, got %v: %s", result.err, result.output)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("installer parent did not stop after cancellation")
	}
	if err := connection.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	_, err = connection.Read(ready[:])
	closed := errors.Is(err, io.EOF) || errors.Is(err, syscall.ECONNRESET)
	if runtime.GOOS == "windows" {
		// Windows reports WSAECONNRESET when the job's descendant is terminated.
		const winsockConnectionReset = syscall.Errno(10054)
		closed = closed || errors.Is(err, winsockConnectionReset)
	}
	if !closed {
		t.Fatalf("installer descendant kept its connection after cancellation: %v", err)
	}
}

func TestInstallerProgramIsAnAbsoluteSystemPath(t *testing.T) {
	program, err := installerProgram()
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(program) {
		t.Fatalf("installer program %q is resolved through PATH", program)
	}
	if info, err := os.Stat(program); err != nil || info.IsDir() {
		t.Fatalf("installer program %q is not a file: %v", program, err)
	}
}

func TestChildEnvironmentWithholdsPATVariables(t *testing.T) {
	environment := []string{
		"TADX_DEV_PAT_NAME=name",
		"TADX_DEV_PAT_SECRET=secret",
		"tadx_prod_pat_secret=secret",
		"CUSTOM_TABLEAU_NAME=name",
		"custom_tableau_secret=secret",
		"GH_TOKEN=kept",
		"HTTPS_PROXY=kept",
		"TADX_LOG_LEVEL=kept",
		"TADX_PAT_SECRET_NOTE=kept",
		"PATH=kept",
		"=C:=kept",
	}
	got := childEnvironment(environment, []string{"CUSTOM_TABLEAU_NAME", "CUSTOM_TABLEAU_SECRET", ""})
	want := []string{"GH_TOKEN=kept", "HTTPS_PROXY=kept", "TADX_LOG_LEVEL=kept", "TADX_PAT_SECRET_NOTE=kept", "PATH=kept", "=C:=kept"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("child environment = %q, want %q", got, want)
	}
}

func TestUpdaterChildrenDoNotReceivePATVariables(t *testing.T) {
	t.Setenv("TADX_DEV_PAT_NAME", "name")
	t.Setenv("TADX_DEV_PAT_SECRET", "secret")
	t.Setenv("CUSTOM_TABLEAU_SECRET", "secret")
	t.Setenv("GH_TOKEN", "kept")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	execute := Runtime{CredentialVariables: func() []string { return []string{"CUSTOM_TABLEAU_SECRET"} }}.runner()
	output, err := execute(t.Context(), 30*time.Second, executable, "-test.run=^TestUpdaterEnvironmentHelper$", "--", "--update-environment-fixture")
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	names := map[string]bool{}
	for _, line := range strings.Split(string(output), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "env:"); ok {
			names[strings.ToUpper(name)] = true
		}
	}
	for _, withheld := range []string{"TADX_DEV_PAT_NAME", "TADX_DEV_PAT_SECRET", "CUSTOM_TABLEAU_SECRET"} {
		if names[withheld] {
			t.Errorf("updater child received %s", withheld)
		}
	}
	if !names["GH_TOKEN"] || !names["PATH"] {
		t.Fatalf("updater child lost required variables: %v", names)
	}
}

func TestUpdaterEnvironmentHelper(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "--update-environment-fixture" {
		return
	}
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		fmt.Println("env:" + name)
	}
	os.Exit(0)
}

func TestUpdaterProcessTreeHelper(t *testing.T) {
	offset := -1
	for i, arg := range os.Args {
		if arg == "--update-process-fixture" {
			offset = i
			break
		}
	}
	if offset < 0 {
		return
	}
	if len(os.Args) != offset+4 {
		t.Fatal("invalid installer fixture arguments")
	}
	mode, address := os.Args[offset+1], os.Args[offset+2]
	startupDelay, err := time.ParseDuration(os.Args[offset+3])
	if err != nil {
		t.Fatal(err)
	}
	if mode == "child" {
		time.Sleep(startupDelay)
		connection, err := net.DialTimeout("tcp", address, 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer connection.Close()
		if _, err := connection.Write([]byte{'r'}); err != nil {
			t.Fatal(err)
		}
		// The observer never sends data, so this holds the connection until the
		// process is stopped or a failing test closes its connection during cleanup.
		var release [1]byte
		_, _ = io.ReadFull(connection, release[:])
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(4)
	}
	child := exec.Command(executable, "-test.run=^TestUpdaterProcessTreeHelper$", "--", "--update-process-fixture", "child", address, startupDelay.String())
	child.Stdout, child.Stderr = os.Stdout, os.Stderr
	if err := child.Run(); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestInstallerUsesPinnedReleaseAndTargets(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			calls := 0
			err := install(context.Background(), platform, "/resolved/installer", t.TempDir(), action.Release{Version: "1.2.3"}, []string{"codex", "claude"}, func(_ context.Context, _ time.Duration, program string, args ...string) ([]byte, error) {
				calls++
				if program != "/resolved/installer" {
					t.Fatalf("installer program = %q, want the resolved path", program)
				}
				joined := strings.Join(args, " ")
				if !strings.Contains(joined, "1.2.3") || !strings.Contains(joined, "codex") || !strings.Contains(joined, "claude") {
					t.Fatal(joined)
				}
				var path string
				if platform == "windows" {
					path = args[5]
				} else {
					path = args[0]
				}
				if _, err := os.Stat(path); err != nil {
					t.Fatal(err)
				}
				return nil, nil
			})
			if err != nil || calls != 1 {
				t.Fatalf("%v %d", err, calls)
			}
		})
	}
}
func TestInstallerFailurePreserved(t *testing.T) {
	err := install(context.Background(), "linux", "/bin/sh", t.TempDir(), action.Release{Version: "1.2.3"}, nil, func(context.Context, time.Duration, string, ...string) ([]byte, error) {
		return []byte("Guidance failed"), errors.New("exit 1")
	})
	if err == nil || !strings.Contains(err.Error(), "Guidance failed") {
		t.Fatal(err)
	}
}
func TestCancelledInstallDoesNotPromiseRollback(t *testing.T) {
	err := install(context.Background(), "linux", "/bin/sh", t.TempDir(), action.Release{Version: "1.2.3"}, nil, func(context.Context, time.Duration, string, ...string) ([]byte, error) {
		return nil, context.DeadlineExceeded
	})
	if err == nil || !strings.Contains(err.Error(), "rollback is not confirmed") || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
}
func TestCaptureBound(t *testing.T) {
	c := &capture{limit: 3}
	n, err := c.Write([]byte("abcdef"))
	if n != 6 || err != nil || string(c.body) != "abc" {
		t.Fatal(c, n, err)
	}
}

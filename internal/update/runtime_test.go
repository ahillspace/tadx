package update

import (
	"context"
	"errors"
	action "github.com/ahillspace/tadx/actions/update"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUpdaterCancellationStopsDescendants(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	marker := filepath.Join(dir, "late-write")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := make(chan bool, 1)
	go func() {
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(ready); err == nil {
				observed <- true
				cancel()
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		observed <- false
		cancel()
	}()
	_, err = run(ctx, 5*time.Second, executable, "-test.run=^TestUpdaterProcessTreeHelper$", "--", "--update-process-fixture", "parent", ready, marker)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if !<-observed {
		t.Fatal("fixture child never started")
	}
	time.Sleep(700 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("installer descendant survived cancellation: %v", err)
	}
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
	mode, ready, marker := os.Args[offset+1], os.Args[offset+2], os.Args[offset+3]
	if mode == "child" {
		if err := os.WriteFile(ready, []byte("ready"), 0600); err != nil {
			os.Exit(2)
		}
		time.Sleep(500 * time.Millisecond)
		if err := os.WriteFile(marker, []byte("late"), 0600); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(4)
	}
	child := exec.Command(executable, "-test.run=^TestUpdaterProcessTreeHelper$", "--", "--update-process-fixture", "child", ready, marker)
	if err := child.Run(); err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestInstallerUsesPinnedReleaseAndTargets(t *testing.T) {
	for _, platform := range []string{"windows", "linux"} {
		t.Run(platform, func(t *testing.T) {
			calls := 0
			err := install(context.Background(), platform, t.TempDir(), action.Release{Version: "1.2.3"}, []string{"codex", "claude"}, func(_ context.Context, _ time.Duration, program string, args ...string) ([]byte, error) {
				calls++
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
	err := install(context.Background(), "linux", t.TempDir(), action.Release{Version: "1.2.3"}, nil, func(context.Context, time.Duration, string, ...string) ([]byte, error) {
		return []byte("Guidance failed"), errors.New("exit 1")
	})
	if err == nil || !strings.Contains(err.Error(), "Guidance failed") {
		t.Fatal(err)
	}
}
func TestCancelledInstallDoesNotPromiseRollback(t *testing.T) {
	err := install(context.Background(), "linux", t.TempDir(), action.Release{Version: "1.2.3"}, nil, func(context.Context, time.Duration, string, ...string) ([]byte, error) {
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

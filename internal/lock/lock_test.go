package lock_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

func TestTryAcquireFailsWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tadx.lock")
	held, err := lock.Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	// A second independent open of the same lock file must observe contention.
	if _, err := lock.TryAcquire(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquire() while held error = %v, want ErrLocked", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	// After release the lock is available again.
	regained, err := lock.TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire() after release error = %v", err)
	}
	if err := regained.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

// TestAcquireBlocksAcrossProcesses proves the lock is a genuine inter-process
// (kernel) lock by holding it in a child process and confirming a TryAcquire in
// the parent fails until the child exits.
func TestAcquireBlocksAcrossProcesses(t *testing.T) {
	if os.Getenv("TADX_LOCK_CHILD") == "1" {
		holdChild(t)
		return
	}
	path := filepath.Join(t.TempDir(), ".tadx.lock")
	ready := filepath.Join(filepath.Dir(path), "ready")
	release := filepath.Join(filepath.Dir(path), "release")

	cmd := exec.Command(os.Args[0], "-test.run", "TestAcquireBlocksAcrossProcesses")
	cmd.Env = append(os.Environ(),
		"TADX_LOCK_CHILD=1",
		"TADX_LOCK_PATH="+path,
		"TADX_LOCK_READY="+ready,
		"TADX_LOCK_RELEASE="+release,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(release, nil, 0o600)
		_ = cmd.Wait()
	})

	waitFor(t, ready)
	if _, err := lock.TryAcquire(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquire() while child holds lock error = %v, want ErrLocked", err)
	}
	// Signal the child to release, then confirm we can take the lock.
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child exited with error: %v", err)
	}
	handle, err := lock.Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() after child released error = %v", err)
	}
	if err := handle.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func holdChild(t *testing.T) {
	path := os.Getenv("TADX_LOCK_PATH")
	ready := os.Getenv("TADX_LOCK_READY")
	release := os.Getenv("TADX_LOCK_RELEASE")
	handle, err := lock.Acquire(path)
	if err != nil {
		t.Fatalf("child Acquire() error = %v", err)
	}
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		t.Fatalf("child signal ready: %v", err)
	}
	waitFor(t, release)
	if err := handle.Release(); err != nil {
		t.Fatalf("child Release() error = %v", err)
	}
}

func waitFor(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %q", path)
}

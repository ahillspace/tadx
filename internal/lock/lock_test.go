package lock_test

import (
	"context"
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

func TestSharedHandlesCoexistAndBlockExclusive(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tadx.lock")
	first, err := lock.TryAcquireShared(path)
	if err != nil {
		t.Fatalf("TryAcquireShared() first error = %v", err)
	}
	defer first.Release()
	second, err := lock.TryAcquireShared(path)
	if err != nil {
		t.Fatalf("TryAcquireShared() second error = %v", err)
	}
	defer second.Release()

	if _, err := lock.TryAcquire(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquire() while two shared handles held error = %v, want ErrLocked", err)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("first Release() error = %v", err)
	}
	if _, err := lock.TryAcquire(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquire() while one shared handle held error = %v, want ErrLocked", err)
	}
	if err := second.Release(); err != nil {
		t.Fatalf("second Release() error = %v", err)
	}

	exclusive, err := lock.TryAcquire(path)
	if err != nil {
		t.Fatalf("TryAcquire() after shared handles released error = %v", err)
	}
	if err := exclusive.Release(); err != nil {
		t.Fatalf("exclusive Release() error = %v", err)
	}
}

func TestSharedAcquireContextCanceledWhileExclusiveHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".tadx.lock")
	exclusive, err := lock.Acquire(path)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer exclusive.Release()
	if _, err := lock.TryAcquireShared(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquireShared() while exclusive handle held error = %v, want ErrLocked", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		handle, err := lock.AcquireSharedContext(ctx, path)
		if handle != nil {
			_ = handle.Release()
		}
		result <- err
	}()
	select {
	case err := <-result:
		t.Fatalf("AcquireSharedContext() returned before cancellation: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("AcquireSharedContext() after cancellation error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for AcquireSharedContext() cancellation")
	}
}

func TestSharedAcquireReleasesOnProcessExit(t *testing.T) {
	if os.Getenv("TADX_LOCK_SHARED_CHILD") == "1" {
		holdSharedChild(t)
		return
	}
	path := filepath.Join(t.TempDir(), ".tadx.lock")
	ready := filepath.Join(filepath.Dir(path), "shared-ready")
	exit := filepath.Join(filepath.Dir(path), "shared-exit")
	cmd := exec.Command(os.Args[0], "-test.run", "TestSharedAcquireReleasesOnProcessExit")
	cmd.Env = append(os.Environ(),
		"TADX_LOCK_SHARED_CHILD=1",
		"TADX_LOCK_PATH="+path,
		"TADX_LOCK_READY="+ready,
		"TADX_LOCK_EXIT="+exit,
	)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	waitFor(t, ready)
	shared, err := lock.TryAcquireShared(path)
	if err != nil {
		t.Fatalf("TryAcquireShared() while child holds shared handle error = %v", err)
	}
	if err := shared.Release(); err != nil {
		t.Fatalf("shared Release() error = %v", err)
	}
	if _, err := lock.TryAcquire(path); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquire() while child holds shared handle error = %v, want ErrLocked", err)
	}
	if err := os.WriteFile(exit, nil, 0o600); err != nil {
		t.Fatalf("signal child exit: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		waited = true
		t.Fatalf("child exited with error: %v", err)
	}
	waited = true

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	exclusive, err := lock.AcquireContext(ctx, path)
	if err != nil {
		t.Fatalf("AcquireContext() after child exit error = %v", err)
	}
	if err := exclusive.Release(); err != nil {
		t.Fatalf("exclusive Release() error = %v", err)
	}
}

func holdSharedChild(t *testing.T) {
	path := os.Getenv("TADX_LOCK_PATH")
	ready := os.Getenv("TADX_LOCK_READY")
	exit := os.Getenv("TADX_LOCK_EXIT")
	shared, err := lock.AcquireSharedContext(t.Context(), path)
	if err != nil {
		t.Fatalf("child AcquireSharedContext() error = %v", err)
	}
	defer shared.Release()
	if err := os.WriteFile(ready, nil, 0o600); err != nil {
		t.Fatalf("child signal ready: %v", err)
	}
	waitFor(t, exit)
	os.Exit(0)
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

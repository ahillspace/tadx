package lock_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

func openRoot(t *testing.T) (*os.Root, string) {
	t.Helper()
	directory := t.TempDir()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = root.Close() })
	return root, directory
}

func TestTryAcquireInFailsWhileHeldAndRemovesTheFileOnRelease(t *testing.T) {
	root, directory := openRoot(t)
	held, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("TryAcquireIn() error = %v", err)
	}
	if _, err := lock.TryAcquireIn(root, ".tadx-install.lock"); !errors.Is(err, lock.ErrLocked) {
		t.Fatalf("TryAcquireIn() while held error = %v, want ErrLocked", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(directory, ".tadx-install.lock")); !os.IsNotExist(err) {
		t.Fatalf("released lock file remains: %v", err)
	}
	regained, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("TryAcquireIn() after release error = %v", err)
	}
	if err := regained.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func TestTryAcquireInReusesALockFileNoProcessHolds(t *testing.T) {
	root, directory := openRoot(t)
	// An interrupted holder leaves its lock file behind without a lock.
	location := filepath.Join(directory, ".tadx-install.lock")
	if err := os.WriteFile(location, []byte("left by an interrupted install"), 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("TryAcquireIn() over an unheld lock file error = %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := os.Lstat(location); !os.IsNotExist(err) {
		t.Fatalf("released lock file remains: %v", err)
	}
}

func TestTryAcquireInRejectsALinkedLockFile(t *testing.T) {
	root, directory := openRoot(t)
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(directory, ".tadx-install.lock")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if held, err := lock.TryAcquireIn(root, ".tadx-install.lock"); err == nil {
		_ = held.Release()
		t.Fatal("TryAcquireIn() locked through a symlink")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("link target changed: %v", err)
	}
}

// TestTryAcquireInReleasesWhenTheHolderIsKilled proves a holder that never
// runs Release, as after a kill, does not block the next caller.
func TestTryAcquireInReleasesWhenTheHolderIsKilled(t *testing.T) {
	if os.Getenv("TADX_LOCK_ROOT_CHILD") == "1" {
		holdRootChild(t)
		return
	}
	root, directory := openRoot(t)
	ready := filepath.Join(t.TempDir(), "ready")
	cmd := exec.Command(os.Args[0], "-test.run", "TestTryAcquireInReleasesWhenTheHolderIsKilled")
	cmd.Env = append(os.Environ(), "TADX_LOCK_ROOT_CHILD=1", "TADX_LOCK_ROOT="+directory, "TADX_LOCK_READY="+ready)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start child: %v", err)
	}
	waitFor(t, ready)
	if _, err := lock.TryAcquireIn(root, ".tadx-install.lock"); !errors.Is(err, lock.ErrLocked) {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("TryAcquireIn() while child holds error = %v, want ErrLocked", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill child: %v", err)
	}
	_ = cmd.Wait()
	if _, err := os.Lstat(filepath.Join(directory, ".tadx-install.lock")); err != nil {
		t.Fatalf("killed holder left no lock file to reuse: %v", err)
	}
	held, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("TryAcquireIn() after the holder was killed error = %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func holdRootChild(t *testing.T) {
	root, err := os.OpenRoot(os.Getenv("TADX_LOCK_ROOT"))
	if err != nil {
		t.Fatalf("child OpenRoot() error = %v", err)
	}
	held, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("child TryAcquireIn() error = %v", err)
	}
	if err := os.WriteFile(os.Getenv("TADX_LOCK_READY"), nil, 0o600); err != nil {
		t.Fatalf("child signal ready: %v", err)
	}
	// Hold the lock until the parent kills this process; Release never runs.
	time.Sleep(time.Minute)
	runtime.KeepAlive(held)
}

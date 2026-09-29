package lock

import (
	"errors"
	"os"
	"testing"
)

func TestLockNamedRejectsAFileWhoseNameWasRemovedBeforeTheLock(t *testing.T) {
	root, err := os.OpenRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	// A caller opened the lock file, then its holder removed the name and
	// released before the caller got the lock.
	stale, err := root.OpenFile(".tadx-install.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := root.Remove(".tadx-install.lock"); err != nil {
		t.Fatal(err)
	}
	if handle, err := lockNamed(root, ".tadx-install.lock", stale); !errors.Is(err, errReplaced) {
		_ = handle.Release()
		t.Fatalf("lockNamed() on a removed name error = %v, want errReplaced", err)
	}
	held, err := TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		t.Fatalf("TryAcquireIn() after the replacement error = %v", err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
}

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/lock"
)

// holdPackageLock holds the skill installation lock in directory as another
// running installer would, until the test ends.
func holdPackageLock(t *testing.T, directory string) {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	held, err := lock.TryAcquireIn(root, ".tadx-install.lock")
	if err != nil {
		_ = root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = held.Release()
		_ = root.Close()
	})
}

func TestLockFileLeftByAnInterruptedInstallDoesNotBlockLaterOperations(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			in := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := in.Install(context.Background(), "codex", false, false); err != nil {
				t.Fatal(err)
			}
			// A killed installer never removes its lock file.
			location := filepath.Join(home, ".codex", "skills", ".tadx-install.lock")
			if err := os.WriteFile(location, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := runOperation(in, operation, false, false); err != nil {
				t.Fatalf("%s blocked by an unheld lock file: %v", operation, err)
			}
			if _, err := os.Lstat(location); !os.IsNotExist(err) {
				t.Fatalf("%s left the lock file behind: %v", operation, err)
			}
		})
	}
}

func TestHeldPackageLockBlocksBothOperations(t *testing.T) {
	for _, operation := range []string{"install", "uninstall"} {
		t.Run(operation, func(t *testing.T) {
			home := t.TempDir()
			in := Installer{Home: func() (string, error) { return home, nil }}
			if _, err := in.Install(context.Background(), "codex", false, false); err != nil {
				t.Fatal(err)
			}
			holdPackageLock(t, filepath.Join(home, ".codex", "skills"))
			if _, err := runOperation(in, operation, false, false); err == nil {
				t.Fatalf("%s ignored a held lock", operation)
			}
		})
	}
}

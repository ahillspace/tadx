package auth

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/lock"
)

const epochTestKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func acquireEpochOwner(ctx context.Context, directory string) (*patSignIn, error) {
	held, err := lock.AcquireContext(ctx, filepath.Join(directory, epochTestKey+".lock"))
	if err != nil {
		return nil, err
	}
	return &patSignIn{handle: held, directory: directory, identity: epochTestKey}, nil
}

// Epoch readers and writers share the same sign-in ownership as production
// authentication. Windows replacement does not support lock-free readers.
func TestSignInEpochReadersUseSignInOwnership(t *testing.T) {
	directory := t.TempDir()
	owner, err := acquireEpochOwner(t.Context(), directory)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.writeEpoch(1); err != nil {
		_ = owner.Release()
		t.Fatal(err)
	}
	_ = owner.Release()
	done := make(chan struct{})
	var readers sync.WaitGroup
	stopReaders := sync.OnceFunc(func() { close(done); readers.Wait() })
	defer stopReaders()
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				owner, err := acquireEpochOwner(t.Context(), directory)
				if err != nil {
					t.Error(err)
					return
				}
				epoch, err := owner.readEpoch()
				_ = owner.Release()
				if err != nil || epoch == 0 {
					t.Errorf("readEpoch() = %d, %v; want a complete epoch", epoch, err)
					return
				}
			}
		})
	}
	for epoch := uint64(2); epoch < 200; epoch++ {
		owner, err := acquireEpochOwner(t.Context(), directory)
		if err != nil {
			t.Fatal(err)
		}
		err = owner.writeEpoch(epoch)
		_ = owner.Release()
		if err != nil {
			t.Fatal(err)
		}
	}
	stopReaders()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Fatalf("staged epoch file left behind: %s", entry.Name())
		}
	}
}

func TestCoordinationSnapshotsRequireLiveSignInOwnership(t *testing.T) {
	owner, err := acquireEpochOwner(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Release(); err != nil {
		t.Fatal(err)
	}
	for _, released := range []*patSignIn{nil, owner} {
		if _, err := released.readEpoch(); err == nil {
			t.Fatal("epoch read accepted missing sign-in ownership")
		}
		if err := released.writeEpoch(1); err == nil {
			t.Fatal("epoch write accepted missing sign-in ownership")
		}
		if _, err := released.readUseKey(); err == nil {
			t.Fatal("sharing-key read accepted missing sign-in ownership")
		}
		if err := released.writeUseKey("fixture-sharing-key"); err == nil {
			t.Fatal("sharing-key write accepted missing sign-in ownership")
		}
	}
}

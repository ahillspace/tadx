package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/lock"
)

func TestMutationRejectsManifestReplacedWhileWaitingForArtifactOwner(t *testing.T) {
	root := t.TempDir()
	manifest := filepath.Join(root, "tadx.yaml")
	if err := os.WriteFile(manifest, []byte("version: 1\nworkspace: {id: original, name: original}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	held, err := lock.Acquire(filepath.Join(root, ".tadx.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer held.Release()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		mutation, err := BeginMutation(ctx, root)
		if mutation != nil {
			_ = mutation.Release()
		}
		done <- err
	}()
	path := lifecycleLockPath(root)
	// Observe the real shared lifecycle lease, proving the mutation captured its
	// root identity and reached the artifact-lock wait before the replacement.
	for {
		probe, err := lock.TryAcquire(path)
		if errors.Is(err, lock.ErrLocked) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		_ = probe.Release()
		select {
		case <-ctx.Done():
			t.Fatal("mutation never acquired its lifecycle lease")
		case <-time.After(5 * time.Millisecond):
		}
	}
	if err := os.WriteFile(manifest, []byte("version: 1\nworkspace: {id: replacement, name: replacement}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := held.Release(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err == nil || !strings.Contains(err.Error(), "manifest identity changed") {
		t.Fatalf("waiting mutation accepted a different workspace identity: %v", err)
	}
}

func TestLifecycleOwnershipIndependentOfUserCacheEnvironment(t *testing.T) {
	root := t.TempDir()
	use, err := beginWorkspaceUse(t.Context(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	defer use.release()
	t.Setenv("LOCALAPPDATA", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	second, err := beginWorkspaceUse(t.Context(), root, true)
	if second != nil {
		_ = second.release()
	}
	if !errors.Is(err, ErrWorkspaceInUse) {
		t.Fatalf("cache environment split lifecycle ownership: %v", err)
	}
}

func TestCloneRetainsSourceLifecycleOwnershipThroughRegistration(t *testing.T) {
	base := t.TempDir()
	manager := NewManager(filepath.Join(base, "config.yaml"), nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(base, "source"))
	if err != nil {
		t.Fatal(err)
	}
	checked := false
	manager.update = func(path string, create bool, mutate func(config.Config) (config.Config, error)) (config.Config, error) {
		checked = true
		mutation, downloads, err := acquireDeletion(t.Context(), source.Root)
		if mutation != nil {
			_ = downloads.Release()
			_ = mutation.Release()
		}
		if !errors.Is(err, ErrWorkspaceInUse) {
			t.Fatalf("source deletion entered while clone was active: %v", err)
		}
		return config.Update(path, create, mutate)
	}
	if _, err := manager.Clone(t.Context(), "source", "clone", filepath.Join(base, "clone")); err != nil {
		t.Fatal(err)
	}
	if !checked {
		t.Fatal("clone did not register its copied workspace")
	}
}

package workspace_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/lock"
	"github.com/ahillspace/tadx/internal/workspace"
)

func deletionFixture(t *testing.T) (*workspace.Manager, workspace.Record) {
	t.Helper()
	base := t.TempDir()
	manager := workspace.NewManager(filepath.Join(base, "config.yaml"), nil)
	if _, err := manager.Create(t.Context(), "default", filepath.Join(base, "default")); err != nil {
		t.Fatal(err)
	}
	target, err := manager.Create(t.Context(), "target", filepath.Join(base, "target"))
	if err != nil {
		t.Fatal(err)
	}
	return manager, target
}

func TestDeleteRefusesActiveArtifactAndDownloadOwners(t *testing.T) {
	for _, owner := range []string{"artifact", "download", "legacy artifact", "legacy download"} {
		t.Run(owner, func(t *testing.T) {
			manager, target := deletionFixture(t)
			var staged string
			switch owner {
			case "artifact":
				held, err := workspace.BeginMutation(t.Context(), target.Root)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Release()
			case "download":
				downloads := workspace.NewDownloads(target.Root)
				defer downloads.Close()
				var err error
				staged, err = downloads.File(t.Context(), func(w io.Writer) error { _, err := io.WriteString(w, "active download"); return err })
				if err != nil {
					t.Fatal(err)
				}
			case "legacy artifact", "legacy download":
				path := filepath.Join(target.Root, ".tadx.lock")
				if owner == "legacy download" {
					path = filepath.Join(target.Root, ".tadx", "tmp.lock")
				}
				held, err := lock.Acquire(path)
				if err != nil {
					t.Fatal(err)
				}
				defer held.Release()
			}
			if _, err := manager.Delete(t.Context(), workspace.DeleteRequest{Expected: target}); !errors.Is(err, workspace.ErrWorkspaceInUse) {
				t.Fatalf("delete error = %v, want workspace in use", err)
			}
			if _, err := manager.Resolve(t.Context(), target.Name, ""); err != nil {
				t.Fatalf("active workspace was unregistered: %v", err)
			}
			if staged != "" {
				if data, err := os.ReadFile(staged); err != nil || string(data) != "active download" {
					t.Fatalf("active download changed: %q, %v", data, err)
				}
			}
		})
	}
}

func TestDeleteRevalidatesUnderExclusiveOwnershipAndClosesInteriorLocks(t *testing.T) {
	manager, target := deletionFixture(t)
	checked := false
	_, err := manager.Delete(t.Context(), workspace.DeleteRequest{Expected: target, Revalidate: func(ctx context.Context, mutation *workspace.Mutation) error {
		checked = true
		if err := mutation.Validate(); err != nil {
			t.Fatal(err)
		}
		waiting, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
		defer cancel()
		if acquired, err := workspace.BeginMutation(waiting, target.Root); !errors.Is(err, context.DeadlineExceeded) {
			if acquired != nil {
				_ = acquired.Release()
			}
			t.Fatalf("new mutation entered during deletion: %v", err)
		}
		return nil
	}})
	if err != nil || !checked {
		t.Fatalf("delete with native root rename = %v, checked = %v", err, checked)
	}
	if _, err := os.Stat(filepath.Join(target.Root, config.WorkspaceConfigName)); !os.IsNotExist(err) {
		t.Fatalf("deleted root remains: %v", err)
	}
}

func TestDeletePreservesWorkspaceWhenFreshContentCheckRejects(t *testing.T) {
	manager, target := deletionFixture(t)
	local := filepath.Join(target.Root, "local.txt")
	if err := os.WriteFile(local, []byte("new local work"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirty := errors.New("workspace became dirty")
	_, err := manager.Delete(t.Context(), workspace.DeleteRequest{Expected: target, Revalidate: func(context.Context, *workspace.Mutation) error { return dirty }})
	if !errors.Is(err, dirty) {
		t.Fatalf("delete error = %v", err)
	}
	if data, err := os.ReadFile(local); err != nil || string(data) != "new local work" {
		t.Fatalf("local edits lost: %q, %v", data, err)
	}
	if _, err := manager.Resolve(t.Context(), target.Name, ""); err != nil {
		t.Fatalf("dirty workspace unregistered: %v", err)
	}
}

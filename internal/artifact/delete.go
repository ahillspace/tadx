package artifact

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/pathspec"
)

// DeleteRequest removes one exact revalidated managed artifact.
type DeleteRequest struct {
	Workspace string
	Expected  Item
}

type deleteOperations struct {
	rename    func(string, string) error
	removeAll func(string) error
}

func defaultDeleteOperations() deleteOperations {
	return deleteOperations{rename: os.Rename, removeAll: os.RemoveAll}
}

// Delete revalidates and removes one exact managed artifact.
func Delete(ctx context.Context, request DeleteRequest) (Item, error) {
	return deleteWithOperations(ctx, request, defaultDeleteOperations())
}

func deleteWithOperations(ctx context.Context, request DeleteRequest, operations deleteOperations) (Item, error) {
	if operations.rename == nil {
		operations.rename = os.Rename
	}
	if operations.removeAll == nil {
		operations.removeAll = os.RemoveAll
	}
	if request.Expected.Path == "" || request.Expected.Kind == "" || request.Expected.LUID == "" {
		return Item{}, errors.New("artifact delete requires an exact planned identity")
	}
	root, err := validateWorkspaceRoot(request.Workspace)
	if err != nil {
		return Item{}, err
	}
	handle, err := lockWorkspace(root)
	if err != nil {
		return Item{}, err
	}
	defer func() { _ = handle.Release() }()
	current, err := Resolve(ctx, root, Selector{Path: request.Expected.Path, Kind: request.Expected.Kind, LUID: request.Expected.LUID})
	if err != nil {
		return Item{}, err
	}
	if !sameArtifactSnapshot(current, request.Expected) {
		return Item{}, errors.New("artifact changed after deletion was planned")
	}
	// Defense in depth: Resolve only ever yields a validated, within-root
	// artifacts/<kind>/<component> path, but this is a destructive mutation
	// boundary, so assert non-escape locally before joining onto the root.
	if pathspec.Escapes(current.Path) {
		return Item{}, fmt.Errorf("artifact path escapes workspace root: %q", current.Path)
	}
	target := filepath.Join(root, filepath.FromSlash(current.Path))
	parent := filepath.Dir(target)
	tombstone, err := os.MkdirTemp(parent, ".tadx-delete-")
	if err != nil {
		return Item{}, err
	}
	// Until the artifact is renamed into the tombstone, the reserved temp
	// directory is transient scaffolding; clean it up on any early error path so
	// it cannot leak. Once staged, the artifact lives there and the explicit
	// restore/removeAll logic below owns its lifecycle, so leave it untouched.
	tombstoneStaged := false
	defer func() {
		if !tombstoneStaged {
			_ = operations.removeAll(tombstone)
		}
	}()
	if err := os.Remove(tombstone); err != nil {
		return Item{}, err
	}
	if err := operations.rename(target, tombstone); err != nil {
		return Item{}, fmt.Errorf("stage artifact deletion: %w", err)
	}
	tombstoneStaged = true
	seized, err := inspectArtifact(ctx, root, current.Kind, tombstone)
	if err != nil || !sameArtifactSnapshot(seized, request.Expected) {
		if restoreErr := operations.rename(tombstone, target); restoreErr != nil {
			return Item{}, fmt.Errorf("artifact changed while deletion was staged and restoration failed: %v", restoreErr)
		}
		return Item{}, errors.New("artifact changed while deletion was staged")
	}
	if err := operations.removeAll(tombstone); err != nil {
		current.Warnings = append(current.Warnings, "The artifact was deleted, but transaction cleanup is incomplete.")
	}
	return current, nil
}

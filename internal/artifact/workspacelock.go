package artifact

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// lockWorkspace waits for the advisory workspace lock for a single root until
// ctx ends. Callers must Release the returned handle when the mutation
// completes.
func lockWorkspace(ctx context.Context, workspace string, operations directoryOperations) (*workspacecore.Mutation, error) {
	handle, err := workspacecore.BeginMutation(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("acquire workspace lock: %w", err)
	}
	if err := recoverBundleTransaction(handle.Root(), operations.withDefaults()); err != nil {
		_ = handle.Release()
		return nil, err
	}
	return handle, nil
}

// RecoverWorkspace reconciles shared artifact transactions while the caller
// holds a live workspace mutation token, including exclusive root deletion.
func RecoverWorkspace(ctx context.Context, mutation *workspacecore.Mutation) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := mutation.Validate(); err != nil {
		return err
	}
	return recoverBundleTransaction(mutation.Root(), defaultDirectoryOperations())
}

func requireWorkspaceIdentity(root, expectedID string) error {
	if expectedID == "" {
		return nil
	}
	manifest, err := workspacecore.ReadManifest(root)
	if err != nil {
		return err
	}
	if manifest.Workspace.ID != expectedID {
		return errors.New("workspace identity changed after selection")
	}
	return nil
}

// lockWorkspaces acquires the advisory lock for one or more workspace roots in a
// deterministic (sorted) order so concurrent cross-workspace moves cannot
// deadlock. Duplicate roots are locked once. The returned function releases
// every acquired handle in reverse order and must always be called.
func lockWorkspaces(ctx context.Context, workspaces ...string) ([]string, func(), error) {
	paths := make(map[string]string, len(workspaces))
	resolved := make([]string, len(workspaces))
	for index, workspace := range workspaces {
		canonical, err := workspacecore.CanonicalRoot(workspace)
		if err != nil {
			return nil, nil, err
		}
		key := filepath.Clean(canonical)
		if runtime.GOOS == "windows" {
			key = strings.ToLower(key)
		}
		paths[key] = canonical
		resolved[index] = key
	}
	keys := slices.Sorted(maps.Keys(paths))

	handles := make([]*workspacecore.Mutation, 0, len(keys))
	release := func() {
		for index := len(handles) - 1; index >= 0; index-- {
			_ = handles[index].Release()
		}
	}
	for _, key := range keys {
		handle, err := lockWorkspace(ctx, paths[key], defaultDirectoryOperations())
		if err != nil {
			release()
			return nil, nil, err
		}
		handles = append(handles, handle)
		paths[key] = handle.Root()
	}
	for index, key := range resolved {
		resolved[index] = paths[key]
	}
	return resolved, release, nil
}

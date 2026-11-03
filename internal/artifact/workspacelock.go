package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
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

// PrepareWorkspaceSnapshot reconciles supported replacement protocols and
// rejects unresolved recovery evidence before a source tree can be copied.
// It preserves evidence for resource types without a standalone recovery owner.
func PrepareWorkspaceSnapshot(ctx context.Context, mutation *workspacecore.Mutation) error {
	if err := RecoverWorkspace(ctx, mutation); err != nil {
		return err
	}
	operations := defaultDirectoryOperations()
	artifacts := filepath.Join(mutation.Root(), "artifacts")
	if info, err := os.Lstat(artifacts); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	} else if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return errors.New("workspace artifacts root must be a real directory")
	}
	for _, resource := range []struct {
		kind    string
		recover func(string, directoryOperations) ([]string, error)
	}{{"workbook", recoverWorkbookRoot}, {"datasource", recoverDatasourceRoot}} {
		if err := ctx.Err(); err != nil {
			return err
		}
		root := filepath.Join(artifacts, resource.kind)
		info, err := os.Lstat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return fmt.Errorf("%s artifact root must be a real directory", resource.kind)
		}
		if _, err := resource.recover(root, operations); err != nil {
			return err
		}
	}
	remaining := 100000
	directories := []string{artifacts}
	for index := 0; index < len(directories); index++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		directory := directories[index]
		info, err := os.Lstat(directory)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errors.New("workspace artifact snapshot root must be a real directory")
		}
		file, err := os.Open(directory)
		if err != nil {
			return err
		}
		entries, readErr := file.ReadDir(remaining + 1)
		closeErr := file.Close()
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(entries) > remaining {
			return errors.New("workspace artifact snapshot exceeds its recovery entry limit")
		}
		remaining -= len(entries)
		slices.SortFunc(entries, func(left, right os.DirEntry) int { return strings.Compare(left.Name(), right.Name()) })
		for _, entry := range entries {
			name := entry.Name()
			if isArtifactRecoveryEntry(name) {
				relative, _ := filepath.Rel(mutation.Root(), filepath.Join(directory, name))
				action := "Preserve the source workspace recovery entry and complete or repair the interrupted artifact replacement before cloning."
				if strings.HasPrefix(name, moveSourcePrefix) || strings.HasPrefix(name, moveStagePrefix) {
					action = "Preserve the move recovery entry and inspect the source and destination workspaces before recovery; do not restore or remove it based on one workspace alone."
				}
				return &errs.Error{Kind: errs.KindOperation, Summary: "Workspace snapshot has unresolved artifact recovery evidence: " + filepath.ToSlash(relative), Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false), CorrectiveAction: action}
			}
			if directory == artifacts && (slices.Contains(directManagedArtifactKinds[:], name) || name == "lineage") || filepath.Base(directory) == "lineage" && isLineageResourceKind(name) {
				directories = append(directories, filepath.Join(directory, name))
			}
		}
	}
	return nil
}

func isArtifactRecoveryEntry(name string) bool {
	for _, prefix := range []string{backupPrefix, stagePrefix, datasourceBackupPrefix, datasourceStagePrefix, moveSourcePrefix, moveStagePrefix, ".tadx-flow-stage-", ".tadx-pulse-definition-stage-", ".tadx-lineage-stage-"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
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

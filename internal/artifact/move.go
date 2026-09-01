package artifact

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// MoveRequest moves one exact managed artifact between named workspace roots.
type MoveRequest struct {
	SourceWorkspace      string
	DestinationWorkspace string
	Selector             Selector
}

type moveOperations struct {
	rename    func(string, string) error
	removeAll func(string) error
}

func defaultMoveOperations() moveOperations {
	return moveOperations{rename: os.Rename, removeAll: os.RemoveAll}
}

// Move preserves one managed artifact's bytes and identity.
func Move(ctx context.Context, request MoveRequest) (Item, error) {
	return moveWithOperations(ctx, request, defaultMoveOperations())
}

func moveWithOperations(ctx context.Context, request MoveRequest, operations moveOperations) (Item, error) {
	if operations.rename == nil {
		operations.rename = os.Rename
	}
	if operations.removeAll == nil {
		operations.removeAll = os.RemoveAll
	}
	sourceRoot, err := validateWorkspaceRoot(request.SourceWorkspace)
	if err != nil {
		return Item{}, fmt.Errorf("source workspace: %w", err)
	}
	destinationRoot, err := validateWorkspaceRoot(request.DestinationWorkspace)
	if err != nil {
		return Item{}, fmt.Errorf("destination workspace: %w", err)
	}
	if sameFilesystemPath(sourceRoot, destinationRoot) {
		return Item{}, errors.New("source and destination workspaces must differ")
	}
	release, err := lockWorkspaces(sourceRoot, destinationRoot)
	if err != nil {
		return Item{}, err
	}
	defer release()
	source, err := Resolve(ctx, sourceRoot, request.Selector)
	if err != nil {
		return Item{}, err
	}
	destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(source.Path))
	if _, err := os.Lstat(destinationPath); err == nil {
		return Item{}, errors.New("destination artifact already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Item{}, err
	}
	destinationParent := filepath.Dir(destinationPath)
	if err := os.MkdirAll(destinationParent, 0o700); err != nil {
		return Item{}, err
	}
	stage, err := os.MkdirTemp(destinationParent, ".tadx-move-stage-")
	if err != nil {
		return Item{}, err
	}
	defer os.RemoveAll(stage)
	if err := copyManagedDirectory(ctx, filepath.Join(sourceRoot, filepath.FromSlash(source.Path)), stage); err != nil {
		return Item{}, err
	}
	staged, err := inspectArtifact(ctx, destinationRoot, source.Kind, stage)
	if err != nil {
		return Item{}, fmt.Errorf("verify staged artifact: %w", err)
	}
	if !sameArtifactSnapshot(staged, source) {
		return Item{}, errors.New("staged artifact identity or fingerprint changed")
	}
	currentSource, err := Resolve(ctx, sourceRoot, Selector{Path: source.Path, Kind: source.Kind, LUID: source.LUID})
	if err != nil || !sameArtifactSnapshot(currentSource, source) {
		return Item{}, errors.New("source artifact changed while the move was staged")
	}
	sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(source.Path))
	tombstone, err := os.MkdirTemp(filepath.Dir(sourcePath), ".tadx-move-source-")
	if err != nil {
		return Item{}, err
	}
	if err := os.Remove(tombstone); err != nil {
		return Item{}, err
	}
	if err := operations.rename(sourcePath, tombstone); err != nil {
		return Item{}, fmt.Errorf("stage source artifact removal: %w", err)
	}
	seized, err := inspectArtifact(ctx, sourceRoot, source.Kind, tombstone)
	if err != nil || !sameArtifactSnapshot(seized, source) || !sameArtifactSnapshot(seized, staged) {
		if restoreErr := operations.rename(tombstone, sourcePath); restoreErr != nil {
			return Item{}, fmt.Errorf("source artifact changed at move commit and restoration failed: %v", restoreErr)
		}
		return Item{}, errors.New("source artifact changed at move commit")
	}
	if err := operations.rename(stage, destinationPath); err != nil {
		if restoreErr := operations.rename(tombstone, sourcePath); restoreErr != nil {
			return Item{}, fmt.Errorf("install destination artifact: %w; source remains in a recovery tombstone because restoration failed: %v", err, restoreErr)
		}
		return Item{}, fmt.Errorf("install destination artifact: %w", err)
	}
	moved, err := inspectArtifact(ctx, destinationRoot, source.Kind, destinationPath)
	if err != nil {
		rollbackErr := operations.rename(destinationPath, stage)
		restoreErr := operations.rename(tombstone, sourcePath)
		if rollbackErr != nil || restoreErr != nil {
			return Item{}, fmt.Errorf("verify installed destination artifact: %w; rollback failed: destination=%v source=%v", err, rollbackErr, restoreErr)
		}
		return Item{}, fmt.Errorf("verify installed destination artifact: %w", err)
	}
	if err := operations.removeAll(tombstone); err != nil {
		moved.Warnings = append(moved.Warnings, "The artifact moved successfully, but source transaction cleanup is incomplete.")
	}
	return moved, nil
}

func copyManagedDirectory(ctx context.Context, source, destination string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		from := filepath.Join(source, entry.Name())
		to := filepath.Join(destination, entry.Name())
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("artifact entry %q must not be a symbolic link", entry.Name())
		}
		if info.IsDir() {
			if err := os.Mkdir(to, 0o700); err != nil {
				return err
			}
			if err := copyManagedDirectory(ctx, from, to); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("artifact entry %q must be a regular file or directory", entry.Name())
		}
		if err := copyManagedFile(ctx, from, to, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

func copyManagedFile(ctx context.Context, source, destination string, mode os.FileMode) error {
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, &contextReader{ctx: ctx, reader: input})
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func sameFilesystemPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

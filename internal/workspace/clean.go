package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/ahillspace/tadx/internal/lock"
)

// CleanResult is a bounded receipt for disposable workspace state removal.
type CleanResult struct {
	EntriesRemoved int
	BytesRemoved   int64
	Removed        []string
}

var cleanClasses = map[string][]string{
	"temporary": {"tmp", "staging"},
	"cache":     {"cache"},
	"logs":      {"logs"},
	"all":       {"tmp", "staging", "cache", "logs"},
}

// downloadsInUseError reports temporary state that a running command is
// still downloading into.
type downloadsInUseError struct{}

func (downloadsInUseError) Error() string {
	return "workspace temporary state is in use by a running download"
}
func (downloadsInUseError) Retryable() bool { return true }
func (downloadsInUseError) CorrectiveAction() string {
	return "Retry the cleanup after the running pull finishes."
}

var errDownloadsInUse error = downloadsInUseError{}

// Clean removes only admitted children of the workspace control directory.
func Clean(ctx context.Context, root, class string) (CleanResult, error) {
	return clean(ctx, root, class, false)
}

// PreviewClean measures the exact selected disposable state without removing it.
func PreviewClean(ctx context.Context, root, class string) (CleanResult, error) {
	return clean(ctx, root, class, true)
}

func clean(ctx context.Context, root, class string, preview bool) (CleanResult, error) {
	children, ok := cleanClasses[class]
	if !ok {
		return CleanResult{}, fmt.Errorf("unsupported workspace cleanup class %q", class)
	}
	control, err := controlDirectory(root, false)
	if errors.Is(err, fs.ErrNotExist) {
		return CleanResult{}, nil
	}
	if err != nil {
		return CleanResult{}, err
	}
	if !preview && slices.Contains(children, "tmp") {
		held, err := lock.TryAcquire(filepath.Join(control, downloadLockName))
		if errors.Is(err, lock.ErrLocked) {
			return CleanResult{}, errDownloadsInUse
		}
		if err != nil {
			return CleanResult{}, fmt.Errorf("lock workspace download state: %w", err)
		}
		defer held.Release()
	}

	result := CleanResult{}
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return CleanResult{}, err
		}
		target := filepath.Join(control, child)
		entries, bytes, exists, err := measureDisposable(ctx, target)
		if err != nil {
			return CleanResult{}, err
		}
		if !exists {
			continue
		}
		if !preview {
			if err := os.RemoveAll(target); err != nil {
				return CleanResult{}, fmt.Errorf("remove disposable workspace state %q: %w", child, err)
			}
		}
		result.EntriesRemoved += entries
		result.BytesRemoved += bytes
		result.Removed = append(result.Removed, filepath.ToSlash(filepath.Join(".tadx", child)))
	}
	sort.Strings(result.Removed)
	return result, nil
}

func measureDisposable(ctx context.Context, target string) (entries int, bytes int64, exists bool, err error) {
	if _, err := os.Lstat(target); os.IsNotExist(err) {
		return 0, 0, false, nil
	} else if err != nil {
		return 0, 0, false, fmt.Errorf("inspect disposable workspace state: %w", err)
	}
	err = filepath.WalkDir(target, func(_ string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entry.Type().IsRegular() {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			bytes += info.Size()
		}
		return nil
	})
	if err != nil {
		return 0, 0, true, fmt.Errorf("measure disposable workspace state: %w", err)
	}
	return entries, bytes, true, nil
}

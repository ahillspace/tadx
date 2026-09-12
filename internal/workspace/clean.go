package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
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
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return CleanResult{}, fmt.Errorf("resolve workspace root: %w", err)
	}
	control := filepath.Join(absoluteRoot, ".tadx")
	if !containedPath(absoluteRoot, control) {
		return CleanResult{}, errors.New("workspace control directory escapes workspace root")
	}
	info, err := os.Lstat(control)
	if os.IsNotExist(err) {
		return CleanResult{}, nil
	}
	if err != nil {
		return CleanResult{}, fmt.Errorf("inspect workspace control directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return CleanResult{}, errors.New("workspace control path must be a real directory")
	}

	result := CleanResult{}
	for _, child := range children {
		if err := ctx.Err(); err != nil {
			return CleanResult{}, err
		}
		target := filepath.Join(control, child)
		if !containedPath(control, target) {
			return CleanResult{}, errors.New("workspace cleanup target escapes control directory")
		}
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

func containedPath(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

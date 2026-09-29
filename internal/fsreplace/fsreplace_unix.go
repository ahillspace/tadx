//go:build unix

package fsreplace

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// Rename renames from to to. POSIX rename replaces an existing destination
// atomically even while other descriptors keep it open, so it never retries.
func Rename(from, to string) error {
	return os.Rename(from, to)
}

// SyncDir flushes a directory's own metadata (its entry list) to stable
// storage so that renames and creations within it survive power loss.
// Filesystems that cannot sync a directory report EINVAL or ENOTSUP; those
// provide no stronger guarantee to wait for, so SyncDir accepts them.
func SyncDir(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for fsync %q: %w", path, err)
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	if syncErr != nil && !errors.Is(syncErr, syscall.EINVAL) && !errors.Is(syncErr, syscall.ENOTSUP) {
		return fmt.Errorf("fsync directory %q: %w", path, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory after fsync %q: %w", path, closeErr)
	}
	return nil
}

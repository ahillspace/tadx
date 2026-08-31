//go:build unix

package artifact

import (
	"fmt"
	"os"
)

// fsyncDir flushes a directory's own metadata (its entry list) to stable
// storage so that renames and creations within it survive power loss.
func fsyncDir(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open directory for fsync %q: %w", path, err)
	}
	syncErr := dir.Sync()
	closeErr := dir.Close()
	if syncErr != nil {
		return fmt.Errorf("fsync directory %q: %w", path, syncErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close directory after fsync %q: %w", path, closeErr)
	}
	return nil
}

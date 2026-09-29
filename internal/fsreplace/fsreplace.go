// Package fsreplace installs staged local files and directories by rename and
// makes the rename durable. It is a standard-library leaf shared by every
// writer that replaces local state, so the Windows sharing retry and the
// directory sync have one owner.
package fsreplace

import (
	"fmt"
	"path/filepath"
)

// syncDir is replaceable so tests can observe the parent directory sync.
var syncDir = SyncDir

// Replace renames from to to, replacing an existing file, and then flushes the
// parent directory of to so the new entry survives power loss. A failed rename
// leaves both paths unchanged and skips the sync. A sync failure is reported
// after the rename has already taken effect.
func Replace(from, to string) error {
	if err := Rename(from, to); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(to)); err != nil {
		return fmt.Errorf("sync %q after rename: %w", filepath.Dir(to), err)
	}
	return nil
}

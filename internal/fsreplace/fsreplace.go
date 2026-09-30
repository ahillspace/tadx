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

// DurabilityError reports that Replace installed the destination but could not
// flush its parent directory. The replacement is visible now but may not
// survive power loss, so callers must not treat it as an unchanged destination.
type DurabilityError struct {
	Dir string
	Err error
}

func (e *DurabilityError) Error() string {
	return fmt.Sprintf("sync %q after rename: %v", e.Dir, e.Err)
}

func (e *DurabilityError) Unwrap() error { return e.Err }

// Replace renames from to to, replacing an existing file, and then flushes the
// parent directory of to so the new entry survives power loss. A failed rename
// leaves both paths unchanged and skips the sync. A sync failure is reported as
// a *DurabilityError after the rename has already taken effect.
func Replace(from, to string) error {
	if err := Rename(from, to); err != nil {
		return err
	}
	if err := syncDir(filepath.Dir(to)); err != nil {
		return &DurabilityError{Dir: filepath.Dir(to), Err: err}
	}
	return nil
}

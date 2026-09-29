//go:build windows

package fsreplace

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// retryLimit bounds how long a rename waits for other handles to close.
const retryLimit = 2 * time.Second

// Rename renames from to to.
//
// os.Rename uses MoveFileEx with MOVEFILE_REPLACE_EXISTING, which fails with
// ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION while any other handle to the
// destination file, or to a file inside a directory being renamed, is open.
// Concurrent TADX processes read local state outside the writer's lock, and
// antivirus and indexing services open newly written files briefly. Callers
// hold the lock that serializes their writers, and a failed rename leaves both
// paths unchanged, so retrying cannot reorder or duplicate updates. A handle
// that outlasts the bound still fails the rename.
func Rename(from, to string) error {
	deadline := time.Now().Add(retryLimit)
	delay := time.Millisecond
	for {
		err := os.Rename(from, to)
		transient := errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
		if !transient || time.Now().Add(delay).After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(2*delay, 100*time.Millisecond)
	}
}

// SyncDir is a no-op on Windows. NTFS orders directory metadata changes
// (creations and renames) through its journal, and the Win32 FlushFileBuffers
// call is not supported on directory handles opened by the standard library.
// Writers still flush each staged file before renaming it into place.
func SyncDir(string) error { return nil }

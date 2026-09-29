//go:build windows

package operationrun

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// replaceRetryLimit bounds how long an update waits, while holding the
// record's exclusive lock, for other handles to the record to close.
const replaceRetryLimit = 2 * time.Second

// replaceRecord renames temporary over path.
//
// os.Rename uses MoveFileEx with MOVEFILE_REPLACE_EXISTING, which fails with
// ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION while any other handle to the
// destination is open. Record lookups stat the file outside the record lock,
// possibly from another TADX process, and file scanners open new files
// briefly. The caller holds the exclusive update lock and a failed rename
// leaves both files unchanged, so retrying cannot reorder or duplicate
// updates. A handle that outlasts the bound still fails the update.
func replaceRecord(temporary, path string) error {
	deadline := time.Now().Add(replaceRetryLimit)
	delay := time.Millisecond
	for {
		err := os.Rename(temporary, path)
		transient := errors.Is(err, windows.ERROR_ACCESS_DENIED) || errors.Is(err, windows.ERROR_SHARING_VIOLATION)
		if !transient || time.Now().Add(delay).After(deadline) {
			return err
		}
		time.Sleep(delay)
		delay = min(2*delay, 100*time.Millisecond)
	}
}

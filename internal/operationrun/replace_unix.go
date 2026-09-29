//go:build unix

package operationrun

import "os"

// replaceRecord renames temporary over path. POSIX rename replaces the
// destination atomically even while other descriptors keep it open.
func replaceRecord(temporary, path string) error {
	return os.Rename(temporary, path)
}

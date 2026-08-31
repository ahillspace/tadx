//go:build windows

package artifact

// fsyncDir is a no-op on Windows. NTFS orders directory metadata changes
// (creations and renames) through its journal, and the Win32 FlushFileBuffers
// call is not supported on directory handles opened by the standard library.
// Per-file durability is still enforced by writeFileSync, which flushes each
// staged file before it is renamed into place.
func fsyncDir(string) error { return nil }

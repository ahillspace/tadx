//go:build unix

package fsreplace

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestSyncDirReportsMissingDirectory(t *testing.T) {
	if err := SyncDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("SyncDir() error = nil for a missing directory")
	}
}

func TestCrossDeviceRecognizesARenameAcrossFilesystems(t *testing.T) {
	if !CrossDevice(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EXDEV}) {
		t.Fatal("CrossDevice() = false for EXDEV")
	}
	if CrossDevice(&os.LinkError{Op: "rename", Old: "a", New: "b", Err: syscall.EACCES}) {
		t.Fatal("CrossDevice() = true for EACCES")
	}
}

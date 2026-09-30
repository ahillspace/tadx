//go:build unix

package fsreplace

import (
	"path/filepath"
	"testing"
)

func TestSyncDirReportsMissingDirectory(t *testing.T) {
	if err := SyncDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("SyncDir() error = nil for a missing directory")
	}
}

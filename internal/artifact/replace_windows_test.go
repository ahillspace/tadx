package artifact

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Antivirus and indexing services open files in a freshly written artifact,
// and a concurrent status command reads them. Either blocks renaming the
// artifact directory on Windows until the handle closes.
func TestReplaceDirectoryWaitsForTransientHandleInsideTarget(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, identityComponent("Finance", testServer, testSite, "wb-1"))
	writeArtifactDir(t, target, "Finance", "wb-1")
	staging, err := os.MkdirTemp(root, stagePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "wb.twb"), []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	holder, err := os.Open(filepath.Join(target, "wb.twb"))
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	done := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = holder.Close()
		close(done)
	}()
	defer func() { <-done }()

	if _, err := replaceDirectory(staging, target); err != nil {
		t.Fatalf("replaceDirectory() while a file inside the target was open error = %v", err)
	}
	data, err := os.ReadFile(filepath.Join(target, "wb.twb"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "v2" {
		t.Fatalf("replaced payload = %q, want v2", data)
	}
	assertBackupCount(t, root, 0)
}

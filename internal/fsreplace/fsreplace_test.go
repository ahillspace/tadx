package fsreplace

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReplaceInstallsFileAndSyncsParentDirectory(t *testing.T) {
	directory := t.TempDir()
	from := filepath.Join(directory, ".staged")
	to := filepath.Join(directory, "record.json")
	writeFile(t, from, "new")
	writeFile(t, to, "old")
	var synced []string
	restore := replaceSyncDir(func(path string) error {
		synced = append(synced, path)
		return nil
	})
	defer restore()

	if err := Replace(from, to); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if got := readFile(t, to); got != "new" {
		t.Fatalf("destination = %q, want new", got)
	}
	if _, err := os.Lstat(from); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("source after Replace() Lstat error = %v, want not exist", err)
	}
	if len(synced) != 1 || synced[0] != directory {
		t.Fatalf("synced directories = %v, want [%s]", synced, directory)
	}
}

func TestReplaceInstallsDirectory(t *testing.T) {
	parent := t.TempDir()
	from := filepath.Join(parent, ".stage")
	to := filepath.Join(parent, "root")
	if err := os.Mkdir(from, 0o700); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(from, "file.txt"), "content")

	if err := Replace(from, to); err != nil {
		t.Fatalf("Replace() error = %v", err)
	}
	if got := readFile(t, filepath.Join(to, "file.txt")); got != "content" {
		t.Fatalf("installed file = %q, want content", got)
	}
}

func TestReplaceSkipsSyncWhenRenameFails(t *testing.T) {
	directory := t.TempDir()
	to := filepath.Join(directory, "record.json")
	writeFile(t, to, "old")
	synced := false
	restore := replaceSyncDir(func(string) error {
		synced = true
		return nil
	})
	defer restore()

	err := Replace(filepath.Join(directory, "missing"), to)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Replace() error = %v, want not exist", err)
	}
	if synced {
		t.Fatal("Replace() synced the parent directory after a failed rename")
	}
	if got := readFile(t, to); got != "old" {
		t.Fatalf("destination after failed Replace() = %q, want old", got)
	}
}

func TestReplaceReportsParentSyncFailure(t *testing.T) {
	directory := t.TempDir()
	from := filepath.Join(directory, ".staged")
	to := filepath.Join(directory, "record.json")
	writeFile(t, from, "new")
	failure := errors.New("sync failed")
	restore := replaceSyncDir(func(string) error { return failure })
	defer restore()

	if err := Replace(from, to); !errors.Is(err, failure) {
		t.Fatalf("Replace() error = %v, want the sync failure", err)
	}
}

func TestRenameReturnsPermanentFailureImmediately(t *testing.T) {
	directory := t.TempDir()
	started := time.Now()
	err := Rename(filepath.Join(directory, "missing"), filepath.Join(directory, "target"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("Rename() error = %v, want not exist", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("Rename() took %s for a permanent failure", elapsed)
	}
}

func replaceSyncDir(replacement func(string) error) func() {
	previous := syncDir
	syncDir = replacement
	return func() { syncDir = previous }
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

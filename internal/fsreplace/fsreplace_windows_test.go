package fsreplace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

// The standard library opens files without FILE_SHARE_DELETE, so any open
// handle blocks replacing that file or renaming a directory that contains it.
// These tests hold such handles the way a concurrent reader or file scanner
// would.

func TestRenameReplacesFileAfterTransientHandleCloses(t *testing.T) {
	directory := t.TempDir()
	from := filepath.Join(directory, ".staged")
	to := filepath.Join(directory, "record.json")
	writeFile(t, from, "new")
	writeFile(t, to, "old")
	holder, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	releaseAfter(t, holder, 200*time.Millisecond)

	if err := Rename(from, to); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if got := readFile(t, to); got != "new" {
		t.Fatalf("destination = %q, want new", got)
	}
}

func TestRenameMovesDirectoryAfterTransientHandleInsideCloses(t *testing.T) {
	parent := t.TempDir()
	from := filepath.Join(parent, "artifact")
	to := filepath.Join(parent, ".backup")
	if err := os.Mkdir(from, 0o700); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(from, "file.txt")
	writeFile(t, inside, "content")
	holder, err := os.Open(inside)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	releaseAfter(t, holder, 200*time.Millisecond)

	if err := Rename(from, to); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if got := readFile(t, filepath.Join(to, "file.txt")); got != "content" {
		t.Fatalf("moved file = %q, want content", got)
	}
}

func TestRenameReportsSharingFailureWhenHandleOutlastsBound(t *testing.T) {
	directory := t.TempDir()
	from := filepath.Join(directory, ".staged")
	to := filepath.Join(directory, "record.json")
	writeFile(t, from, "new")
	writeFile(t, to, "old")
	holder, err := os.Open(to)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()

	started := time.Now()
	err = Rename(from, to)
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		t.Fatalf("Rename() error = %v, want the sharing failure", err)
	}
	if elapsed := time.Since(started); elapsed > retryLimit+5*time.Second {
		t.Fatalf("Rename() kept retrying for %s", elapsed)
	}
	if err := holder.Close(); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, to); got != "old" {
		t.Fatalf("destination after failed Rename() = %q, want old", got)
	}
	if got := readFile(t, from); got != "new" {
		t.Fatalf("source after failed Rename() = %q, want new", got)
	}
}

func releaseAfter(t *testing.T, holder *os.File, delay time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() {
		time.Sleep(delay)
		_ = holder.Close()
		close(done)
	}()
	t.Cleanup(func() { <-done })
}

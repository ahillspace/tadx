package workspace_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestDownloadsStageEachPayloadPrivatelyAndCloseRemovesThem(t *testing.T) {
	root := t.TempDir()
	downloads := workspacecore.NewDownloads(root)
	if _, err := os.Stat(filepath.Join(root, ".tadx")); !os.IsNotExist(err) {
		t.Fatalf("staging touched the workspace before a download: %v", err)
	}
	first, err := downloads.File(t.Context(), writeContent("first"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := downloads.File(t.Context(), writeContent("second"))
	if err != nil {
		t.Fatal(err)
	}
	directory := filepath.Dir(first)
	if first == second || filepath.Dir(second) != directory || filepath.Dir(directory) != filepath.Join(root, ".tadx", "tmp") {
		t.Fatalf("staged files = %q, %q", first, second)
	}
	if content, err := os.ReadFile(second); err != nil || string(content) != "second" {
		t.Fatalf("staged content = %q, %v", content, err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
		t.Fatalf("download directory mode = %v", info.Mode().Perm())
	}
	downloads.Close()
	if _, err := os.Stat(directory); !os.IsNotExist(err) {
		t.Fatalf("closing staging left the download directory: %v", err)
	}
}

func TestDownloadsRemoveAFailedPayload(t *testing.T) {
	root := t.TempDir()
	downloads := workspacecore.NewDownloads(root)
	defer downloads.Close()
	failure := errors.New("unexpected EOF")
	_, err := downloads.File(t.Context(), func(destination io.Writer) error {
		_, _ = io.WriteString(destination, "partial")
		return failure
	})
	if !errors.Is(err, failure) {
		t.Fatalf("error = %v", err)
	}
	directories, _ := os.ReadDir(filepath.Join(root, ".tadx", "tmp"))
	for _, directory := range directories {
		if entries, _ := os.ReadDir(filepath.Join(root, ".tadx", "tmp", directory.Name())); len(entries) != 0 {
			t.Fatalf("a failed download left %v", entries)
		}
	}
}

func TestCleanRefusesTemporaryStateWhileADownloadIsStaged(t *testing.T) {
	root := t.TempDir()
	downloads := workspacecore.NewDownloads(root)
	staged, err := downloads.File(t.Context(), writeContent("payload"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = workspacecore.Clean(t.Context(), root, "temporary")
	var advice interface {
		Retryable() bool
		CorrectiveAction() string
	}
	if err == nil || !errors.As(err, &advice) || !advice.Retryable() || !strings.Contains(err.Error(), "in use") {
		t.Fatalf("error = %v", err)
	}
	if _, err := os.Stat(staged); err != nil {
		t.Fatalf("cleanup removed a download in progress: %v", err)
	}
	if _, err := workspacecore.PreviewClean(t.Context(), root, "temporary"); err != nil {
		t.Fatalf("preview while a download is staged: %v", err)
	}
	downloads.Close()
	if err := os.WriteFile(filepath.Join(root, ".tadx", "tmp", "abandoned"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workspacecore.Clean(t.Context(), root, "temporary"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".tadx", "tmp")); !os.IsNotExist(err) {
		t.Fatalf("temporary cleanup left the download state: %v", err)
	}
}

func writeContent(content string) func(io.Writer) error {
	return func(destination io.Writer) error {
		_, err := io.WriteString(destination, content)
		return err
	}
}

func TestDownloadDirRejectsSymlinkedControlState(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation can require Windows developer mode")
	}
	for _, link := range []string{".tadx", filepath.Join(".tadx", "tmp")} {
		t.Run(link, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".tadx"), 0o700); err != nil {
				t.Fatal(err)
			}
			if link == ".tadx" {
				if err := os.Remove(filepath.Join(root, ".tadx")); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(outside, filepath.Join(root, link)); err != nil {
				t.Fatal(err)
			}
			downloads := workspacecore.NewDownloads(root)
			defer downloads.Close()
			if _, err := downloads.File(t.Context(), writeContent("payload")); err == nil {
				t.Fatal("expected symlink boundary rejection")
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Fatalf("download directory escaped the workspace: %v", entries)
			}
		})
	}
}

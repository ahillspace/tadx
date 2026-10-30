package workspace_test

import (
	"context"
	"errors"
	"io"
	"io/fs"
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

func usePrivateTempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, root)
	}
	return root
}

func TestTemporaryDownloadsUsePrivateCommandStorageAndCloseRemovesIt(t *testing.T) {
	root := usePrivateTempRoot(t)
	downloads := workspacecore.NewTemporaryDownloads()
	defer downloads.Close()
	staged, err := downloads.File(t.Context(), writeContent("inspection"))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(filepath.Dir(staged)) != root {
		t.Fatalf("temporary download did not use private OS storage: %q", staged)
	}
	if content, err := os.ReadFile(staged); err != nil || string(content) != "inspection" {
		t.Fatalf("staged content=%q, error=%v", content, err)
	}
	for path, mode := range map[string]os.FileMode{filepath.Dir(staged): 0o700, staged: 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != mode {
			t.Fatalf("temporary storage permissions=%v, want %v", info.Mode().Perm(), mode)
		}
	}
	downloads.Close()
	if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
		t.Fatalf("temporary download storage survived close: entries=%v, error=%v", entries, err)
	}
}

func TestTemporaryDownloadsCleanFailedAndCanceledPayloads(t *testing.T) {
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "failed download", true: "canceled download"}[canceled], func(t *testing.T) {
			root := usePrivateTempRoot(t)
			downloads := workspacecore.NewTemporaryDownloads()
			defer downloads.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			failure := errors.New("download interrupted")
			_, err := downloads.File(ctx, func(output io.Writer) error {
				if _, err := io.WriteString(output, "partial"); err != nil {
					t.Fatal(err)
				}
				if canceled {
					cancel()
					return ctx.Err()
				}
				return failure
			})
			if canceled {
				failure = context.Canceled
			}
			if !errors.Is(err, failure) {
				t.Fatalf("error=%v, want original cause %v", err, failure)
			}
			directories, err := os.ReadDir(root)
			if err != nil || len(directories) != 1 {
				t.Fatalf("private download owner=%v, error=%v", directories, err)
			}
			if files, err := os.ReadDir(filepath.Join(root, directories[0].Name())); err != nil || len(files) != 0 {
				t.Fatalf("failed download retained partial payload: files=%v, error=%v", files, err)
			}
			downloads.Close()
			if entries, err := os.ReadDir(root); err != nil || len(entries) != 0 {
				t.Fatalf("failed download storage survived close: entries=%v, error=%v", entries, err)
			}
		})
	}
}

type retryableStagingFailure struct{ cause error }

func (e retryableStagingFailure) Error() string   { return e.cause.Error() }
func (e retryableStagingFailure) Unwrap() error   { return e.cause }
func (e retryableStagingFailure) Retryable() bool { return true }

func TestDownloadsRenderRelativeErrorsAndPreserveClassification(t *testing.T) {
	root := t.TempDir()
	downloads := workspacecore.NewDownloads(root)
	defer downloads.Close()
	var cause *fs.PathError
	_, err := downloads.File(t.Context(), func(output io.Writer) error {
		cause = &fs.PathError{Op: "write", Path: output.(*os.File).Name(), Err: fs.ErrPermission}
		return retryableStagingFailure{cause: cause}
	})
	if err == nil || strings.Contains(err.Error(), root) || !strings.Contains(err.Error(), ".tadx/tmp/") {
		t.Fatalf("unsafe or incomplete staging error: %v", err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatal("relative rendering lost the operating-system cause")
	}
	unwrapped, ok := errors.AsType[*fs.PathError](err)
	if !ok || unwrapped != cause {
		t.Fatalf("relative rendering lost the original path error: %#v", unwrapped)
	}
	advice, ok := errors.AsType[interface {
		error
		Retryable() bool
	}](err)
	if !ok || !advice.Retryable() {
		t.Fatal("relative rendering lost retry classification")
	}
}

func TestTemporaryDownloadsCreationFailureDoesNotExposeTempRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(root, []byte("occupied"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
		t.Setenv(variable, root)
	}
	downloads := workspacecore.NewTemporaryDownloads()
	defer downloads.Close()
	_, err := downloads.File(t.Context(), writeContent("unreachable"))
	if err == nil || strings.Contains(err.Error(), root) {
		t.Fatalf("temporary storage failure exposed its root: %v", err)
	}
	if cause, ok := errors.AsType[*fs.PathError](err); !ok || cause.Path == "" {
		t.Fatal("temporary storage failure lost its original operating-system cause")
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

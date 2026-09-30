package artifact

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func deniedAt(path string) error {
	return &fs.PathError{Op: "unlinkat", Path: path, Err: syscall.EACCES}
}

func assertWarningsOmitRoot(t *testing.T, warnings []string, root string, want string) {
	t.Helper()
	if len(warnings) == 0 {
		t.Fatal("no warnings, want a cleanup warning")
	}
	for _, warning := range warnings {
		if strings.Contains(warning, root) || strings.Contains(warning, filepath.ToSlash(root)) {
			t.Fatalf("warning %q renders an absolute path under %q", warning, root)
		}
	}
	if !strings.Contains(strings.Join(warnings, "\n"), want) {
		t.Fatalf("warnings = %q, want %q", warnings, want)
	}
}

func TestWarningCauseOmitsPaths(t *testing.T) {
	root := t.TempDir()
	cases := []struct {
		err  error
		want string
	}{
		{deniedAt(filepath.Join(root, "backup")), "unlinkat: " + syscall.EACCES.Error()},
		{&os.LinkError{Op: "rename", Old: filepath.Join(root, "a"), New: filepath.Join(root, "b"), Err: syscall.EACCES}, "rename: " + syscall.EACCES.Error()},
		{errors.Join(errors.New("wrapped"), deniedAt(filepath.Join(root, "backup"))), "unlinkat: " + syscall.EACCES.Error()},
		{errors.New("metadata in " + root + " is invalid"), "fallback"},
	}
	for _, test := range cases {
		if got := warningCause(test.err, "fallback"); got != test.want {
			t.Fatalf("warningCause(%v) = %q, want %q", test.err, got, test.want)
		}
	}
}

func TestReplacementCleanupWarningsOmitAbsolutePaths(t *testing.T) {
	for _, replace := range []struct {
		name string
		run  func(staging, target string, operations directoryOperations) ([]string, error)
	}{
		{"workbook", replaceDirectoryWithOperations},
		{"datasource", replaceDatasourceDirectory},
	} {
		t.Run(replace.name, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			staging := filepath.Join(root, "staging")
			for _, directory := range []string{target, staging} {
				if err := os.Mkdir(directory, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			operations := defaultDirectoryOperations()
			operations.removeAll = func(path string) error { return deniedAt(path) }
			warnings, err := replace.run(staging, target, operations)
			if err != nil {
				t.Fatal(err)
			}
			assertWarningsOmitRoot(t, warnings, root, syscall.EACCES.Error())
		})
	}
}

func TestRecoveryWarningsOmitAbsolutePaths(t *testing.T) {
	for _, recovery := range []struct {
		name          string
		stage, backup string
		run           func(root string, operations directoryOperations) ([]string, error)
	}{
		{"workbook", stagePrefix + "old", backupPrefix + "old", recoverWorkbookRoot},
		{"datasource", datasourceStagePrefix + "old", datasourceBackupPrefix + "old", recoverDatasourceRoot},
	} {
		t.Run(recovery.name, func(t *testing.T) {
			root := t.TempDir()
			for _, name := range []string{recovery.stage, recovery.backup} {
				if err := os.Mkdir(filepath.Join(root, name), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			// A directory where metadata.json belongs makes the backup's metadata invalid.
			if err := os.Mkdir(filepath.Join(root, recovery.backup, "metadata.json"), 0o700); err != nil {
				t.Fatal(err)
			}
			operations := defaultDirectoryOperations()
			operations.removeAll = func(path string) error { return deniedAt(path) }
			warnings, err := recovery.run(root, operations)
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) < 2 {
				t.Fatalf("warnings = %q, want stale staging and orphaned backup warnings", warnings)
			}
			assertWarningsOmitRoot(t, warnings, root, syscall.EACCES.Error())
		})
	}
}

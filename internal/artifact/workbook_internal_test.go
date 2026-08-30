package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReplaceDirectoryReportsCleanupFailureAsWarning(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "new.twb"), []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}

	warnings, err := replaceDirectoryWithOperations(staging, target, directoryOperations{
		stat: os.Stat, rename: os.Rename,
		removeAll: func(string) error { return errors.New("cleanup denied") },
	})
	if err != nil {
		t.Fatalf("replaceDirectoryWithOperations() error = %v", err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "replacement committed") || !strings.Contains(warnings[0], "cleanup denied") {
		t.Fatalf("warnings = %#v", warnings)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	foundHiddenBackup := false
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".tadx-workbook-backup-") {
			foundHiddenBackup = true
		}
	}
	if !foundHiddenBackup {
		t.Fatalf("backup entries = %#v", entries)
	}
	content, err := os.ReadFile(filepath.Join(target, "new.twb"))
	if err != nil || string(content) != "new" {
		t.Fatalf("installed content = %q, error = %v", content, err)
	}
}

func TestReplaceDirectoryPreservesInstallAndRollbackFailures(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	renameCount := 0
	_, err := replaceDirectoryWithOperations(staging, target, directoryOperations{
		stat: os.Stat,
		rename: func(oldPath, newPath string) error {
			renameCount++
			switch renameCount {
			case 1:
				return os.Rename(oldPath, newPath)
			case 2:
				return errors.New("install denied")
			default:
				return errors.New("restore denied")
			}
		},
		removeAll: os.RemoveAll,
	})
	if err == nil || !strings.Contains(err.Error(), "install denied") || !strings.Contains(err.Error(), "restore denied") {
		t.Fatalf("replaceDirectoryWithOperations() error = %v", err)
	}
}

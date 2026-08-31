package artifact

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeArtifactDir creates a minimal, readable managed-artifact directory whose
// metadata carries the given identity, sufficient for recovery reconciliation.
func writeArtifactDir(t *testing.T, dir, name, luid string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	metadata := map[string]string{
		"kind":                       "workbook",
		"name":                       name,
		"tableau_id":                 luid,
		"source_server_origin":       "https://tableau.example.com",
		"source_site_luid":           "site-1",
		"source_site":                "",
		"canonical_payload":          "wb.twb",
		"local_baseline_fingerprint": "sha256:0",
		"pulled_at":                  "2026-08-31T00:00:00Z",
	}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "wb.twb"), []byte(name+luid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "view.md"), []byte("# "+name), 0o600); err != nil {
		t.Fatal(err)
	}
}

const (
	testServer = "https://tableau.example.com"
	testSite   = "site-1"
)

func TestWriteStagedArtifactSyncsFilesAndDirectory(t *testing.T) {
	staging := t.TempDir()
	var syncedDirs []string
	writtenAndSynced := map[string]bool{}
	operations := directoryOperations{
		writeFile: func(path string, data []byte, perm os.FileMode) error {
			if err := os.WriteFile(path, data, perm); err != nil {
				return err
			}
			writtenAndSynced[filepath.Base(path)] = true
			return nil
		},
		syncDir: func(path string) error {
			syncedDirs = append(syncedDirs, path)
			return nil
		},
	}
	files := map[string][]byte{"wb.twb": []byte("x"), "metadata.json": []byte("{}"), "view.md": []byte("v")}
	if err := writeStagedArtifact(staging, files, operations); err != nil {
		t.Fatalf("writeStagedArtifact() error = %v", err)
	}
	for name := range files {
		if !writtenAndSynced[name] {
			t.Fatalf("staged file %q was not written through the durable writer", name)
		}
	}
	if len(syncedDirs) != 1 || syncedDirs[0] != staging {
		t.Fatalf("syncDir calls = %#v, want one call for staging %q", syncedDirs, staging)
	}
}

func TestReplaceDirectorySyncsParentAfterInstall(t *testing.T) {
	for _, preexisting := range []bool{false, true} {
		t.Run(map[bool]string{false: "newInstall", true: "replacement"}[preexisting], func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "target")
			staging := filepath.Join(root, "staging")
			if err := os.Mkdir(staging, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(staging, "new.twb"), []byte("new"), 0o600); err != nil {
				t.Fatal(err)
			}
			if preexisting {
				if err := os.Mkdir(target, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			var syncedDirs []string
			operations := directoryOperations{
				syncDir: func(path string) error { syncedDirs = append(syncedDirs, path); return nil },
			}
			if _, err := replaceDirectoryWithOperations(staging, target, operations); err != nil {
				t.Fatalf("replaceDirectoryWithOperations() error = %v", err)
			}
			foundParent := false
			for _, d := range syncedDirs {
				if d == root {
					foundParent = true
				}
			}
			if !foundParent {
				t.Fatalf("parent directory %q was never fsynced; syncDir calls = %#v", root, syncedDirs)
			}
		})
	}
}

func TestReplaceDirectorySurfacesParentSyncFailure(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target")
	staging := filepath.Join(root, "staging")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := replaceDirectoryWithOperations(staging, target, directoryOperations{
		syncDir: func(string) error { return errors.New("disk lost power") },
	})
	if err == nil || !strings.Contains(err.Error(), "disk lost power") {
		t.Fatalf("replaceDirectoryWithOperations() error = %v, want parent-sync failure surfaced", err)
	}
}

func TestUniqueSuffixIsCollisionResistant(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 10000; i++ {
		suffix := uniqueSuffix()
		if seen[suffix] {
			t.Fatalf("uniqueSuffix() produced a duplicate %q", suffix)
		}
		seen[suffix] = true
	}
}

func TestRecoverRestoresOrphanedBackupAfterCrashInWindow(t *testing.T) {
	root := t.TempDir()
	name := identityComponent("Finance", testServer, testSite, "wb-1")
	target := filepath.Join(root, name)
	writeArtifactDir(t, target, "Finance", "wb-1")

	// Simulate a crash in the replaceDirectory backup window: the first rename
	// (target -> backup) succeeds, then both the install and the rollback fail,
	// leaving a lone backup with the target missing.
	staging, err := os.MkdirTemp(root, stagePrefix)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "wb.twb"), []byte("v2"), 0o600); err != nil {
		t.Fatal(err)
	}
	renameCount := 0
	_, err = replaceDirectoryWithOperations(staging, target, directoryOperations{
		rename: func(oldPath, newPath string) error {
			renameCount++
			switch renameCount {
			case 1:
				return os.Rename(oldPath, newPath) // target -> backup
			case 2:
				return errors.New("crash: install lost")
			default:
				return errors.New("crash: rollback lost")
			}
		},
	})
	if err == nil {
		t.Fatal("expected simulated crash to fail the replacement")
	}
	if _, statErr := os.Stat(target); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("post-crash target stat = %v, want ErrNotExist (artifact should appear deleted)", statErr)
	}
	assertBackupCount(t, root, 1)

	// Recovery must resurrect the artifact and clear the stale staging dir.
	warnings, err := recoverWorkbookRoot(root, defaultDirectoryOperations())
	if err != nil {
		t.Fatalf("recoverWorkbookRoot() error = %v", err)
	}
	if len(warnings) == 0 || !strings.Contains(strings.Join(warnings, ";"), "recovered workbook artifact") {
		t.Fatalf("recovery warnings = %#v, want a recovery notice", warnings)
	}
	metadata, err := readMetadata(target)
	if err != nil {
		t.Fatalf("restored artifact unreadable: %v", err)
	}
	if metadata.TableauID != "wb-1" {
		t.Fatalf("restored identity = %q, want wb-1", metadata.TableauID)
	}
	assertBackupCount(t, root, 0)
	assertNoStages(t, root)
}

func TestRecoverGarbageCollectsCommittedBackupAndStages(t *testing.T) {
	root := t.TempDir()
	name := identityComponent("Finance", testServer, testSite, "wb-1")
	live := filepath.Join(root, name)
	writeArtifactDir(t, live, "Finance", "wb-1")

	// A committed backup for the same identity that the crash never removed.
	backup := filepath.Join(root, backupPrefix+name+"-"+uniqueSuffix())
	writeArtifactDir(t, backup, "Finance", "wb-1")
	// An abandoned staging directory from a crashed pull.
	if _, err := os.MkdirTemp(root, stagePrefix); err != nil {
		t.Fatal(err)
	}

	if _, err := recoverWorkbookRoot(root, defaultDirectoryOperations()); err != nil {
		t.Fatalf("recoverWorkbookRoot() error = %v", err)
	}
	assertBackupCount(t, root, 0)
	assertNoStages(t, root)
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("live artifact was disturbed: %v", err)
	}
}

func assertBackupCount(t *testing.T, root string, want int) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), backupPrefix) {
			got++
		}
	}
	if got != want {
		t.Fatalf("backup count = %d, want %d (entries=%v)", got, want, entries)
	}
}

func assertNoStages(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), stagePrefix) {
			t.Fatalf("stale staging directory survived recovery: %q", e.Name())
		}
	}
}

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

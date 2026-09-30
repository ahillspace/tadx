package artifact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Recovery only reconciles real directories that tadx staged itself. A link
// or file that merely carries a recovery prefix is left in place with a
// warning, never followed, removed through, or installed as a live artifact.

func TestRecoverWorkbookRootLeavesLinkedAndIrregularRecoveryEntries(t *testing.T) {
	root := t.TempDir()
	outsideBackup := t.TempDir()
	writeArtifactDir(t, outsideBackup, "Finance", "wb-1")
	outsideStage := t.TempDir()
	outsideFile := filepath.Join(outsideStage, "keep.txt")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedBackup := filepath.Join(root, backupPrefix+"linked")
	linkedStage := filepath.Join(root, stagePrefix+"linked")
	symlinkOrSkip(t, outsideBackup, linkedBackup)
	symlinkOrSkip(t, outsideStage, linkedStage)
	fileBackup := filepath.Join(root, backupPrefix+"file")
	if err := os.WriteFile(fileBackup, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	warnings, err := recoverWorkbookRoot(root, defaultDirectoryOperations())
	if err != nil {
		t.Fatalf("recoverWorkbookRoot() error = %v", err)
	}

	assertRecoveryEntriesLeft(t, warnings, linkedBackup, linkedStage, fileBackup)
	restored := filepath.Join(root, identityComponent("Finance", testServer, testSite, "wb-1"))
	if _, err := os.Lstat(restored); !os.IsNotExist(err) {
		t.Fatalf("linked backup was installed as %q: Lstat error = %v", restored, err)
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("recovery removed content through a linked staging entry: %v", err)
	}
}

func TestRecoverDatasourceRootLeavesLinkedAndIrregularRecoveryEntries(t *testing.T) {
	root := t.TempDir()
	outsideBackup := t.TempDir()
	metadata := DatasourceMetadata{Kind: "datasource", Name: "Sales", TableauID: "ds-1", SourceServerOrigin: testServer, SourceSiteLUID: testSite}
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideBackup, "metadata.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	outsideStage := t.TempDir()
	outsideFile := filepath.Join(outsideStage, "keep.txt")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	linkedBackup := filepath.Join(root, datasourceBackupPrefix+"linked")
	linkedStage := filepath.Join(root, datasourceStagePrefix+"linked")
	symlinkOrSkip(t, outsideBackup, linkedBackup)
	symlinkOrSkip(t, outsideStage, linkedStage)
	fileBackup := filepath.Join(root, datasourceBackupPrefix+"file")
	if err := os.WriteFile(fileBackup, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	warnings, err := recoverDatasourceRoot(root, defaultDirectoryOperations())
	if err != nil {
		t.Fatalf("recoverDatasourceRoot() error = %v", err)
	}

	assertRecoveryEntriesLeft(t, warnings, linkedBackup, linkedStage, fileBackup)
	restored := filepath.Join(root, datasourceIdentityComponent("Sales", testServer, testSite, "ds-1"))
	if _, err := os.Lstat(restored); !os.IsNotExist(err) {
		t.Fatalf("linked backup was installed as %q: Lstat error = %v", restored, err)
	}
	if _, err := os.Stat(outsideFile); err != nil {
		t.Fatalf("recovery removed content through a linked staging entry: %v", err)
	}
}

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
}

func assertRecoveryEntriesLeft(t *testing.T, warnings []string, paths ...string) {
	t.Helper()
	joined := strings.Join(warnings, "\n")
	for _, path := range paths {
		if _, err := os.Lstat(path); err != nil {
			t.Fatalf("recovery entry %q was not left in place: %v", filepath.Base(path), err)
		}
		if !strings.Contains(joined, filepath.Base(path)) || !strings.Contains(joined, "not a real directory") {
			t.Fatalf("recovery warnings = %q, want a not-a-real-directory warning for %q", warnings, filepath.Base(path))
		}
	}
}

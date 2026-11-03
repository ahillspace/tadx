package artifact

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStandalonePullReconcilesInterruptedBundleBeforeAcceptingNewWork(t *testing.T) {
	for _, existing := range []bool{false, true} {
		name := "new target"
		if existing {
			name = "existing target backup"
		}
		t.Run(name, func(t *testing.T) {
			workspace := createDatasourceWorkspace(t)
			input := DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata("Sales", "ds-1")}
			manager := NewDatasourceManager(time.Now)
			if existing {
				input.PayloadPath = PayloadFile(t, []byte(`<datasource><connection dbname="original"/></datasource>`))
				if _, err := manager.Pull(t.Context(), input); err != nil {
					t.Fatal(err)
				}
			}
			root, err := ensureDatasourceRoot(workspace)
			if err != nil {
				t.Fatal(err)
			}
			input.PayloadPath = PayloadFile(t, []byte(`<datasource><connection dbname="interrupted"/></datasource>`))
			prepared, _, err := NewWorkbookBundleManager(time.Now).prepareDatasource(t.Context(), workspace, root, input, defaultDirectoryOperations())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := prepareBundleJournal([]preparedDirectory{prepared}, defaultDirectoryOperations())
			if err != nil {
				t.Fatal(err)
			}
			if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
				t.Fatal(err)
			}
			if existing {
				if err := os.Rename(prepared.target, journal.Entries[0].Backup); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(prepared.staging, prepared.target); err != nil {
					t.Fatal(err)
				}
			}
			input.PayloadPath = PayloadFile(t, []byte(`<datasource><connection dbname="successful-pull"/></datasource>`))
			pulled, err := manager.Pull(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			edits := `<datasource><connection dbname="later-user-edits"/></datasource>`
			if err := os.WriteFile(pulled.CanonicalPath, []byte(edits), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := NewWorkbookBundleManager(time.Now).Pull(t.Context(), validWorkbookBundle(t, workspace, "next")); err == nil || !strings.Contains(err.Error(), "dirty") {
				t.Fatalf("next bundle must protect later edits: %v", err)
			}
			assertFileContent(t, pulled.CanonicalPath, edits)
			if _, err := os.Stat(bundleJournalPath(workspace)); !os.IsNotExist(err) {
				t.Fatalf("standalone pull left prior journal: %v", err)
			}
		})
	}
}

func TestBundleRecoveryPreservesNewerGenerationAndOwnedBackup(t *testing.T) {
	for _, backup := range []bool{false, true} {
		t.Run(map[bool]string{false: "new target", true: "existing target backup"}[backup], func(t *testing.T) {
			workspace := createDatasourceWorkspace(t)
			input := DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata("Sales", "ds-1"), PayloadPath: PayloadFile(t, []byte(`<datasource><connection dbname="newer"/></datasource>`))}
			pulled, err := NewDatasourceManager(time.Now).Pull(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			entry := bundleJournalEntry{Target: pulled.ArtifactPath, HadTarget: backup}
			if backup {
				entry.Backup = filepath.Join(filepath.Dir(pulled.ArtifactPath), datasourceBackupPrefix+"prior-generation")
				writeBundleFixture(t, entry.Backup, "prior backup")
			}
			if err := writeBundleJournal(workspace, bundleJournal{State: bundleStatePrepared, Entries: []bundleJournalEntry{entry}}, defaultDirectoryOperations()); err != nil {
				t.Fatal(err)
			}
			input.PayloadPath = PayloadFile(t, []byte(`<datasource/>`))
			if _, err := NewDatasourceManager(time.Now).Pull(t.Context(), input); err == nil || !strings.Contains(err.Error(), "recovery conflict") {
				t.Fatalf("stale journal error = %v", err)
			}
			assertFileContent(t, pulled.CanonicalPath, `<datasource><connection dbname="newer"/></datasource>`)
			if backup {
				assertFileContent(t, filepath.Join(entry.Backup, "payload"), "prior backup")
			}
			if _, err := os.Stat(bundleJournalPath(workspace)); err != nil {
				t.Fatalf("conflicting recovery lost journal: %v", err)
			}
		})
	}
}

func TestBundleRecoveryPreservesEditsToItsOwnInstalledGeneration(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	root, err := ensureDatasourceRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	input := DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata("Sales", "ds-1"), PayloadPath: PayloadFile(t, []byte(`<datasource/>`))}
	prepared, _, err := NewWorkbookBundleManager(time.Now).prepareDatasource(t.Context(), workspace, root, input, defaultDirectoryOperations())
	if err != nil {
		t.Fatal(err)
	}
	journal, err := prepareBundleJournal([]preparedDirectory{prepared}, defaultDirectoryOperations())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(prepared.staging, prepared.target); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(prepared.target, "Sales.tds")
	if err := os.WriteFile(canonical, []byte("local edits before recovery"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := recoverBundleTransaction(workspace, defaultDirectoryOperations()); err == nil || !strings.Contains(err.Error(), "recovery conflict") {
		t.Fatalf("edited transaction recovery error = %v", err)
	}
	assertFileContent(t, canonical, "local edits before recovery")
}

func TestFailedBundleRollbackRetainsJournalOwnedStagesAndBackups(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewWorkbookBundleManager(time.Now)
	initial, err := manager.Pull(t.Context(), validWorkbookBundle(t, workspace, "original"))
	if err != nil {
		t.Fatal(err)
	}
	target := initial.Datasources[1].ArtifactPath
	manager.operations.rename = func(from, to string) error {
		if to == target && (strings.HasPrefix(filepath.Base(from), datasourceStagePrefix) || strings.HasPrefix(filepath.Base(from), datasourceBackupPrefix)) {
			return errors.New("injected install and restore interruption")
		}
		return os.Rename(from, to)
	}
	if _, err := manager.Pull(t.Context(), validWorkbookBundle(t, workspace, "interrupted")); err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("interrupted rollback error = %v", err)
	}
	data, err := os.ReadFile(bundleJournalPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	var journal bundleJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		t.Fatal(err)
	}
	var ownedStage, ownedBackup string
	for _, entry := range journal.Entries {
		if entry.Target == target {
			ownedStage, ownedBackup = entry.Staging, entry.Backup
		}
	}
	if _, err := os.Stat(ownedStage); err != nil {
		t.Fatalf("failed rollback lost the live journal's stage: %v", err)
	}
	note := filepath.Join(ownedBackup, "local-notes.txt")
	if err := os.WriteFile(note, []byte("preserve recovery evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	standalone := validWorkbookBundle(t, workspace, "later").Datasources[0]
	if _, err := NewDatasourceManager(time.Now).Pull(t.Context(), standalone); err == nil || !strings.Contains(err.Error(), "recovery conflict") {
		t.Fatalf("standalone pull bypassed conflicting live recovery: %v", err)
	}
	assertFileContent(t, note, "preserve recovery evidence")
	if _, err := os.Stat(ownedStage); err != nil {
		t.Fatalf("standalone cleanup removed the live journal's stage: %v", err)
	}
	if _, err := os.Stat(bundleJournalPath(workspace)); err != nil {
		t.Fatalf("standalone cleanup lost the pending journal: %v", err)
	}
}

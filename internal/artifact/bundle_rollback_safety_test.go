package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBundleRollbackValidatesStageInEveryOriginalTargetState(t *testing.T) {
	for _, state := range []string{"untouched", "backup present", "already restored"} {
		for _, stage := range []string{"unchanged", "modified", "missing"} {
			name := state + "/" + stage + " stage"
			t.Run(name, func(t *testing.T) {
				workspace := createDatasourceWorkspace(t)
				input := DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata("Sales", "datasource-1"), PayloadPath: PayloadFile(t, []byte("original"))}
				original, err := NewDatasourceManager(time.Now).Pull(t.Context(), input)
				if err != nil {
					t.Fatal(err)
				}
				root, err := ensureDatasourceRoot(workspace)
				if err != nil {
					t.Fatal(err)
				}
				input.PayloadPath = PayloadFile(t, []byte("prepared"))
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
				entry := journal.Entries[0]
				if state != "untouched" {
					if err := os.Rename(entry.Target, entry.Backup); err != nil {
						t.Fatal(err)
					}
				}
				if state == "already restored" {
					if err := os.Rename(entry.Backup, entry.Target); err != nil {
						t.Fatal(err)
					}
				}
				note := filepath.Join(entry.Staging, "recovery-notes.txt")
				if stage == "modified" {
					if err := os.WriteFile(note, []byte("retain recovery evidence"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if stage == "missing" {
					if err := os.RemoveAll(entry.Staging); err != nil {
						t.Fatal(err)
					}
				}
				err = recoverBundleTransaction(workspace, defaultDirectoryOperations())
				if stage == "modified" {
					if err == nil || !strings.Contains(err.Error(), "recovery conflict") {
						t.Errorf("modified stage recovery error = %v", err)
					}
					assertFileContent(t, note, "retain recovery evidence")
					if _, err := os.Stat(bundleJournalPath(workspace)); err != nil {
						t.Fatalf("conflicting recovery lost journal: %v", err)
					}
					if state == "backup present" {
						assertFileContent(t, filepath.Join(entry.Backup, "Sales.tds"), "original")
						if _, err := os.Stat(entry.Target); !errors.Is(err, os.ErrNotExist) {
							t.Fatalf("conflicting recovery changed absent target: %v", err)
						}
					} else {
						assertFileContent(t, original.CanonicalPath, "original")
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				assertFileContent(t, original.CanonicalPath, "original")
				for _, path := range []string{entry.Staging, entry.Backup, bundleJournalPath(workspace)} {
					if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
						t.Errorf("completed rollback retained %q: %v", path, err)
					}
				}
			})
		}
	}
}

func TestBundleRollbackPreservesUnverifiableSurvivingStage(t *testing.T) {
	for _, missingProof := range []string{"legacy ownerless journal", "missing fingerprint", "different owner", "missing owner marker"} {
		t.Run(missingProof, func(t *testing.T) {
			workspace := createDatasourceWorkspace(t)
			root, err := ensureDatasourceRoot(workspace)
			if err != nil {
				t.Fatal(err)
			}
			input := DatasourcePull{Workspace: workspace, Filename: "Sales.tds", Metadata: validDatasourceMetadata("Sales", "datasource-1"), PayloadPath: PayloadFile(t, []byte("prepared"))}
			prepared, _, err := NewWorkbookBundleManager(time.Now).prepareDatasource(t.Context(), workspace, root, input, defaultDirectoryOperations())
			if err != nil {
				t.Fatal(err)
			}
			journal, err := prepareBundleJournal([]preparedDirectory{prepared}, defaultDirectoryOperations())
			if err != nil {
				t.Fatal(err)
			}
			switch missingProof {
			case "legacy ownerless journal":
				journal.Entries[0].Owner, journal.Entries[0].Fingerprint = "", ""
			case "missing fingerprint":
				journal.Entries[0].Fingerprint = ""
			case "different owner":
				if err := os.WriteFile(filepath.Join(prepared.staging, bundleOwnerFile), []byte("another-generation"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "missing owner marker":
				if err := os.Remove(filepath.Join(prepared.staging, bundleOwnerFile)); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
				t.Fatal(err)
			}
			if err := recoverBundleTransaction(workspace, defaultDirectoryOperations()); err == nil || !strings.Contains(err.Error(), "recovery conflict") {
				t.Errorf("unverifiable stage recovery error = %v", err)
			}
			assertFileContent(t, filepath.Join(prepared.staging, "Sales.tds"), "prepared")
			if _, err := os.Stat(bundleJournalPath(workspace)); err != nil {
				t.Fatalf("unverifiable stage lost journal: %v", err)
			}
		})
	}
}

func TestBundleRollbackPreflightsAllStagesBeforeAnyMutation(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	inputs := validWorkbookBundle(t, workspace, "original").Datasources
	manager := NewDatasourceManager(time.Now)
	for _, input := range inputs {
		if _, err := manager.Pull(t.Context(), input); err != nil {
			t.Fatal(err)
		}
	}
	root, err := ensureDatasourceRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	var prepared []preparedDirectory
	for _, input := range inputs {
		input.PayloadPath = PayloadFile(t, []byte("prepared"))
		item, _, err := NewWorkbookBundleManager(time.Now).prepareDatasource(t.Context(), workspace, root, input, defaultDirectoryOperations())
		if err != nil {
			t.Fatal(err)
		}
		prepared = append(prepared, item)
	}
	journal, err := prepareBundleJournal(prepared, defaultDirectoryOperations())
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	first := journal.Entries[0]
	if err := os.Rename(first.Target, first.Backup); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(first.Staging, first.Target); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(journal.Entries[1].Staging, "recovery-notes.txt")
	if err := os.WriteFile(note, []byte("retain recovery evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	operations := defaultDirectoryOperations()
	mutations := 0
	operations.removeAll = func(path string) error {
		mutations++
		return os.RemoveAll(path)
	}
	operations.rename = func(from, to string) error {
		mutations++
		return os.Rename(from, to)
	}
	if err := recoverBundleTransaction(workspace, operations); err == nil || !strings.Contains(err.Error(), "recovery conflict") {
		t.Errorf("conflicting later stage recovery error = %v", err)
	}
	if mutations != 0 {
		t.Errorf("recovery performed %d mutations before rejecting a conflicting stage", mutations)
	}
	assertFileContent(t, filepath.Join(first.Target, inputs[0].Filename), "prepared")
	assertFileContent(t, note, "retain recovery evidence")
	if _, err := os.Stat(first.Backup); err != nil {
		t.Fatalf("conflicting later stage lost original backup: %v", err)
	}
	if _, err := os.Stat(bundleJournalPath(workspace)); err != nil {
		t.Fatalf("conflicting later stage lost journal: %v", err)
	}
}

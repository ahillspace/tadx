package artifact

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWorkbookBundleRejectsLaterDirtyDependencyWithoutChangingAnyArtifact(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewWorkbookBundleManager(func() time.Time { return time.Date(2026, 8, 31, 20, 0, 0, 0, time.UTC) })
	initial, err := manager.Pull(context.Background(), validWorkbookBundle(workspace, "v1"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initial.Workbook.CanonicalPath, []byte("local workbook edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(initial.Datasources[1].CanonicalPath, []byte("local datasource edit"), 0o600); err != nil {
		t.Fatal(err)
	}

	refresh := validWorkbookBundle(workspace, "v2")
	refresh.Workbook.Overwrite = true
	_, err = manager.Pull(context.Background(), refresh)
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("Pull error = %v, want dirty dependency rejection", err)
	}
	assertFileContent(t, initial.Workbook.CanonicalPath, "local workbook edit")
	assertFileContent(t, initial.Datasources[0].CanonicalPath, "datasource-1-v1")
	assertFileContent(t, initial.Datasources[1].CanonicalPath, "local datasource edit")
}

func TestWorkbookBundlePersistsWorkbookLineageInSameTransaction(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	input := validWorkbookBundle(workspace, "v1")
	input.Workbook.Lineage = LineageDocument{
		Complete: true, Direction: "both", Depth: 1,
		Nodes: []LineageNode{{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1"}},
		Edges: []LineageEdge{},
	}
	input.Workbook.LineageCountsKnown = true

	result, err := NewWorkbookBundleManager(time.Now).Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if result.Workbook.LineageStatus != LineageStatusComplete || result.Workbook.LineagePath != filepath.Join(result.Workbook.ArtifactPath, "lineage.json") {
		t.Fatalf("workbook result = %#v", result.Workbook)
	}
	for _, name := range []string{"metadata.json", "lineage.json", "view.md", "Finance.twbx"} {
		if _, err := os.Stat(filepath.Join(result.Workbook.ArtifactPath, name)); err != nil {
			t.Fatalf("missing workbook bundle file %q: %v", name, err)
		}
	}
	metadata, err := readMetadata(result.Workbook.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.LineageNodeCount == nil || *metadata.LineageNodeCount != 1 || metadata.LineageEdgeCount == nil || *metadata.LineageEdgeCount != 0 {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookBundleClassifiesAcquiredDatasourceForRepublish(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	input := validWorkbookBundle(workspace, "v1")
	input.Workbook.Metadata.PublishedDatasources = input.Workbook.Metadata.PublishedDatasources[:1]
	input.Datasources = input.Datasources[:1]
	input.Datasources[0].Filename = "Sales.tds"
	input.Datasources[0].Content = []byte(`<datasource><connection class="sqlserver"/></datasource>`)

	result, err := NewWorkbookBundleManager(time.Now).Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := NewDatasourceManager(time.Now).Read(context.Background(), result.Datasources[0].ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CompositionStatus != CompositionStatusOrdinary || len(stored.ParentDataSourceURLs) != 0 {
		t.Fatalf("acquired datasource composition = %q, parents = %#v", stored.CompositionStatus, stored.ParentDataSourceURLs)
	}
	if string(input.Datasources[0].Content) != `<datasource><connection class="sqlserver"/></datasource>` {
		t.Fatal("dependency classification changed native datasource bytes")
	}
}

func TestWorkbookBundleRestoresEveryArtifactWhenLaterInstallFails(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewWorkbookBundleManager(time.Now)
	initial, err := manager.Pull(context.Background(), validWorkbookBundle(workspace, "v1"))
	if err != nil {
		t.Fatal(err)
	}

	realRename := os.Rename
	renames := 0
	manager.operations.rename = func(oldPath, newPath string) error {
		renames++
		if renames == 5 {
			return errors.New("injected later install failure")
		}
		return realRename(oldPath, newPath)
	}
	_, err = manager.Pull(context.Background(), validWorkbookBundle(workspace, "v2"))
	if err == nil || !strings.Contains(err.Error(), "injected later install failure") {
		t.Fatalf("Pull error = %v", err)
	}
	assertFileContent(t, initial.Workbook.CanonicalPath, "workbook-v1")
	assertFileContent(t, initial.Datasources[0].CanonicalPath, "datasource-1-v1")
	assertFileContent(t, initial.Datasources[1].CanonicalPath, "datasource-2-v1")
	for _, root := range []string{filepath.Join(workspace, "artifacts", "workbook"), filepath.Join(workspace, "artifacts", "datasource")} {
		entries, readErr := os.ReadDir(root)
		if readErr != nil {
			t.Fatal(readErr)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), ".tadx-") {
				t.Fatalf("transaction residue remains at %q", filepath.Join(root, entry.Name()))
			}
		}
	}
}

func TestWorkbookBundleCleansEveryStageWhenPreflightRejectsDuplicateDatasource(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewWorkbookBundleManager(time.Now)
	input := validWorkbookBundle(workspace, "v1")
	input.Datasources[1].Metadata.TableauID = input.Datasources[0].Metadata.TableauID

	_, err := manager.Pull(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("Pull error = %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "datasource"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), datasourceStagePrefix) {
			t.Fatalf("staging directory remains at %q", entry.Name())
		}
	}
}

func TestWorkbookBundleRecoveryRollsBackPreparedTransactionAfterProcessExit(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	root := filepath.Join(workspace, "artifacts", "datasource")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	oldTarget := filepath.Join(root, "Sales")
	oldBackup := filepath.Join(root, datasourceBackupPrefix+"Sales-test")
	newTarget := filepath.Join(root, "Inventory")
	writeBundleFixture(t, oldTarget, "old")
	if err := os.Rename(oldTarget, oldBackup); err != nil {
		t.Fatal(err)
	}
	writeBundleFixture(t, oldTarget, "new")
	writeBundleFixture(t, newTarget, "new inventory")
	journal := bundleJournal{State: bundleStatePrepared, Entries: []bundleJournalEntry{
		{Target: oldTarget, Backup: oldBackup, HadTarget: true},
		{Target: newTarget, HadTarget: false},
	}}
	if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}

	if err := recoverBundleTransaction(workspace, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(oldTarget, "payload"), "old")
	if _, err := os.Stat(newTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("new target remains after rollback: %v", err)
	}
	if _, err := os.Stat(bundleJournalPath(workspace)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal remains after recovery: %v", err)
	}
}

func TestWorkbookBundleRecoveryCompletesCommittedTransactionAfterProcessExit(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	root := filepath.Join(workspace, "artifacts", "workbook")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "Finance")
	backup := filepath.Join(root, ".tadx-workbook-backup-Finance-test")
	writeBundleFixture(t, target, "new")
	writeBundleFixture(t, backup, "old")
	journal := bundleJournal{State: bundleStateCommitted, Entries: []bundleJournalEntry{{Target: target, Backup: backup, HadTarget: true}}}
	if err := writeBundleJournal(workspace, journal, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}

	if err := recoverBundleTransaction(workspace, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(target, "payload"), "new")
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("backup remains after committed recovery: %v", err)
	}
}

func TestWorkbookBundleRecoveryKeepsCommittedJournalUntilCleanupFinishes(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	root := filepath.Join(workspace, "artifacts", "workbook")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "Finance")
	writeBundleFixture(t, target, "new")
	prepared := bundleJournal{State: bundleStatePrepared, Entries: []bundleJournalEntry{{Target: target, HadTarget: false}}}
	if err := writeBundleJournal(workspace, prepared, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	committed := prepared
	committed.State = bundleStateCommitted
	if err := writeBundleJournal(workspace, committed, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	operations := defaultDirectoryOperations()
	realRemoveAll := operations.removeAll
	operations.removeAll = func(path string) error {
		if path == bundleCommittedPath(workspace) {
			return errors.New("injected cleanup interruption")
		}
		return realRemoveAll(path)
	}
	if err := recoverBundleTransaction(workspace, operations); err == nil || !strings.Contains(err.Error(), "injected cleanup interruption") {
		t.Fatalf("recovery error = %v", err)
	}
	if _, err := os.Stat(bundleCommittedPath(workspace)); err != nil {
		t.Fatalf("committed recovery journal was removed early: %v", err)
	}
	if err := recoverBundleTransaction(workspace, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(target, "payload"), "new")
}

func TestWorkbookBundleCleanupRemovesPreparedBeforeCommittedJournal(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	var events []string
	operations := defaultDirectoryOperations()
	operations.removeAll = func(path string) error {
		events = append(events, "remove "+path)
		return nil
	}
	operations.syncDir = func(path string) error {
		events = append(events, "sync "+path)
		return nil
	}

	if err := removeBundleJournal(workspace, operations); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"remove " + bundleJournalPath(workspace),
		"sync " + filepath.Join(workspace, "artifacts"),
		"remove " + bundleCommittedPath(workspace),
		"sync " + filepath.Join(workspace, "artifacts"),
	}
	if strings.Join(events, "\n") != strings.Join(want, "\n") {
		t.Fatalf("cleanup events = %#v, want %#v", events, want)
	}
}

func TestWorkbookBundleJournalDoesNotReplaceExistingRecoveryOwner(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	root := filepath.Join(workspace, "artifacts", "datasource")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	first := bundleJournal{State: bundleStatePrepared, Entries: []bundleJournalEntry{{
		Target: filepath.Join(root, "Sales"), Staging: filepath.Join(root, ".tadx-stage-first"),
	}}}
	second := bundleJournal{State: bundleStatePrepared, Entries: []bundleJournalEntry{{
		Target: filepath.Join(root, "Inventory"), Staging: filepath.Join(root, ".tadx-stage-second"),
	}}}
	if err := writeBundleJournal(workspace, first, defaultDirectoryOperations()); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(bundleJournalPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if err := writeBundleJournal(workspace, second, defaultDirectoryOperations()); err == nil {
		t.Fatal("second transaction replaced the active recovery journal")
	}
	after, err := os.ReadFile(bundleJournalPath(workspace))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("active recovery journal changed:\n%s", after)
	}
}

func writeBundleFixture(t *testing.T, directory, content string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "payload"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func validWorkbookBundle(workspace, version string) WorkbookBundlePull {
	workbookMetadata := WorkbookMetadata{
		Kind: "workbook", Name: "Finance", TableauID: "wb-1",
		SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1",
		SourceEnvironment: "dev", SourceSite: "test-site", SourceProjectName: "Ops", SourceProjectID: "project-1",
		Portability: PortabilitySourceSiteBound, DependenciesAcquired: true,
		PublishedDatasources: []PublishedDatasourceRef{{LUID: "ds-1", Name: "Sales"}, {LUID: "ds-2", Name: "Inventory"}},
	}
	datasource := func(name, luid string) DatasourcePull {
		metadata := validDatasourceMetadata(name, luid)
		return DatasourcePull{Workspace: workspace, Filename: name + ".tdsx", Content: []byte("datasource-" + strings.TrimPrefix(luid, "ds-") + "-" + version), Metadata: metadata}
	}
	return WorkbookBundlePull{
		Workbook:    WorkbookPull{Workspace: workspace, Filename: "Finance.twbx", Content: []byte("workbook-" + version), Metadata: workbookMetadata},
		Datasources: []DatasourcePull{datasource("Sales", "ds-1"), datasource("Inventory", "ds-2")},
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != want {
		t.Fatalf("%s = %q, want %q", path, content, want)
	}
}

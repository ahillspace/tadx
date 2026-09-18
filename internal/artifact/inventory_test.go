package artifact_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/lock"
)

func TestInventoryReportsBoundedInvalidMetadataReason(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	created, err := artifact.NewWorkbookManager(time.Now).Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspaceRoot, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(created.ArtifactPath, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata["tableau_id"] = ""
	data, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := artifact.Inventory(context.Background(), workspaceRoot, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != artifact.StateInvalid || page.Items[0].Reason != "metadata_invalid" || page.Items[0].Path == "" || len(page.Warnings) != 1 || !strings.Contains(page.Warnings[0], "metadata failed validation") {
		t.Fatalf("Inventory() = %#v", page)
	}
	if strings.Contains(page.Warnings[0], workspaceRoot) {
		t.Fatalf("warning exposed workspace root: %q", page.Warnings[0])
	}
}

func TestInventoryReturnsRelativeBoundedArtifactState(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspaceRoot, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(created.CanonicalPath, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := artifact.Inventory(context.Background(), workspaceRoot, artifact.InventoryOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != "workbook" || page.Items[0].LUID != "wb-1" || page.Items[0].State != artifact.StateDirty {
		t.Fatalf("Inventory() = %#v", page)
	}
	if filepath.IsAbs(page.Items[0].Path) || strings.Contains(page.Items[0].Path, `\`) {
		t.Fatalf("inventory path = %q", page.Items[0].Path)
	}
}

func TestInventoryStateCountsCoverWholeWorkspaceAcrossPages(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	clean, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspaceRoot, Filename: "Alpha.twb", Content: []byte("remote"), Metadata: validMetadata("Alpha", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = clean
	dirty, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspaceRoot, Filename: "Beta.twb", Content: []byte("remote"), Metadata: validMetadata("Beta", "wb-2"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirty.CanonicalPath, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := artifact.Inventory(context.Background(), workspaceRoot, artifact.InventoryOptions{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	// Only one artifact is returned on this page, but the state counts and Total
	// must describe the whole workspace so the summary is not misleading.
	if page.Returned != 1 || page.Total != 2 {
		t.Fatalf("Inventory() returned=%d total=%d, want returned=1 total=2", page.Returned, page.Total)
	}
	if page.Clean != 1 || page.Dirty != 1 || page.Missing != 0 || page.Invalid != 0 {
		t.Fatalf("Inventory() counts clean=%d dirty=%d missing=%d invalid=%d, want clean=1 dirty=1", page.Clean, page.Dirty, page.Missing, page.Invalid)
	}
}

func TestInventoryRejectsUnboundedLimit(t *testing.T) {
	if _, err := artifact.Inventory(context.Background(), createWorkspace(t), artifact.InventoryOptions{Limit: 10001}); err == nil {
		t.Fatal("Inventory() accepted an unbounded limit")
	}
}

func TestInventoryReportsMissingCanonicalPayload(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	created, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspaceRoot, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(created.CanonicalPath); err != nil {
		t.Fatal(err)
	}
	page, err := artifact.Inventory(context.Background(), workspaceRoot, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != artifact.StateMissing || page.Missing != 1 {
		t.Fatalf("Inventory() = %#v", page)
	}
}

func TestInventoryRecognizesManagedFlowArtifact(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	_, err := artifact.NewFlowManager(time.Now).Pull(context.Background(), artifact.FlowPull{
		Workspace: workspaceRoot,
		Filename:  "Pipeline.tflx",
		Content:   []byte("flow"),
		Metadata: artifact.FlowMetadata{
			Name: "Pipeline", TableauID: "flow-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1",
			SourceEnvironment: "development", SourceSite: "test-site", SourceProjectName: "Ops", SourceProjectID: "project-1",
		},
		Lineage: artifact.LineageDocument{Complete: true, Direction: "both", Depth: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	page, err := artifact.Inventory(context.Background(), workspaceRoot, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].Kind != "flow" || page.Items[0].LUID != "flow-1" || page.Items[0].State != artifact.StateClean {
		t.Fatalf("Inventory() = %#v", page)
	}
}

func TestWorkbookManagerHonorsWorkspaceLock(t *testing.T) {
	workspaceRoot := createWorkspace(t)
	handle, err := lock.Acquire(filepath.Join(workspaceRoot, ".tadx.lock"))
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, pullErr := artifact.NewWorkbookManager(time.Now).Pull(context.Background(), artifact.WorkbookPull{Workspace: workspaceRoot, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1")})
		done <- pullErr
	}()
	select {
	case pullErr := <-done:
		_ = handle.Release()
		t.Fatalf("workbook pull mutated the workspace while another process held the lock: %v", pullErr)
	case <-time.After(300 * time.Millisecond):
		// Expected: the pull is serialized behind the advisory workspace lock.
	}
	if err := handle.Release(); err != nil {
		t.Fatal(err)
	}
	select {
	case pullErr := <-done:
		if pullErr != nil {
			t.Fatal(pullErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("workbook pull did not proceed after the workspace lock was released")
	}
}

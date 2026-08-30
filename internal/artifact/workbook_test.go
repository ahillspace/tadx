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
)

func TestWorkbookManagerWritesCanonicalArtifactAndProvenance(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(func() time.Time { return time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC) })
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twbx", Content: []byte("native-package"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BaselineFingerprint == "" || result.ArtifactPath == "" {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range []string{"Finance.twbx", "metadata.json", "view.md"} {
		if _, err := os.Stat(filepath.Join(result.ArtifactPath, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(result.ArtifactPath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["tableau_id"] != "wb-1" || metadata["local_baseline_fingerprint"] == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if _, exists := metadata["publish_target"]; exists {
		t.Fatalf("metadata persists publish target: %#v", metadata)
	}
}

func TestWorkbookManagerProtectsDirtyRepullAndOverwriteIsExplicit(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	first, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v1"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(first.ArtifactPath, "Finance.twb")
	if err := os.WriteFile(payload, []byte("local-edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v2"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty re-pull error = %v", err)
	}
	unchanged, _ := os.ReadFile(payload)
	if string(unchanged) != "local-edit" {
		t.Fatalf("dirty payload changed to %q", unchanged)
	}
	overwritten, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v2"), Overwrite: true,
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(overwritten.Warnings) != 1 {
		t.Fatalf("warnings = %#v", overwritten.Warnings)
	}
	updated, _ := os.ReadFile(payload)
	if string(updated) != "remote-v2" {
		t.Fatalf("payload = %q", updated)
	}
}

func TestWorkbookManagerCleanRepullWarnsAndReplaces(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v1"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v2"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "clean") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func createWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

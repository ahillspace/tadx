package artifact_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/artifact"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWorkbookArtifactCreationReturnsManagedPath(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Book.twb", Content: []byte("book"), Metadata: artifact.WorkbookMetadata{Name: "Book", TableauID: "wb", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "", SourceProjectName: "Ops", SourceProjectID: "project-1"}})
	if err != nil || result.ArtifactPath == "" {
		t.Fatalf("artifact result = %#v, error = %v", result, err)
	}
}

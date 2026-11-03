package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceClonePreservesUnresolvedFlowReplacementBeforeCreatingDestination(t *testing.T) {
	parent := t.TempDir()
	configPath := filepath.Join(parent, "config.yaml")
	manager := workspace.NewManager(configPath, nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
	if err != nil {
		t.Fatal(err)
	}
	content := []byte(`{"flow":"original"}`)
	payload := filepath.Join(parent, "flow.tfl")
	if err := os.WriteFile(payload, content, 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := artifact.NewFlowManager(nil).Pull(t.Context(), artifact.FlowPull{Workspace: source.Root, Filename: "flow.tfl", PayloadPath: payload, Metadata: artifact.FlowMetadata{Name: "Flow", TableauID: "flow-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "fixture", SourceSite: "fixture", SourceProjectName: "Project", SourceProjectID: "project-1"}, Lineage: artifact.LineageDocument{Complete: true, Direction: "both", Depth: 1}})
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(filepath.Dir(first.ArtifactPath), ".tadx-workbook-backup-interrupted-flow")
	if err := os.Rename(first.ArtifactPath, backup); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(parent, "clone")
	var output bytes.Buffer
	code := app.Run(t.Context(), []string{"workspace", "clone", "source", "--name", "clone", "--path", destination, "--full", "--json"}, &output, app.Options{ConfigPath: configPath})
	if code == 0 || !strings.Contains(output.String(), "unresolved artifact recovery evidence") {
		t.Fatalf("clone accepted an unresolved source replacement: code=%d output=%s", code, output.String())
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("failed source snapshot created a destination: %v", err)
	}
	current, err := config.Load(configPath)
	if err != nil || len(current.Workspaces) != 1 || current.Workspaces["source"].ID != source.ID {
		t.Fatalf("failed source snapshot changed registrations: %+v, %v", current.Workspaces, err)
	}
	bytes, err := os.ReadFile(filepath.Join(backup, "flow.tfl"))
	if err != nil || string(bytes) != string(content) {
		t.Fatalf("unresolved replacement evidence was changed: %s, %v", bytes, err)
	}
}

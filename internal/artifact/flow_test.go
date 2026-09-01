package artifact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFlowManagerPreservesNativePackageAndLineage(t *testing.T) {
	workspace := createFlowWorkspace(t)
	content := []byte("native\x00tflx")
	result, err := NewFlowManager(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }).Pull(context.Background(), FlowPull{
		Workspace: workspace, Filename: "Daily.tflx", Content: content,
		Metadata: FlowMetadata{Name: "Daily", TableauID: "f-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sandbox", SourceProjectName: "Department/Ops", SourceProjectID: "p-1", FileType: "tflx"},
		Lineage:  LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []LineageNode{{MetadataID: "m-1", Kind: "flow", RESTLUID: "f-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(result.CanonicalPath)
	if err != nil || string(got) != string(content) {
		t.Fatalf("payload = %q, error = %v", got, err)
	}
	if filepath.IsAbs(result.WorkspaceRelativePath) || strings.Contains(result.WorkspaceRelativePath, "\\") || result.LineagePath == "" {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(result.ArtifactPath, "lineage.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lineage LineageDocument
	if json.Unmarshal(data, &lineage) != nil || !lineage.Complete || len(lineage.Nodes) != 1 {
		t.Fatalf("lineage = %#v", lineage)
	}
}

func TestFlowManagerProtectsDirtyNativePackage(t *testing.T) {
	workspace := createFlowWorkspace(t)
	manager := NewFlowManager(time.Now)
	input := FlowPull{Workspace: workspace, Filename: "Daily.tfl", Content: []byte("v1"), Metadata: validFlowMetadata(), Lineage: LineageDocument{Complete: true, Direction: "both", Depth: 1}}
	first, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.CanonicalPath, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	input.Content = []byte("v2")
	if _, err := manager.Pull(context.Background(), input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("error = %v", err)
	}
}

func TestFlowManagerRejectsUnsupportedExtensionAndOversizedLineage(t *testing.T) {
	input := FlowPull{Workspace: createFlowWorkspace(t), Filename: "Daily.zip", Content: []byte("native"), Metadata: validFlowMetadata(), Lineage: LineageDocument{Complete: true, Direction: "both", Depth: 1}}
	if _, err := NewFlowManager(time.Now).Pull(context.Background(), input); err == nil {
		t.Fatal("expected extension error")
	}
	input.Filename = "Daily.tflx"
	input.Lineage.Nodes = make([]LineageNode, MaxLineageNodes+1)
	if _, err := NewFlowManager(time.Now).Pull(context.Background(), input); err == nil {
		t.Fatal("expected lineage bound error")
	}
}

func TestReadBoundedFileEnforcesStreamingLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lineage.json")
	if err := os.WriteFile(path, []byte("12345"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readBoundedFile(path, 5)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "12345" {
		t.Fatalf("content = %q", got)
	}

	if _, err := readBoundedFile(path, 4); err == nil || !strings.Contains(err.Error(), "exceeds its byte limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestReadBoundedFileRejectsNonRegularPaths(t *testing.T) {
	directory := t.TempDir()
	if _, err := readBoundedFile(directory, 64); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("directory error = %v", err)
	}

	target := filepath.Join(directory, "metadata.json")
	if err := os.WriteFile(target, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(directory, "metadata-link.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := readBoundedFile(link, 64); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("symbolic link error = %v", err)
	}
}

func createFlowWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func validFlowMetadata() FlowMetadata {
	return FlowMetadata{Name: "Daily", TableauID: "f-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceProjectName: "Ops", SourceProjectID: "p-1", FileType: "tfl"}
}

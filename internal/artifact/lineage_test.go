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

func TestLineageManagerWritesMetadataOnlyRelativeArtifact(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewLineageManager(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	result, err := manager.Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "flow", Name: "Daily", TableauID: "flow-1", MetadataID: "meta-flow", ProjectPath: "Department/Ops", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sandbox"},
		Lineage:   LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []LineageNode{{MetadataID: "meta-flow", Kind: "flow", RESTLUID: "flow-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(result.Path) || filepath.IsAbs(result.LineagePath) || strings.Contains(result.Path+result.LineagePath, "\\") {
		t.Fatalf("result paths = %#v", result)
	}
	absolute := filepath.Join(workspace, filepath.FromSlash(result.Path))
	for _, name := range []string{"metadata.json", "lineage.json", "view.md"} {
		if _, err := os.Stat(filepath.Join(absolute, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	entries, err := os.ReadDir(absolute)
	if err != nil || len(entries) != 3 {
		t.Fatalf("entries = %v, error = %v", entries, err)
	}
	data, err := os.ReadFile(filepath.Join(absolute, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata LineageMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Kind != "lineage" || metadata.LineagePath != "lineage.json" || metadata.Fingerprint == "" || metadata.NodeCount == nil || *metadata.NodeCount != 1 || !metadata.Complete {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestLineageManagerRejectsInvalidRootIdentityAndBounds(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := LineagePull{Workspace: workspace, Metadata: LineageMetadata{ResourceKind: "flow", Name: "Daily", TableauID: "flow-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"}, Lineage: LineageDocument{Direction: "both", Depth: 1}}
	invalid := valid
	invalid.Metadata.TableauID = ""
	if _, err := NewLineageManager(time.Now).Pull(context.Background(), invalid); err == nil {
		t.Fatal("expected identity error")
	}
	invalid = valid
	invalid.Lineage.Nodes = make([]LineageNode, MaxLineageNodes+1)
	if _, err := NewLineageManager(time.Now).Pull(context.Background(), invalid); err == nil {
		t.Fatal("expected bound error")
	}
}

func TestLineageManagerOmitsUnknownCountsForIncompleteCapture(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewLineageManager(time.Now).Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "workbook", Name: "Book", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"},
		Lineage:   LineageDocument{Complete: false, Direction: "upstream", Depth: 1, Warnings: []string{"Permission limited."}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.Path), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if _, exists := fields["node_count"]; exists {
		t.Fatalf("unknown node_count was persisted: %s", data)
	}
	if _, exists := fields["edge_count"]; exists {
		t.Fatalf("unknown edge_count was persisted: %s", data)
	}
}

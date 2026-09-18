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

func newLineageWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func pullSampleLineage(t *testing.T, workspace string) LineagePullResult {
	t.Helper()
	manager := NewLineageManager(func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) })
	result, err := manager.Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "flow", Name: "Daily", TableauID: "flow-1", MetadataID: "meta-flow", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"},
		Lineage:   LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []LineageNode{{MetadataID: "meta-flow", Kind: "flow", RESTLUID: "flow-1"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInventorySurfacesStandaloneLineageArtifact(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result := pullSampleLineage(t, workspace)

	page, err := Inventory(context.Background(), workspace, InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 {
		t.Fatalf("Inventory() items = %#v", page.Items)
	}
	item := page.Items[0]
	if item.Kind != "lineage" || item.LUID != "flow-1" || item.Name != "Daily" || item.State != StateClean {
		t.Fatalf("lineage item = %#v", item)
	}
	if item.ServerOrigin != "https://tableau.example.com" || item.SiteLUID != "site-1" {
		t.Fatalf("lineage source identity = %#v", item)
	}
	if item.Path != result.Path || item.CanonicalPath != result.LineagePath {
		t.Fatalf("lineage paths = %q / %q, want %q / %q", item.Path, item.CanonicalPath, result.Path, result.LineagePath)
	}
	if strings.Count(item.Path, "/") != 3 {
		t.Fatalf("lineage path is not four-segment: %q", item.Path)
	}
}

func TestInventoryMarksDirtyLineageArtifact(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result := pullSampleLineage(t, workspace)
	if err := os.WriteFile(filepath.Join(workspace, filepath.FromSlash(result.LineagePath)), []byte("local edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := Inventory(context.Background(), workspace, InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != StateDirty || page.Dirty != 1 {
		t.Fatalf("Inventory() = %#v", page)
	}
}

func TestResolveMatchesLineageByPathAndByKindLUID(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result := pullSampleLineage(t, workspace)

	byPath, err := Resolve(context.Background(), workspace, Selector{Path: result.Path})
	if err != nil {
		t.Fatal(err)
	}
	if byPath.Kind != "lineage" || byPath.LUID != "flow-1" {
		t.Fatalf("Resolve(path) = %#v", byPath)
	}
	byIdentity, err := Resolve(context.Background(), workspace, Selector{Kind: "lineage", LUID: "flow-1"})
	if err != nil {
		t.Fatal(err)
	}
	if byIdentity.Path != result.Path {
		t.Fatalf("Resolve(kind+LUID) = %#v", byIdentity)
	}
}

func TestDeleteRemovesStandaloneLineageArtifact(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result := pullSampleLineage(t, workspace)
	item, err := Resolve(context.Background(), workspace, Selector{Path: result.Path})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := Delete(context.Background(), DeleteRequest{Workspace: workspace, Expected: item})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Kind != "lineage" || deleted.LUID != "flow-1" {
		t.Fatalf("Delete() = %#v", deleted)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(result.Path))); !os.IsNotExist(err) {
		t.Fatalf("lineage artifact still exists: %v", err)
	}
}

func TestMoveRelocatesStandaloneLineageArtifact(t *testing.T) {
	source := newLineageWorkspace(t)
	destination := newLineageWorkspace(t)
	result := pullSampleLineage(t, source)

	moved, err := Move(context.Background(), MoveRequest{
		SourceWorkspace: source, DestinationWorkspace: destination, Selector: Selector{Kind: "lineage", LUID: "flow-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if moved.Kind != "lineage" || moved.LUID != "flow-1" || moved.Path != result.Path {
		t.Fatalf("Move() = %#v", moved)
	}
	if _, err := os.Stat(filepath.Join(source, filepath.FromSlash(result.Path))); !os.IsNotExist(err) {
		t.Fatalf("source lineage artifact still exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination, filepath.FromSlash(result.LineagePath))); err != nil {
		t.Fatalf("destination lineage payload missing: %v", err)
	}
}

func TestLineageManagerSerializesEmptyGraphAsArrays(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result, err := NewLineageManager(time.Now).Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "workbook", Name: "Book", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"},
		Lineage:   LineageDocument{Complete: false, Direction: "upstream", Depth: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.LineagePath)))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if string(fields["nodes"]) != "[]" || string(fields["edges"]) != "[]" {
		t.Fatalf("empty graph did not serialize as arrays: %s", data)
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

func TestLineageManagerOmitsCountsForPartialCaptureWithConfirmedMembers(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := NewLineageManager(time.Now).Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "workbook", Name: "Book", TableauID: "wb-1", MetadataID: "meta-wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"},
		Lineage: LineageDocument{Complete: false, Direction: "upstream", Depth: 1, Nodes: []LineageNode{
			{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1"},
			{MetadataID: "meta-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"},
		}, Edges: []LineageEdge{{FromMetadataID: "meta-ds-1", ToMetadataID: "meta-wb-1", Relationship: "upstream"}}, Warnings: []string{"provider relation failure"}},
		CountsKnown: false,
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
		t.Fatalf("partial node_count was persisted: %s", data)
	}
	if _, exists := fields["edge_count"]; exists {
		t.Fatalf("partial edge_count was persisted: %s", data)
	}
}

func TestLineageManagerPersistsSanitizedFailureContext(t *testing.T) {
	workspace := newLineageWorkspace(t)
	result, err := NewLineageManager(time.Now).Pull(context.Background(), LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "workbook", Name: "Book", TableauID: "wb-1", MetadataID: "meta-wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "dev"},
		Lineage: LineageDocument{Complete: false, Direction: "upstream", Depth: 1, Failure: &LineageFailure{
			Provider: "tableau-metadata", Relation: "upstreamDatabasesConnection", RootKind: "workbook", RootRESTLUID: "wb-1", RequestID: "request-3",
		}, Nodes: []LineageNode{{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1"}}, Warnings: []string{"relationship unavailable"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.LineagePath)))
	if err != nil {
		t.Fatal(err)
	}
	var document LineageDocument
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document.Failure == nil || document.Failure.Provider != "tableau-metadata" || document.Failure.Relation != "upstreamDatabasesConnection" || document.Failure.RootRESTLUID != "wb-1" || document.Failure.RequestID != "request-3" {
		t.Fatalf("failure = %#v", document.Failure)
	}
}

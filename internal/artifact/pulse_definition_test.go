package artifact_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
)

func TestPulseDefinitionManagerWritesCanonicalManagedArtifact(t *testing.T) {
	workspace := pulseWorkspace(t)
	manager := artifact.NewPulseDefinitionManager(func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) })
	result, err := manager.Pull(context.Background(), artifact.PulseDefinitionPull{
		Workspace: workspace, Configuration: []byte(`{"specification":{"datasource":{"id":"datasource-1"}},"metadata":{"name":"Revenue","id":"definition-1"}}`),
		Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test/", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sales"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if filepath.IsAbs(result.WorkspaceRelativePath) || !strings.HasPrefix(result.WorkspaceRelativePath, "artifacts/pulse-definition/") || result.CanonicalPath != result.WorkspaceRelativePath+"/resource.json" {
		t.Fatalf("result=%#v", result)
	}
	resource, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(result.CanonicalPath)))
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"metadata\": {\n    \"id\": \"definition-1\",\n    \"name\": \"Revenue\"\n  },\n  \"specification\": {\n    \"datasource\": {\n      \"id\": \"datasource-1\"\n    }\n  }\n}\n"
	if string(resource) != want {
		t.Fatalf("resource:\n%s\nwant:\n%s", resource, want)
	}
	loaded, err := manager.Read(context.Background(), filepath.Join(workspace, filepath.FromSlash(result.WorkspaceRelativePath)))
	if err != nil || loaded.Metadata.TableauID != "definition-1" || loaded.Fingerprint != result.BaselineFingerprint {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestPulseDefinitionManagerProtectsDirtyArtifact(t *testing.T) {
	workspace := pulseWorkspace(t)
	manager := artifact.NewPulseDefinitionManager(nil)
	input := artifact.PulseDefinitionPull{
		Workspace: workspace, Configuration: []byte(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`),
		Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sales"},
	}
	first, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	resource := filepath.Join(workspace, filepath.FromSlash(first.CanonicalPath))
	if err := os.WriteFile(resource, []byte(`{"edited":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Pull(context.Background(), input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("error=%v", err)
	}
	input.Overwrite = true
	second, err := manager.Pull(context.Background(), input)
	if err != nil || len(second.Warnings) != 1 {
		t.Fatalf("result=%#v err=%v", second, err)
	}
}

func TestPulseDefinitionManagerRejectsIdentityMismatch(t *testing.T) {
	workspace := pulseWorkspace(t)
	_, err := artifact.NewPulseDefinitionManager(nil).Pull(context.Background(), artifact.PulseDefinitionPull{
		Workspace: workspace, Configuration: []byte(`{"metadata":{"id":"other","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`),
		Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sales"},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("error=%v", err)
	}
}

func TestPulseDefinitionManagerRejectsTrailingJSON(t *testing.T) {
	workspace := pulseWorkspace(t)
	_, err := artifact.NewPulseDefinitionManager(nil).Pull(context.Background(), artifact.PulseDefinitionPull{
		Workspace: workspace, Configuration: []byte(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}} true`),
		Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sales"},
	})
	if err == nil || !strings.Contains(err.Error(), "exactly one JSON object") {
		t.Fatalf("error=%v", err)
	}
}

func TestInventorySurfacesPulseDefinitionArtifact(t *testing.T) {
	workspace := pulseWorkspace(t)
	result := pullSamplePulseDefinition(t, workspace)

	page, err := artifact.Inventory(context.Background(), workspace, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Clean != 1 || page.Total != 1 {
		t.Fatalf("Inventory() = %#v", page)
	}
	item := page.Items[0]
	if item.Kind != "pulse-definition" || item.LUID != "definition-1" || item.Name != "Revenue" || item.State != artifact.StateClean {
		t.Fatalf("Pulse definition item = %#v", item)
	}
	if item.Path != result.WorkspaceRelativePath || item.CanonicalPath != result.CanonicalPath || item.ServerOrigin != "https://tableau.example.test" || item.SiteLUID != "site-1" {
		t.Fatalf("Pulse definition paths or provenance = %#v", item)
	}
}

func TestResolveMatchesPulseDefinitionByPathAndIdentity(t *testing.T) {
	workspace := pulseWorkspace(t)
	result := pullSamplePulseDefinition(t, workspace)

	byPath, err := artifact.Resolve(context.Background(), workspace, artifact.Selector{Path: result.WorkspaceRelativePath})
	if err != nil {
		t.Fatal(err)
	}
	byIdentity, err := artifact.Resolve(context.Background(), workspace, artifact.Selector{Kind: "pulse-definition", LUID: "definition-1"})
	if err != nil {
		t.Fatal(err)
	}
	if byPath.Kind != "pulse-definition" || byIdentity.Path != result.WorkspaceRelativePath || byPath.TreeFingerprint != byIdentity.TreeFingerprint {
		t.Fatalf("Resolve(path) = %#v, Resolve(identity) = %#v", byPath, byIdentity)
	}
	if _, err := artifact.Resolve(context.Background(), workspace, artifact.Selector{Path: result.WorkspaceRelativePath, Kind: "workbook"}); err == nil {
		t.Fatal("Resolve() accepted a Pulse definition path with a mismatched kind")
	}
	if _, err := artifact.Resolve(context.Background(), workspace, artifact.Selector{Path: "artifacts/unknown/item"}); err == nil {
		t.Fatal("Resolve() accepted an unknown managed artifact root")
	}
}

func TestInventoryTracksPulseDefinitionPayloadState(t *testing.T) {
	workspace := pulseWorkspace(t)
	result := pullSamplePulseDefinition(t, workspace)
	payload := filepath.Join(workspace, filepath.FromSlash(result.CanonicalPath))
	if err := os.WriteFile(payload, []byte("local edit\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	page, err := artifact.Inventory(context.Background(), workspace, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != artifact.StateDirty || page.Dirty != 1 {
		t.Fatalf("dirty Inventory() = %#v", page)
	}
	if err := os.Remove(payload); err != nil {
		t.Fatal(err)
	}
	page, err = artifact.Inventory(context.Background(), workspace, artifact.InventoryOptions{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 1 || page.Items[0].State != artifact.StateMissing || page.Missing != 1 {
		t.Fatalf("missing Inventory() = %#v", page)
	}
}

func TestDeleteRemovesExactPulseDefinitionArtifact(t *testing.T) {
	workspace := pulseWorkspace(t)
	result := pullSamplePulseDefinition(t, workspace)
	item, err := artifact.Resolve(context.Background(), workspace, artifact.Selector{Kind: "pulse-definition", LUID: "definition-1"})
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := artifact.Delete(context.Background(), artifact.DeleteRequest{Workspace: workspace, Expected: item})
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Kind != "pulse-definition" || deleted.LUID != "definition-1" {
		t.Fatalf("Delete() = %#v", deleted)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(result.WorkspaceRelativePath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Pulse definition artifact still exists: %v", err)
	}
}

func pullSamplePulseDefinition(t *testing.T, workspace string) artifact.PulseDefinitionPullResult {
	t.Helper()
	result, err := artifact.NewPulseDefinitionManager(func() time.Time { return time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC) }).Pull(context.Background(), artifact.PulseDefinitionPull{
		Workspace: workspace, Configuration: []byte(`{"specification":{"datasource":{"id":"datasource-1"}},"metadata":{"name":"Revenue","id":"definition-1"}}`),
		Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test/", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sales"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func pulseWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

package artifact_test

import (
	"context"
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

func pulseWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

package artifact

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestAcquisitionPreviewPreservesMissingRootsAndDirtyGuards(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "lineage", "pulse-definition"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			root := newLineageWorkspace(t)
			input := PullPreview{Workspace: root, Kind: kind, ResourceKind: "flow", Name: "Example", LUID: "item-1", ServerOrigin: "https://tableau.example.test", SiteLUID: "site-1"}
			before := previewTree(t, root)
			plan, err := PreviewPull(ctx, input)
			if err != nil || plan.Exists || !strings.HasPrefix(plan.Path, "artifacts/"+kind+"/") {
				t.Fatalf("plan=%#v error=%v", plan, err)
			}
			if after := previewTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("preview created an artifact root")
			}
			canonical := createPreviewArtifact(t, input)
			before = previewTree(t, root)
			// A renamed source must still resolve its previously acquired identity.
			if kind != "lineage" {
				input.Name = "Renamed"
			}
			existing, err := PreviewPull(ctx, input)
			if err != nil || !existing.Exists || existing.Path != plan.Path {
				t.Fatalf("existing=%#v error=%v", existing, err)
			}
			if after := previewTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("preview changed a clean artifact")
			}
			if err := os.WriteFile(canonical, []byte("local edit"), 0o600); err != nil {
				t.Fatal(err)
			}
			before = previewTree(t, root)
			if _, err := PreviewPull(ctx, input); err == nil || !strings.Contains(err.Error(), "dirty") {
				t.Fatalf("dirty target accepted: %v", err)
			}
			input.Overwrite = true
			if _, err := PreviewPull(ctx, input); err != nil {
				t.Fatal(err)
			}
			if after := previewTree(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("preview modified a dirty artifact")
			}
		})
	}
}

func TestAcquisitionPreviewRejectsUnmanagedCollision(t *testing.T) {
	root := newLineageWorkspace(t)
	input := PullPreview{Workspace: root, Kind: "workbook", Name: "Example", LUID: "item-1", ServerOrigin: "https://tableau.example.test", SiteLUID: "site-1", Overwrite: true}
	plan, err := PreviewPull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(plan.Path)), 0o700); err != nil {
		t.Fatal(err)
	}
	before := previewTree(t, root)
	if _, err := PreviewPull(context.Background(), input); err == nil {
		t.Fatal("unmanaged target accepted with overwrite")
	}
	if after := previewTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("collision preview wrote files")
	}
}

func TestAcquisitionPreviewProtectsPulseBundleSeparatelyFromDefinition(t *testing.T) {
	root := newLineageWorkspace(t)
	definition := json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
	bundle := PulseBundle{Version: 1, SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", DefinitionLUID: "definition-1", DatasourceReferences: []string{"datasource-1"}, Definition: definition, Metrics: []PulseBundleMetric{{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: json.RawMessage(`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","last_n":17},"filters":[]}`)}}}
	data, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewPulseDefinitionManager(nil).Pull(context.Background(), PulseDefinitionPull{Workspace: root, Configuration: definition, Bundle: data, Metadata: PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: bundle.SourceServerOrigin, SourceSiteLUID: bundle.SourceSiteLUID, SourceEnvironment: "dev", SourceSite: "test"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(result.WorkspaceRelativePath), "bundle.json")
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), `"last_n":17`, `"last_n":19`)), 0o600); err != nil {
		t.Fatal(err)
	}
	before := previewTree(t, root)
	input := PullPreview{Workspace: root, Kind: "pulse-definition", Name: "Revenue", LUID: "definition-1", ServerOrigin: bundle.SourceServerOrigin, SiteLUID: bundle.SourceSiteLUID}
	if _, err := PreviewPull(context.Background(), input); err == nil || !strings.Contains(err.Error(), "bundle is dirty") {
		t.Fatalf("dirty bundle accepted: %v", err)
	}
	input.Overwrite = true
	if _, err := PreviewPull(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	if after := previewTree(t, root); !reflect.DeepEqual(before, after) {
		t.Fatal("preview replaced Pulse bundle")
	}
}

func createPreviewArtifact(t *testing.T, input PullPreview) string {
	t.Helper()
	ctx := context.Background()
	switch input.Kind {
	case "workbook":
		result, err := NewWorkbookManager(nil).Pull(ctx, WorkbookPull{Workspace: input.Workspace, Filename: "Example.twb", Content: []byte("<workbook/>"), Metadata: WorkbookMetadata{Name: input.Name, TableauID: input.LUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: "dev", SourceSite: "test", SourceProjectName: "Shared", SourceProjectID: "project-1"}})
		if err != nil {
			t.Fatal(err)
		}
		return result.CanonicalPath
	case "datasource":
		result, err := NewDatasourceManager(nil).Pull(ctx, DatasourcePull{Workspace: input.Workspace, Filename: "Example.tds", Content: []byte("<datasource/>"), Metadata: DatasourceMetadata{Name: input.Name, TableauID: input.LUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: "dev", SourceSite: "test", SourceProjectName: "Shared", SourceProjectID: "project-1"}, Lineage: LineageDocument{Direction: "both", Depth: 1}})
		if err != nil {
			t.Fatal(err)
		}
		return result.CanonicalPath
	case "flow":
		result, err := NewFlowManager(nil).Pull(ctx, FlowPull{Workspace: input.Workspace, Filename: "Example.tfl", Content: []byte("native"), Metadata: FlowMetadata{Name: input.Name, TableauID: input.LUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: "dev", SourceSite: "test", SourceProjectName: "Shared", SourceProjectID: "project-1", FileType: "tfl"}, Lineage: LineageDocument{Direction: "both", Depth: 1}})
		if err != nil {
			t.Fatal(err)
		}
		return result.CanonicalPath
	case "lineage":
		result, err := NewLineageManager(nil).Pull(ctx, LineagePull{Workspace: input.Workspace, Metadata: LineageMetadata{ResourceKind: input.ResourceKind, Name: input.Name, TableauID: input.LUID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: "dev"}, Lineage: LineageDocument{Direction: "both", Depth: 1}})
		if err != nil {
			t.Fatal(err)
		}
		return filepath.Join(input.Workspace, filepath.FromSlash(result.LineagePath))
	case "pulse-definition":
		result, err := NewPulseDefinitionManager(nil).Pull(ctx, PulseDefinitionPull{Workspace: input.Workspace, Configuration: []byte(`{"metadata":{"id":"item-1","name":"Example"},"specification":{"datasource":{"id":"datasource-1"}}}`), Metadata: PulseDefinitionMetadata{Name: input.Name, TableauID: input.LUID, DatasourceLUID: "datasource-1", SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: "dev", SourceSite: "test"}})
		if err != nil {
			t.Fatal(err)
		}
		return filepath.Join(input.Workspace, filepath.FromSlash(result.CanonicalPath))
	}
	t.Fatal("unsupported fixture kind")
	return ""
}

func previewTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[rel] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

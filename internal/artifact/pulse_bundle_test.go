package artifact_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
)

func pulseBundleFixture() artifact.PulseBundle {
	return artifact.PulseBundle{Version: 1, SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", DefinitionLUID: "definition-1", DatasourceReferences: []string{"datasource-1"}, Definition: json.RawMessage(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`), Metrics: []artifact.PulseBundleMetric{{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: json.RawMessage(`{"measurement_period":{"granularity":"GRANULARITY_BY_DAY","last_n":17},"filters":[]}`)}}}
}

func TestPulseBundleManagedDirtyProtectionAndCompleteRead(t *testing.T) {
	ctx := context.Background()
	workspace := pulseWorkspace(t)
	bundle := pulseBundleFixture()
	data, _ := json.Marshal(bundle)
	input := artifact.PulseDefinitionPull{Workspace: workspace, Configuration: bundle.Definition, Bundle: data, Metadata: artifact.PulseDefinitionMetadata{Name: "Revenue", TableauID: bundle.DefinitionLUID, DatasourceLUID: "datasource-1", SourceServerOrigin: bundle.SourceServerOrigin, SourceSiteLUID: bundle.SourceSiteLUID, SourceEnvironment: "test", SourceSite: "test"}}
	manager := artifact.NewPulseDefinitionManager(nil)
	result, err := manager.Pull(ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(workspace, filepath.FromSlash(result.WorkspaceRelativePath))
	loaded, err := artifact.ReadPulseBundle(ctx, path)
	if err != nil || len(loaded.Metrics) != 1 {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
	page, err := artifact.Inventory(ctx, workspace, artifact.InventoryOptions{Limit: 20})
	if err != nil || page.Clean != 1 {
		t.Fatalf("inventory=%#v err=%v", page, err)
	}
	found := false
	for _, managed := range page.ManagedPaths {
		if strings.HasSuffix(managed, "/bundle.json") {
			found = true
		}
	}
	if !found {
		t.Fatal("bundle not recognized as managed")
	}
	dirty := bytes.Replace(data, []byte(`"last_n":17`), []byte(`"last_n":19`), 1)
	if err := os.WriteFile(filepath.Join(path, "bundle.json"), dirty, 0600); err != nil {
		t.Fatal(err)
	}
	page, err = artifact.Inventory(ctx, workspace, artifact.InventoryOptions{Limit: 20})
	if err != nil || page.Dirty != 1 {
		t.Fatalf("dirty=%#v err=%v", page, err)
	}
	if _, err := manager.Pull(ctx, input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("overwrite protection=%v", err)
	}
	before, _ := os.ReadFile(filepath.Join(path, "bundle.json"))
	if !bytes.Equal(before, dirty) {
		t.Fatal("failed pull replaced edited bundle")
	}
	input.Overwrite = true
	if result, err := manager.Pull(ctx, input); err != nil || len(result.Warnings) == 0 {
		t.Fatalf("overwrite=%#v err=%v", result, err)
	}
	input.Bundle = nil
	if _, err := manager.Pull(ctx, input); err == nil {
		t.Fatal("accepted downgrade to snapshot only")
	}
}

func TestPulseBundleRejectsVersionIdentityIncompleteAndOversizedDocuments(t *testing.T) {
	for _, change := range []func(*artifact.PulseBundle){func(b *artifact.PulseBundle) { b.Version = 2 }, func(b *artifact.PulseBundle) { b.DefinitionLUID = "other" }, func(b *artifact.PulseBundle) { b.DatasourceReferences = []string{"other"} }, func(b *artifact.PulseBundle) { b.Metrics = nil }, func(b *artifact.PulseBundle) { b.Metrics = append(b.Metrics, b.Metrics[0]) }, func(b *artifact.PulseBundle) { b.Metrics[0].Specification = json.RawMessage(`{"filters":[]}`) }} {
		bundle := pulseBundleFixture()
		change(&bundle)
		data, _ := json.Marshal(bundle)
		if _, err := artifact.DecodePulseBundle(data); err == nil {
			t.Fatalf("accepted invalid bundle %s", data)
		}
	}
	if _, err := artifact.DecodePulseBundle(make([]byte, artifact.MaxPulseBundleBytes+1)); err == nil {
		t.Fatal("accepted oversized bundle")
	}
	data, _ := json.Marshal(pulseBundleFixture())
	if _, err := artifact.DecodePulseBundle(append(data, []byte(` {}`)...)); err == nil {
		t.Fatal("accepted trailing document")
	}
}

package artifact

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
)

func TestPulsePullRejectsDestinationMovedAfterPreparation(t *testing.T) {
	workspace := newLineageWorkspace(t)
	manager := NewPulseDefinitionManager(nil)
	input := PulseDefinitionPull{
		Workspace:     workspace,
		Configuration: []byte(`{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`),
		Metadata:      PulseDefinitionMetadata{Name: "Revenue", TableauID: "definition-1", DatasourceLUID: "datasource-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "test"},
	}
	first, err := manager.Pull(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedPath = first.WorkspaceRelativePath
	original := filepath.Join(workspace, filepath.FromSlash(first.WorkspaceRelativePath))
	moved := original + "-moved"
	if err := os.Rename(original, moved); err != nil {
		t.Fatal(err)
	}
	metadata, err := os.ReadFile(filepath.Join(moved, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Pull(t.Context(), input)
	requirePullDestinationConflict(t, err)
	after, err := os.ReadFile(filepath.Join(moved, "metadata.json"))
	if err != nil || string(metadata) != string(after) {
		t.Fatalf("moved artifact changed: err=%v", err)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Fatalf("original path recreated: %v", err)
	}
}

func TestLineagePullRejectsRenamedDestinationAfterPreparation(t *testing.T) {
	workspace := newLineageWorkspace(t)
	input := LineagePull{
		Workspace: workspace,
		Metadata:  LineageMetadata{ResourceKind: "flow", Name: "Original", TableauID: "flow-1", MetadataID: "flow-meta", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "test", SourceSite: "test"},
		Lineage:   LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []LineageNode{{MetadataID: "flow-meta", Kind: "flow", RESTLUID: "flow-1"}}},
	}
	manager := NewLineageManager(nil)
	first, err := manager.Pull(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedPath = first.Path
	input.Metadata.Name = "Renamed"
	_, err = manager.Pull(t.Context(), input)
	requirePullDestinationConflict(t, err)
	paths, err := filepath.Glob(filepath.Join(workspace, "artifacts", "lineage", "flow", "*"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("paths=%v err=%v", paths, err)
	}
}

func requirePullDestinationConflict(t *testing.T, err error) {
	t.Helper()
	failure, ok := errors.AsType[*errs.Error](err)
	if !ok || !strings.Contains(failure.Summary, "destination changed") || failure.Phase != errs.PhaseValidation || failure.Outcome != errs.OutcomeNotAttempted || failure.Retryable == nil || *failure.Retryable || failure.CorrectiveAction == "" {
		t.Fatalf("destination conflict must preserve a nonretryable validation/not_attempted outcome: %v", err)
	}
}

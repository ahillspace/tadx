package artifact

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func registeredArtifactSelection(t *testing.T) (workspacecore.Record, workspacecore.Record, Item) {
	t.Helper()
	base := t.TempDir()
	manager := workspacecore.NewManager(filepath.Join(base, "config.yaml"), nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(base, "source"))
	if err != nil {
		t.Fatal(err)
	}
	destination, err := manager.Create(t.Context(), "destination", filepath.Join(base, "destination"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewWorkbookManager(nil).Pull(t.Context(), WorkbookPull{Workspace: source.Root, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("<workbook/>")), Metadata: moveTestMetadata()}); err != nil {
		t.Fatal(err)
	}
	selected, err := Resolve(t.Context(), source.Root, Selector{Kind: "workbook", LUID: "wb-1", ExpectedWorkspaceID: source.ID})
	if err != nil {
		t.Fatal(err)
	}
	return source, destination, selected
}

func TestMoveRejectsPinnedArtifactReplacedByAnotherSource(t *testing.T) {
	source, destination, selected := registeredArtifactSelection(t)
	directory := filepath.Join(source.Root, filepath.FromSlash(selected.Path))
	metadata, err := readMetadata(directory)
	if err != nil {
		t.Fatal(err)
	}
	metadata.SourceSiteLUID = "another-site"
	encoded, err := encodeWorkbookMetadata(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "metadata.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	request := MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, DestinationWorkspaceID: destination.ID, Selector: Selector{Path: selected.Path, Kind: selected.Kind, LUID: selected.LUID, ExpectedWorkspaceID: source.ID, Expected: &selected}}
	if _, err := Move(t.Context(), request); err == nil || !strings.Contains(err.Error(), "source identity changed") {
		t.Fatalf("move retargeted the prepared source: %v", err)
	}
	if _, err := os.Stat(filepath.Join(source.Root, filepath.FromSlash(selected.CanonicalPath))); err != nil {
		t.Fatalf("replacement source changed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destination.Root, filepath.FromSlash(selected.Path))); !os.IsNotExist(err) {
		t.Fatalf("rejected move created a destination artifact: %v", err)
	}
}

func TestArtifactMutationRejectsPinnedWorkspaceReplacement(t *testing.T) {
	for _, operation := range []string{"move", "delete"} {
		t.Run(operation, func(t *testing.T) {
			source, destination, selected := registeredArtifactSelection(t)
			manifestPath := filepath.Join(source.Root, "tadx.yaml")
			manifest, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(manifestPath, []byte(strings.Replace(string(manifest), source.ID, "replacement-workspace", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			if operation == "move" {
				_, err = Move(t.Context(), MoveRequest{SourceWorkspace: source.Root, DestinationWorkspace: destination.Root, DestinationWorkspaceID: destination.ID, Selector: Selector{Path: selected.Path, Kind: selected.Kind, LUID: selected.LUID, ExpectedWorkspaceID: source.ID, Expected: &selected}})
			} else {
				_, err = Delete(t.Context(), DeleteRequest{Workspace: source.Root, ExpectedWorkspaceID: source.ID, Expected: selected})
			}
			if err == nil || !strings.Contains(err.Error(), "workspace identity changed") {
				t.Fatalf("%s retargeted a replaced workspace: %v", operation, err)
			}
			if _, err := os.Stat(filepath.Join(source.Root, filepath.FromSlash(selected.CanonicalPath))); err != nil {
				t.Fatalf("replacement workspace artifact changed: %v", err)
			}
		})
	}
}

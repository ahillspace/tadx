package app_test

import (
	"bytes"
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceCloneRejectsDeleteRecoveryEntryThroughCLI(t *testing.T) {
	parent := t.TempDir()
	configPath := filepath.Join(parent, "config.yaml")
	manager := workspace.NewManager(configPath, nil)
	source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(parent, "Workbook.twb")
	if err := os.WriteFile(payload, []byte(`<workbook generation="retained-delete"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	created, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{Workspace: source.Root, Filename: "Workbook.twb", PayloadPath: payload, Metadata: artifact.WorkbookMetadata{Name: "Workbook", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "fixture", SourceSite: "fixture", SourceProjectName: "Project", SourceProjectID: "project-1"}})
	if err != nil {
		t.Fatal(err)
	}
	// Artifact tests exercise both real Delete branches that can retain this
	// directory. Clone must not infer their outcome from the shared prefix.
	retained := filepath.Join(filepath.Dir(created.ArtifactPath), ".tadx-delete-fixture")
	if err := os.Rename(created.ArtifactPath, retained); err != nil {
		t.Fatal(err)
	}
	viewPath := filepath.Join(retained, "view.md")
	view, err := os.ReadFile(viewPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(viewPath, append(view, []byte("\nRetained deletion-stage edit.\n")...), 0o600); err != nil {
		t.Fatal(err)
	}
	before := acquisitionSnapshot(t, retained)
	destination := filepath.Join(parent, "clone")
	var output bytes.Buffer
	code := app.Run(t.Context(), []string{"workspace", "clone", "source", "--name", "clone", "--path", destination, "--full", "--json"}, &output, app.Options{ConfigPath: configPath})
	var document struct {
		Error struct {
			ID               string `json:"id"`
			UpstreamCause    string `json:"upstream_cause"`
			CorrectiveAction string `json:"corrective_action"`
			Phase            string `json:"phase"`
			Outcome          string `json:"outcome"`
			Retryable        *bool  `json:"retryable"`
		}
	}
	if err := json.Unmarshal(output.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if code == 0 || document.Error.ID != "workspace.clone.failed" || !strings.Contains(document.Error.UpstreamCause, "unresolved artifact recovery evidence") || !strings.Contains(document.Error.UpstreamCause, "artifacts/workbook/.tadx-delete-fixture") || document.Error.Phase != "validation" || document.Error.Outcome != "not_attempted" || document.Error.Retryable == nil || *document.Error.Retryable {
		t.Fatalf("clone did not reject ambiguous Delete recovery evidence: code=%d output=%s", code, &output)
	}
	for _, guidance := range []string{"Preserve the deletion recovery entry", "confirm whether deletion completed or was rejected", "do not infer the outcome from its name"} {
		if !strings.Contains(document.Error.CorrectiveAction, guidance) {
			t.Errorf("recovery guidance omitted %q: %s", guidance, document.Error.CorrectiveAction)
		}
	}
	if _, err := os.Lstat(destination); !os.IsNotExist(err) {
		t.Fatalf("rejected Delete snapshot created a destination: %v", err)
	}
	current, err := config.Load(configPath)
	if err != nil || len(current.Workspaces) != 1 || current.Workspaces["source"].ID != source.ID {
		t.Fatalf("rejected Delete snapshot changed registrations: %+v, %v", current.Workspaces, err)
	}
	if after := acquisitionSnapshot(t, retained); !maps.Equal(before, after) {
		t.Fatal("clone changed retained Delete recovery contents")
	}
}

package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/workspace"
)

func TestWorkspaceCloneRejectsMoveRecoveryEntriesThroughCLI(t *testing.T) {
	for _, prefix := range []string{".tadx-move-source-", ".tadx-move-stage-"} {
		t.Run(prefix, func(t *testing.T) {
			parent := t.TempDir()
			configPath := filepath.Join(parent, "config.yaml")
			manager := workspace.NewManager(configPath, nil)
			source, err := manager.Create(t.Context(), "source", filepath.Join(parent, "source"))
			if err != nil {
				t.Fatal(err)
			}
			payload := filepath.Join(parent, "Workbook.twb")
			if err := os.WriteFile(payload, []byte(`<workbook generation="retained-move"/>`), 0o600); err != nil {
				t.Fatal(err)
			}
			created, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{Workspace: source.Root, Filename: "Workbook.twb", PayloadPath: payload, Metadata: artifact.WorkbookMetadata{Name: "Workbook", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "fixture", SourceSite: "fixture", SourceProjectName: "Project", SourceProjectID: "project-1"}})
			if err != nil {
				t.Fatal(err)
			}
			// The artifact package's real Move failure/crash tests establish both
			// retained states; exercise the complete CLI snapshot boundary here.
			retained := filepath.Join(filepath.Dir(created.ArtifactPath), prefix+"fixture")
			if err := os.Rename(created.ArtifactPath, retained); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			for _, name := range []string{"Workbook.twb", "metadata.json", "view.md"} {
				before[name], err = os.ReadFile(filepath.Join(retained, name))
				if err != nil {
					t.Fatal(err)
				}
			}
			destination := filepath.Join(parent, "clone")
			var output bytes.Buffer
			code := app.Run(t.Context(), []string{"workspace", "clone", "source", "--name", "clone", "--path", destination, "--full", "--json"}, &output, app.Options{ConfigPath: configPath})
			if code == 0 || !strings.Contains(output.String(), "unresolved artifact recovery evidence") || !strings.Contains(output.String(), prefix+"fixture") || !strings.Contains(output.String(), "source and destination workspaces") {
				t.Fatalf("clone did not reject Move recovery evidence with safe guidance: code=%d output=%s", code, output.String())
			}
			if _, err := os.Lstat(destination); !os.IsNotExist(err) {
				t.Fatalf("rejected Move snapshot created a clone: %v", err)
			}
			current, err := config.Load(configPath)
			if err != nil || len(current.Workspaces) != 1 || current.Workspaces["source"].ID != source.ID {
				t.Fatalf("rejected Move snapshot changed registrations: %+v, %v", current.Workspaces, err)
			}
			for name, expected := range before {
				after, err := os.ReadFile(filepath.Join(retained, name))
				if err != nil || !bytes.Equal(after, expected) {
					t.Fatalf("clone recovery changed retained Move content %q: %v", name, err)
				}
			}
		})
	}
}

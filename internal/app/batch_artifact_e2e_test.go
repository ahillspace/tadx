package app_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestLocalArtifactBatchesRejectPathAndIdentityAliasesBeforeChanges(t *testing.T) {
	for _, operation := range []string{"delete", "move"} {
		t.Run(operation, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "source")
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				code := app.Run(t.Context(), args, &output, options)
				return code, output.String()
			}
			for _, name := range []string{"source", "first", "second"} {
				if code, output := run("workspace", "create", name, "--path", filepath.Join(directory, name)); code != 0 {
					t.Fatalf("create: %s", output)
				}
			}
			pulled, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{
				Workspace: root, Filename: "Workbook.twb", PayloadPath: app.PayloadFile(t, []byte("<workbook/>")),
				Metadata: artifact.WorkbookMetadata{Name: "Workbook", TableauID: "w1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "example", SourceSite: "sandbox", SourceProjectName: "Examples", SourceProjectID: "p1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			canonicalRoot, err := workspacecore.CanonicalRoot(root)
			if err != nil {
				t.Fatal(err)
			}
			relative, err := filepath.Rel(canonicalRoot, pulled.ArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			relative = filepath.ToSlash(relative)
			body := fmt.Sprintf(`{"items":[{"artifact":%q},{"kind":"workbook","id":"w1"}]}`, relative)
			args := []string{"workspace", "artifact", operation, "--workspace", "source"}
			if operation == "move" {
				body = fmt.Sprintf(`{"items":[{"artifact":%q,"destination":"first"},{"kind":"workbook","id":"w1","destination":"second"}]}`, relative)
				args = []string{"workspace", "artifact", "move", "--source", "source"}
			}
			batch := filepath.Join(directory, "items.json")
			if err := os.WriteFile(batch, []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			args = append(args, "--batch-file", batch)
			code, output := run(args...)
			_, statErr := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
			if code == 0 || !strings.Contains(output, "batch item 2 duplicates batch item 1") || statErr != nil {
				t.Fatalf("code=%d artifact=%v output=%s", code, statErr, output)
			}
		})
	}
}

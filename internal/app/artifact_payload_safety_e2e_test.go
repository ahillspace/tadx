package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestWorkspaceArtifactRejectsCanonicalPayloadDirectory(t *testing.T) {
	for _, selector := range []string{"path", "identity"} {
		for _, operation := range []string{"delete", "move"} {
			for _, preview := range []bool{true, false} {
				forceOptions := []bool{false}
				if operation == "delete" {
					forceOptions = append(forceOptions, true)
				}
				for _, force := range forceOptions {
					name := selector + "/" + operation
					if preview {
						name += "/preview"
					} else {
						name += "/execute"
					}
					if force {
						name += "/force"
					}
					t.Run(name, func(t *testing.T) {
						base := t.TempDir()
						root := filepath.Join(base, "source")
						options := app.Options{ConfigPath: filepath.Join(base, "config.yaml")}
						run := func(args ...string) (int, string) {
							t.Helper()
							var out bytes.Buffer
							code := app.Run(t.Context(), args, &out, options)
							return code, out.String()
						}
						for _, workspace := range []string{"source", "destination"} {
							if code, out := run("workspace", "create", workspace, "--path", filepath.Join(base, workspace)); code != 0 {
								t.Fatal(out)
							}
						}
						created, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{
							Workspace: root, Filename: "Finance.twb", PayloadPath: app.PayloadFile(t, []byte("<workbook/>")),
							Metadata: artifact.WorkbookMetadata{Name: "Finance", TableauID: "workbook-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sandbox", SourceProjectName: "Examples", SourceProjectID: "project-1"},
						})
						if err != nil {
							t.Fatal(err)
						}
						if err := os.Remove(created.CanonicalPath); err != nil {
							t.Fatal(err)
						}
						if err := os.Mkdir(created.CanonicalPath, 0o700); err != nil {
							t.Fatal(err)
						}
						note := filepath.Join(created.CanonicalPath, "local-work.txt")
						if err := os.WriteFile(note, []byte("retain local work"), 0o600); err != nil {
							t.Fatal(err)
						}
						code, out := run("workspace", "status", "--workspace", "source", "--json")
						var status struct {
							Artifacts struct{ Invalid int } `json:"artifacts"`
						}
						if err := json.Unmarshal([]byte(out), &status); code != 0 || err != nil || status.Artifacts.Invalid != 1 {
							t.Fatalf("status: code=%d, error=%v, output=%s", code, err, out)
						}
						args := []string{"workspace", "artifact", operation}
						if operation == "delete" {
							args = append(args, "--workspace", "source")
						} else {
							args = append(args, "--source", "source", "--destination", "destination")
						}
						if selector == "path" {
							relative, err := filepath.Rel(root, created.ArtifactPath)
							if err != nil {
								t.Fatal(err)
							}
							args = append(args, "--artifact", filepath.ToSlash(relative))
						} else {
							args = append(args, "--kind", "workbook", "--id", "workbook-1")
						}
						if preview {
							args = append(args, "--preview")
						}
						if force {
							args = append(args, "--force")
						}
						if code, out := run(args...); code == 0 || !strings.Contains(out, "canonical artifact payload must be a regular file") {
							t.Errorf("invalid payload accepted: code=%d, output=%s", code, out)
						}
						if data, err := os.ReadFile(note); err != nil || string(data) != "retain local work" {
							t.Fatalf("local work changed: %q, %v", data, err)
						}
					})
				}
			}
		}
	}
}

package app_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestWorkspaceRemovalRequiresDefaultReassignmentThroughCLI(t *testing.T) {
	for _, operation := range []string{"unregister", "delete"} {
		t.Run(operation, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "development")
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				exit := app.Run(context.Background(), args, &output, options)
				return exit, output.String()
			}
			if exit, out := run("workspace", "create", "development", "--path", root); exit != 0 {
				t.Fatalf("create: %s", out)
			}
			if exit, out := run("workspace", "create", "other", "--path", filepath.Join(directory, "other")); exit != 0 {
				t.Fatalf("create other: %s", out)
			}
			if exit, out := run("workspace", operation, "development"); exit == 0 || !strings.Contains(out, "workspace set-default other") {
				t.Fatalf("removal: exit=%d %s", exit, out)
			}
			if _, err := os.Stat(filepath.Join(root, "tadx.yaml")); err != nil {
				t.Fatal(err)
			}
			if exit, out := run("workspace", "set-default", "other"); exit != 0 {
				t.Fatalf("set default: %s", out)
			}
			if exit, out := run("workspace", operation, "development"); exit != 0 {
				t.Fatalf("removal after reassignment: %s", out)
			}
			_, statErr := os.Stat(root)
			if operation == "delete" && !os.IsNotExist(statErr) {
				t.Fatalf("deleted workspace exists: %v", statErr)
			}
			if operation == "unregister" && statErr != nil {
				t.Fatalf("unregistered files changed: %v", statErr)
			}
		})
	}
}

func TestWorkspaceDeletionProtectsUnmanagedArtifactFilesThroughCLI(t *testing.T) {
	for _, extra := range []string{"", "clean-pulled", "artifacts/lineage/notes.txt", "artifacts/lineage/workbook/notes.txt", "artifacts/.tadx-personal/notes.txt", "artifacts/workbook/.tadx-personal/notes.txt", "artifact-notes", "artifact-nested", "artifact-lock", ".tadx.lock.bak", ".tadx.lock/notes.txt"} {
		t.Run(extra, func(t *testing.T) {
			directory := t.TempDir()
			root := filepath.Join(directory, "development")
			options := app.Options{ConfigPath: filepath.Join(directory, "config.yaml")}
			run := func(args ...string) (int, string) {
				var output bytes.Buffer
				exit := app.Run(context.Background(), args, &output, options)
				return exit, output.String()
			}
			for _, args := range [][]string{
				{"workspace", "create", "development", "--path", root},
				{"workspace", "create", "other", "--path", filepath.Join(directory, "other")},
				{"workspace", "set-default", "other"},
			} {
				if exit, out := run(args...); exit != 0 {
					t.Fatalf("setup: %s", out)
				}
			}
			pulled, err := artifact.NewWorkbookManager(nil).Pull(context.Background(), artifact.WorkbookPull{
				Workspace: root, Filename: "Finance.twb", Content: []byte("<workbook/>"),
				Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sandbox", SourceProjectName: "Ops", SourceProjectID: "project-1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := artifact.NewLineageManager(nil).Pull(context.Background(), artifact.LineagePull{
				Workspace: root,
				Metadata:  artifact.LineageMetadata{ResourceKind: "published_datasource", Name: "Sales", TableauID: "ds-1", MetadataID: "metadata-ds-1", SourceServerOrigin: "https://tableau.example.test", SourceSiteLUID: "site-1", SourceEnvironment: "dev", SourceSite: "sandbox"},
				Lineage:   artifact.LineageDocument{Complete: true, Direction: "both", Depth: 1, Nodes: []artifact.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}},
			}); err != nil {
				t.Fatal(err)
			}
			// A copied workspace can contain artifacts without a local mutation lock.
			if extra != "clean-pulled" {
				if err := os.Remove(filepath.Join(root, ".tadx.lock")); err != nil {
					t.Fatal(err)
				}
			}
			hasExtra := extra != "" && extra != "clean-pulled"
			var extraPath string
			if hasExtra {
				extraPath = filepath.Join(root, filepath.FromSlash(extra))
				if extra == "artifact-notes" {
					extraPath = filepath.Join(pulled.ArtifactPath, "notes.txt")
				} else if extra == "artifact-nested" {
					extraPath = filepath.Join(pulled.ArtifactPath, "drafts", "notes.txt")
				} else if extra == "artifact-lock" {
					extraPath = filepath.Join(pulled.ArtifactPath, ".tadx.lock")
				}
				if err := os.MkdirAll(filepath.Dir(extraPath), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(extraPath, []byte("keep these local notes"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if exit, out := run("workspace", "delete", "development", "--preview"); exit != 0 || !strings.Contains(out, "dirty: "+strconv.FormatBool(hasExtra)) {
				t.Errorf("deletion preview: exit=%d %s", exit, out)
			}
			exit, out := run("workspace", "delete", "development")
			if !hasExtra {
				if exit != 0 {
					t.Fatalf("clean artifact deletion: %s", out)
				}
				return
			}
			if exit == 0 || !strings.Contains(out, "workspace.delete.dirty") {
				t.Errorf("unmanaged files must require force: exit=%d %s", exit, out)
			}
			if data, err := os.ReadFile(extraPath); err != nil || string(data) != "keep these local notes" {
				t.Fatalf("unmanaged file was lost: %q, %v", data, err)
			}
			if exit, out := run("workspace", "delete", "development", "--force"); exit != 0 {
				t.Fatalf("forced deletion: %s", out)
			}
		})
	}
}

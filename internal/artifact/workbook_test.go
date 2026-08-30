package artifact_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
)

func TestWorkbookManagerWritesCanonicalArtifactAndProvenance(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(func() time.Time { return time.Date(2026, 8, 30, 10, 0, 0, 0, time.UTC) })
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twbx", Content: []byte("native-package"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.BaselineFingerprint == "" || result.ArtifactPath == "" {
		t.Fatalf("result = %#v", result)
	}
	for _, name := range []string{"Finance.twbx", "metadata.json", "view.md"} {
		if _, err := os.Stat(filepath.Join(result.ArtifactPath, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(result.ArtifactPath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["tableau_id"] != "wb-1" || metadata["local_baseline_fingerprint"] == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if _, exists := metadata["publish_target"]; exists {
		t.Fatalf("metadata persists publish target: %#v", metadata)
	}
}

func TestWorkbookManagerUsesPortableBoundedPathComponents(t *testing.T) {
	tests := []struct {
		name      string
		workbook  string
		filename  string
		extension string
	}{
		{name: "reserved device name", workbook: "CON", filename: "CON.twb", extension: ".twb"},
		{name: "long Unicode name", workbook: strings.Repeat("界", 100), filename: strings.Repeat("界", 100) + ".twbx", extension: ".twbx"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := artifact.NewWorkbookManager(time.Now)
			result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
				Workspace: createWorkspace(t), Filename: test.filename, Content: []byte("native-package"),
				Metadata: artifact.WorkbookMetadata{Name: test.workbook, TableauID: "wb-1", SourceEnvironment: "production", SourceProjectName: "Ops", SourceProjectID: "project-1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			artifactName := filepath.Base(result.ArtifactPath)
			payloadName := filepath.Base(result.CanonicalPath)
			if len([]byte(artifactName)) > 180 || len([]byte(payloadName)) > 180 {
				t.Fatalf("artifact component = %q, payload component = %q", artifactName, payloadName)
			}
			if strings.EqualFold(artifactName, "CON") || strings.EqualFold(payloadName, "CON.twb") {
				t.Fatalf("reserved path components were retained: %q, %q", artifactName, payloadName)
			}
			if filepath.Ext(payloadName) != test.extension {
				t.Fatalf("payload extension = %q", filepath.Ext(payloadName))
			}
			workbook, err := manager.Read(context.Background(), result.ArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(workbook.Content) != "native-package" || workbook.Filename != payloadName {
				t.Fatalf("workbook = %#v", workbook)
			}
		})
	}
}

func TestWorkbookManagerProtectsDirtyRepullAndOverwriteIsExplicit(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	first, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v1"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := filepath.Join(first.ArtifactPath, "Finance.twb")
	if err := os.WriteFile(payload, []byte("local-edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v2"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty re-pull error = %v", err)
	}
	unchanged, _ := os.ReadFile(payload)
	if string(unchanged) != "local-edit" {
		t.Fatalf("dirty payload changed to %q", unchanged)
	}
	overwritten, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v2"), Overwrite: true,
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(overwritten.Warnings) != 1 {
		t.Fatalf("warnings = %#v", overwritten.Warnings)
	}
	updated, _ := os.ReadFile(payload)
	if string(updated) != "remote-v2" {
		t.Fatalf("payload = %q", updated)
	}
}

func TestWorkbookManagerCleanRepullWarnsAndReplaces(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v1"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v2"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "clean") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestWorkbookManagerRejectsUnmanagedTargetWithoutChangingIt(t *testing.T) {
	workspace := createWorkspace(t)
	target := filepath.Join(workspace, "artifacts", "workbook", "Finance")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(target, "notes.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata:  artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
		Overwrite: true,
	})
	if err == nil || !strings.Contains(err.Error(), "not a managed workbook artifact") {
		t.Fatalf("Pull() error = %v", err)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(content) != "keep me" {
		t.Fatalf("unmanaged content = %q, error = %v", content, readErr)
	}
}

func TestWorkbookManagerRejectsArtifactRootOutsideWorkspaceBeforeWriting(t *testing.T) {
	container := t.TempDir()
	workspace := filepath.Join(container, "workspace")
	outside := filepath.Join(container, "outside")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "artifacts")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("Pull() error = %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(outside, "workbook")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("outside workbook directory was created: %v", statErr)
	}
}

func TestWorkbookManagerRejectsTargetOutsideArtifactRootWithoutChangingIt(t *testing.T) {
	workspace := createWorkspace(t)
	root := filepath.Join(workspace, "artifacts", "workbook")
	outside := filepath.Join(filepath.Dir(workspace), "outside")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "Finance")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"), Overwrite: true,
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1"},
	})
	if err == nil || !strings.Contains(err.Error(), "escapes artifact root") {
		t.Fatalf("Pull() error = %v", err)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(content) != "keep me" {
		t.Fatalf("outside content = %q, error = %v", content, readErr)
	}
}

func TestWorkbookManagerRejectsEscapingCanonicalPayload(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(workspace, "artifacts", "outside.twb")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(result.ArtifactPath, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata artifact.WorkbookMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata.CanonicalPayload = filepath.Join("..", "..", "outside.twb")
	metadataData, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, metadataData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "invalid workbook canonical payload") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWorkbookManagerRejectsCanonicalPayloadSymlink(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(workspace, "outside.twb")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(result.CanonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, result.CanonicalPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}

	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "must not be a symbolic link") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWorkbookManagerRejectsUnsupportedCanonicalPayload(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(result.ArtifactPath, "metadata.json")
	metadataData, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata artifact.WorkbookMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatal(err)
	}
	metadata.CanonicalPayload = "payload.txt"
	metadataData, err = json.Marshal(metadata)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, metadataData, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "unsupported workbook canonical payload") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWorkbookManagerRejectsIncompleteMetadataBeforeReadingPayload(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		modify func(*artifact.WorkbookMetadata)
	}{
		{name: "canonical name", field: "name", modify: func(metadata *artifact.WorkbookMetadata) { metadata.Name = "" }},
		{name: "Tableau LUID", field: "tableau_id", modify: func(metadata *artifact.WorkbookMetadata) { metadata.TableauID = "" }},
		{name: "source environment", field: "source_environment", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceEnvironment = "" }},
		{name: "source project name", field: "source_project_name", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceProjectName = "" }},
		{name: "source project LUID", field: "source_project_id", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceProjectID = "" }},
		{name: "pull time", field: "pulled_at", modify: func(metadata *artifact.WorkbookMetadata) { metadata.PulledAt = "" }},
		{name: "baseline", field: "local_baseline_fingerprint", modify: func(metadata *artifact.WorkbookMetadata) { metadata.LocalBaselineFingerprint = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := createWorkspace(t)
			manager := artifact.NewWorkbookManager(time.Now)
			result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
				Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
				Metadata: artifact.WorkbookMetadata{Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceSite: "", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
			})
			if err != nil {
				t.Fatal(err)
			}
			metadataPath := filepath.Join(result.ArtifactPath, "metadata.json")
			metadataData, err := os.ReadFile(metadataPath)
			if err != nil {
				t.Fatal(err)
			}
			var metadata artifact.WorkbookMetadata
			if err := json.Unmarshal(metadataData, &metadata); err != nil {
				t.Fatal(err)
			}
			test.modify(&metadata)
			metadataData, err = json.Marshal(metadata)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(metadataPath, metadataData, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Read() error = %v", err)
			}
		})
	}
}

func TestWorkbookManagerRejectsNoncanonicalFileSelector(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata: artifact.WorkbookMetadata{Name: "Finance", TableauID: "wb-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), filepath.Join(result.ArtifactPath, "view.md")); err == nil || !strings.Contains(err.Error(), "not the canonical payload") {
		t.Fatalf("Read() error = %v", err)
	}
	workbook, err := manager.Read(context.Background(), result.CanonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.Filename != "Finance.twb" || string(workbook.Content) != "remote" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func createWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

package artifact_test

import (
	"bytes"
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
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Ops", SourceProjectID: "project-1"},
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
	if metadata["tableau_id"] != "wb-1" || metadata["source_server_origin"] != "https://tableau.example.com" || metadata["source_site_luid"] != "site-1" || metadata["local_baseline_fingerprint"] == "" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if _, exists := metadata["publish_target"]; exists {
		t.Fatalf("metadata persists publish target: %#v", metadata)
	}
}

func TestWorkbookManagerRecordsPublishedDatasourcePortability(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twbx", Content: []byte("native"),
		Metadata: artifact.WorkbookMetadata{
			Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Ops", SourceProjectID: "project-1",
			Portability:          "source-site-bound",
			PublishedDatasources: []artifact.PublishedDatasourceRef{{LUID: "ds-1", Name: "Sales", SourceSite: "marketing"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(result.ArtifactPath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if fields["portability"] != "source-site-bound" {
		t.Fatalf("portability = %#v", fields["portability"])
	}
	view, err := os.ReadFile(filepath.Join(result.ArtifactPath, "view.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(view), "source-site-bound") || !strings.Contains(string(view), "ds-1") {
		t.Fatalf("view.md missing portability provenance:\n%s", view)
	}
	metadata, err := manager.ReadMetadata(context.Background(), result.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Portability != "source-site-bound" || len(metadata.PublishedDatasources) != 1 || metadata.PublishedDatasources[0].LUID != "ds-1" {
		t.Fatalf("metadata = %#v", metadata)
	}
}

func TestWorkbookManagerRejectsInvalidPortabilityContract(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	tests := []struct {
		name     string
		mutate   func(*artifact.WorkbookMetadata)
		contains string
	}{
		{name: "unknown enum", mutate: func(m *artifact.WorkbookMetadata) { m.Portability = "sometimes" }, contains: "portability"},
		{name: "reference without luid", mutate: func(m *artifact.WorkbookMetadata) {
			m.Portability = "source-site-bound"
			m.PublishedDatasources = []artifact.PublishedDatasourceRef{{Name: "Sales"}}
		}, contains: "published_datasource"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metadata := artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Ops", SourceProjectID: "project-1"}
			test.mutate(&metadata)
			_, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v"), Metadata: metadata})
			if err == nil || !strings.Contains(err.Error(), test.contains) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestWorkbookManagerReadExposesSourceProvenance(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twbx", Content: []byte("native"),
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "marketing", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := manager.Read(context.Background(), result.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if workbook.SourceEnvironment != "production" || workbook.SourceSite != "marketing" || workbook.SourceProjectName != "Department/Ops" || workbook.SourceProjectID != "project-1" {
		t.Fatalf("workbook = %#v", workbook)
	}
	metadata, err := manager.ReadMetadata(context.Background(), result.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.SourceEnvironment != "production" || metadata.TableauID != "wb-1" || metadata.Name != "Finance" {
		t.Fatalf("metadata = %#v", metadata)
	}
	if _, err := manager.ReadMetadata(context.Background(), filepath.Join(workspace, "missing")); err == nil {
		t.Fatal("ReadMetadata accepted a missing artifact")
	}
}

func TestWorkbookManagerPreservesEmptyDefaultSiteProvenance(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("native-workbook"),
		Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(result.ArtifactPath, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	site, exists := fields["source_site"]
	if !exists || string(site) != `""` {
		t.Fatalf("source_site = %s, exists = %t", site, exists)
	}
	delete(fields, "source_site")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "source_site") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWorkbookManagerStoresSameNameWorkbooksByTableauIdentity(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	firstMetadata := validMetadata("Finance", "wb-1")
	firstMetadata.SourceProjectName = "ProjectA"
	firstMetadata.SourceProjectID = "project-a"
	first, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("project-a"), Metadata: firstMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata := validMetadata("Finance", "wb-2")
	secondMetadata.SourceProjectName = "ProjectB"
	secondMetadata.SourceProjectID = "project-b"
	second, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("project-b"), Metadata: secondMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ArtifactPath == second.ArtifactPath || !strings.HasPrefix(filepath.Base(first.ArtifactPath), "Finance--") || !strings.HasPrefix(filepath.Base(second.ArtifactPath), "Finance--") {
		t.Fatalf("artifact paths = %q and %q", first.ArtifactPath, second.ArtifactPath)
	}
	for _, expected := range []struct {
		path    string
		id      string
		content string
	}{{first.ArtifactPath, "wb-1", "project-a"}, {second.ArtifactPath, "wb-2", "project-b"}} {
		workbook, err := manager.Read(context.Background(), expected.path)
		if err != nil {
			t.Fatal(err)
		}
		content, readErr := os.ReadFile(workbook.PayloadPath)
		if readErr != nil {
			t.Fatal(readErr)
		}
		if workbook.TableauID != expected.id || string(content) != expected.content {
			t.Fatalf("workbook = %#v", workbook)
		}
	}
}

func TestWorkbookManagerScopesWorkbookIdentityToServerAndSite(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	tests := []struct {
		name         string
		serverOrigin string
		siteLUID     string
	}{
		{name: "first server", serverOrigin: "https://first.example.com", siteLUID: "site-1"},
		{name: "second server", serverOrigin: "https://second.example.com", siteLUID: "site-1"},
		{name: "second site", serverOrigin: "https://first.example.com", siteLUID: "site-2"},
	}
	paths := make(map[string]bool, len(tests))
	for _, test := range tests {
		metadata := validMetadata("Finance", "wb-1")
		metadata.SourceServerOrigin = test.serverOrigin
		metadata.SourceSiteLUID = test.siteLUID
		result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
			Workspace: workspace, Filename: "Finance.twb", Content: []byte(test.name), Metadata: metadata,
		})
		if err != nil {
			t.Fatal(err)
		}
		if paths[result.ArtifactPath] {
			t.Fatalf("artifact path reused across source identities: %q", result.ArtifactPath)
		}
		paths[result.ArtifactPath] = true
		workbook, err := manager.Read(context.Background(), result.ArtifactPath)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(workbook.PayloadPath)
		if err != nil {
			t.Fatal(err)
		}
		if string(content) != test.name || !strings.HasPrefix(filepath.Base(result.ArtifactPath), "Finance--") {
			t.Fatalf("artifact = %q, content = %q", result.ArtifactPath, content)
		}
	}
}

func TestWorkbookManagerNormalizesServerOriginBeforeIdentityMatching(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	firstMetadata := validMetadata("Finance", "wb-1")
	firstMetadata.SourceServerOrigin = "HTTPS://TABLEAU.EXAMPLE.COM:443/tableau"
	first, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("first"), Metadata: firstMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	secondMetadata := validMetadata("Finance", "wb-1")
	secondMetadata.SourceServerOrigin = "https://tableau.example.com/other-path"
	second, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("second"), Metadata: secondMetadata,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.ArtifactPath != second.ArtifactPath {
		t.Fatalf("equivalent server origins produced different paths: %q and %q", first.ArtifactPath, second.ArtifactPath)
	}
	data, err := os.ReadFile(filepath.Join(second.ArtifactPath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata artifact.WorkbookMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.SourceServerOrigin != "https://tableau.example.com" {
		t.Fatalf("source server origin = %q", metadata.SourceServerOrigin)
	}
}

func TestWorkbookManagerIdentityPathsCannotCollideWithWorkbookNames(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	inputs := []struct {
		name string
		id   string
	}{
		{name: "Finance", id: "wb-a"},
		{name: "Finance-a1986ff60b17b262", id: "wb-c"},
		{name: "Finance", id: "wb-b"},
	}
	paths := make(map[string]bool, len(inputs))
	for _, input := range inputs {
		metadata := validMetadata(input.name, input.id)
		result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
			Workspace: workspace, Filename: input.name + ".twb", Content: []byte(input.id), Metadata: metadata,
		})
		if err != nil {
			t.Fatal(err)
		}
		if paths[result.ArtifactPath] {
			t.Fatalf("artifact path reused for %q: %q", input.id, result.ArtifactPath)
		}
		paths[result.ArtifactPath] = true
		workbook, err := manager.Read(context.Background(), result.ArtifactPath)
		if err != nil {
			t.Fatal(err)
		}
		if workbook.TableauID != input.id {
			t.Fatalf("workbook identity = %q, want %q", workbook.TableauID, input.id)
		}
	}
}

func TestWorkbookManagerRejectsIncompleteProvenanceBeforeStaging(t *testing.T) {
	tests := []struct {
		name   string
		field  string
		modify func(*artifact.WorkbookMetadata)
	}{
		{name: "environment", field: "source_environment", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceEnvironment = "" }},
		{name: "server origin", field: "source_server_origin", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceServerOrigin = "" }},
		{name: "site LUID", field: "source_site_luid", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceSiteLUID = "" }},
		{name: "project name", field: "source_project_name", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceProjectName = "" }},
		{name: "project LUID", field: "source_project_id", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceProjectID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workspace := createWorkspace(t)
			metadata := validMetadata("Finance", "wb-1")
			test.modify(&metadata)
			manager := artifact.NewWorkbookManager(time.Now)
			_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
				Workspace: workspace, Filename: "Finance.twb", Content: []byte("native-workbook"), Metadata: metadata,
			})
			if err == nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("Pull() error = %v", err)
			}
			entries, readErr := os.ReadDir(filepath.Join(workspace, "artifacts", "workbook"))
			if errors.Is(readErr, os.ErrNotExist) {
				return
			}
			if readErr != nil {
				t.Fatal(readErr)
			}
			if len(entries) != 0 {
				t.Fatalf("artifact entries = %#v", entries)
			}
		})
	}
}

func TestWorkbookManagerRejectsOversizedMetadataBeforeStaging(t *testing.T) {
	workspace := createWorkspace(t)
	metadata := validMetadata("Finance", "wb-1")
	metadata.SourceEnvironment = strings.Repeat("x", 64*1024)
	manager := artifact.NewWorkbookManager(time.Now)
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("native-workbook"), Metadata: metadata,
	})
	if err == nil || !strings.Contains(err.Error(), "65536-byte limit") {
		t.Fatalf("Pull() error = %v", err)
	}
	entries, readErr := os.ReadDir(filepath.Join(workspace, "artifacts", "workbook"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("artifact entries = %#v", entries)
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
		{name: "superscript COM device name", workbook: "COM¹", filename: "COM¹.twb", extension: ".twb"},
		{name: "superscript LPT device name", workbook: "LPT³", filename: "LPT³.twbx", extension: ".twbx"},
		{name: "long Unicode name", workbook: strings.Repeat("界", 100), filename: strings.Repeat("界", 100) + ".twbx", extension: ".twbx"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manager := artifact.NewWorkbookManager(time.Now)
			result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
				Workspace: createWorkspace(t), Filename: test.filename, Content: []byte("native-package"),
				Metadata: validMetadata(test.workbook, "wb-1"),
			})
			if err != nil {
				t.Fatal(err)
			}
			artifactName := filepath.Base(result.ArtifactPath)
			payloadName := filepath.Base(result.CanonicalPath)
			if len([]byte(artifactName)) > 180 || len([]byte(payloadName)) > 180 {
				t.Fatalf("artifact component = %q, payload component = %q", artifactName, payloadName)
			}
			if artifactName == test.workbook || payloadName == test.filename {
				t.Fatalf("reserved path components were retained: %q, %q", artifactName, payloadName)
			}
			if filepath.Ext(payloadName) != test.extension {
				t.Fatalf("payload extension = %q", filepath.Ext(payloadName))
			}
			workbook, err := manager.Read(context.Background(), result.ArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			content, readErr := os.ReadFile(workbook.PayloadPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if string(content) != "native-package" || workbook.Filename != payloadName {
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
		Metadata: validMetadata("Finance", "wb-1"),
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
		Metadata: validMetadata("Finance", "wb-1"),
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
		Metadata: validMetadata("Finance", "wb-1"),
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
	_, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v1"), Metadata: validMetadata("Finance", "wb-1")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Finance.twb", Content: []byte("v2"), Metadata: validMetadata("Finance", "wb-1")})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "clean") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestWorkbookManagerPreservesUnmanagedNamePath(t *testing.T) {
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
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"),
		Metadata:  validMetadata("Finance", "wb-1"),
		Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.ArtifactPath == target {
		t.Fatalf("managed artifact reused unmanaged path %q", target)
	}
	content, readErr := os.ReadFile(sentinel)
	if readErr != nil || string(content) != "keep me" {
		t.Fatalf("unmanaged content = %q, error = %v", content, readErr)
	}
}

func TestWorkbookManagerRejectsIncompleteIdentityMatchWithoutReplacingFiles(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v1"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metadataPath := filepath.Join(result.ArtifactPath, "metadata.json")
	data, err := os.ReadFile(metadataPath)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "source_project_id")
	data, err = json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(metadataPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(result.ArtifactPath, "notes.txt")
	if err := os.WriteFile(sentinel, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote-v2"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err == nil || !strings.Contains(err.Error(), "source_project_id") {
		t.Fatalf("Pull() error = %v", err)
	}
	payload, payloadErr := os.ReadFile(result.CanonicalPath)
	note, noteErr := os.ReadFile(sentinel)
	if payloadErr != nil || string(payload) != "remote-v1" || noteErr != nil || string(note) != "keep me" {
		t.Fatalf("payload = %q, payload error = %v, note = %q, note error = %v", payload, payloadErr, note, noteErr)
	}
}

func TestWorkbookManagerRejectsOversizedMetadata(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.ArtifactPath, "metadata.json"), bytes.Repeat([]byte("x"), 64*1024+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "65536-byte limit") {
		t.Fatalf("Read() error = %v", err)
	}
}

func TestWorkbookManagerRejectsNonregularCanonicalPayload(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(result.CanonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(result.CanonicalPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("Read() error = %v", err)
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
		Metadata: validMetadata("Finance", "wb-1"),
	})
	if err == nil || (!strings.Contains(err.Error(), "escapes workspace") && !strings.Contains(err.Error(), "must not be a symbolic link")) {
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
		Metadata: validMetadata("Finance", "wb-1"),
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
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
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
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
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
		Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
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
		{name: "source server origin", field: "source_server_origin", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceServerOrigin = "" }},
		{name: "source site LUID", field: "source_site_luid", modify: func(metadata *artifact.WorkbookMetadata) { metadata.SourceSiteLUID = "" }},
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
				Metadata: artifact.WorkbookMetadata{Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
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
		Metadata: artifact.WorkbookMetadata{Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceProjectName: "Department/Ops", SourceProjectID: "project-1"},
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
	content, readErr := os.ReadFile(workbook.PayloadPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if workbook.Filename != "Finance.twb" || string(content) != "remote" {
		t.Fatalf("workbook = %#v", workbook)
	}
}

func TestWorkbookManagerAcceptsHardLinkToCanonicalPayload(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(result.ArtifactPath, "Finance-hardlink.twb")
	if err := os.Link(result.CanonicalPath, alias); err != nil {
		t.Skipf("hard links are unavailable: %v", err)
	}
	workbook, err := manager.Read(context.Background(), alias)
	if err != nil {
		t.Fatalf("Read() rejected the same filesystem file: %v", err)
	}
	if workbook.PayloadPath != result.CanonicalPath {
		t.Fatalf("payload path = %q, want %q", workbook.PayloadPath, result.CanonicalPath)
	}
}

func TestWorkbookManagerRejectsSymlinkSelectorToCanonicalPayload(t *testing.T) {
	workspace := createWorkspace(t)
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{
		Workspace: workspace, Filename: "Finance.twb", Content: []byte("remote"), Metadata: validMetadata("Finance", "wb-1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(result.ArtifactPath, "Finance-symlink.twb")
	if err := os.Symlink(result.CanonicalPath, alias); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := manager.Read(context.Background(), alias); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Read() error = %v", err)
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

func validMetadata(name, tableauID string) artifact.WorkbookMetadata {
	return artifact.WorkbookMetadata{
		Name:               name,
		TableauID:          tableauID,
		SourceServerOrigin: "https://tableau.example.com",
		SourceSiteLUID:     "site-1",
		SourceEnvironment:  "production",
		SourceSite:         "",
		SourceProjectName:  "Ops",
		SourceProjectID:    "project-1",
	}
}

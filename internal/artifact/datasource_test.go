package artifact

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

func TestDatasourceManagerPullPreservesNativePayloadAndRecordsProvenance(t *testing.T) {
	fixed := time.Date(2026, 8, 31, 17, 0, 0, 123, time.UTC)
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(func() time.Time { return fixed })

	for _, extension := range []string{".tds", ".tdsx"} {
		t.Run(extension, func(t *testing.T) {
			content := []byte("native" + extension + "\x00payload")
			metadata := validDatasourceMetadata("Finance", "ds-"+extension)
			result, err := manager.Pull(context.Background(), DatasourcePull{
				Workspace: workspace,
				Filename:  "Finance" + extension,
				Content:   content,
				Metadata:  metadata,
			})
			if err != nil {
				t.Fatal(err)
			}
			wantRoot := filepath.Join(workspace, "artifacts", "datasource") + string(filepath.Separator)
			if !strings.HasPrefix(result.ArtifactPath, wantRoot) || filepath.Dir(result.ArtifactPath) != strings.TrimSuffix(wantRoot, string(filepath.Separator)) {
				t.Fatalf("artifact path = %q", result.ArtifactPath)
			}
			wantRelative, err := filepath.Rel(workspace, result.ArtifactPath)
			if err != nil {
				t.Fatal(err)
			}
			wantRelative = filepath.ToSlash(wantRelative)
			if result.WorkspaceRelativePath != wantRelative || strings.Contains(result.WorkspaceRelativePath, "\\") {
				t.Fatalf("workspace-relative path = %q, want %q", result.WorkspaceRelativePath, wantRelative)
			}
			got, err := os.ReadFile(result.CanonicalPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(content) {
				t.Fatalf("canonical payload changed: %q", got)
			}

			data, err := os.ReadFile(filepath.Join(result.ArtifactPath, "metadata.json"))
			if err != nil {
				t.Fatal(err)
			}
			var stored DatasourceMetadata
			if err := json.Unmarshal(data, &stored); err != nil {
				t.Fatal(err)
			}
			if stored.Kind != "datasource" || stored.Name != metadata.Name || stored.TableauID != metadata.TableauID ||
				stored.SourceServerOrigin != "https://tableau.example.com" || stored.SourceSiteLUID != metadata.SourceSiteLUID ||
				stored.SourceEnvironment != metadata.SourceEnvironment || stored.SourceSite != metadata.SourceSite ||
				stored.SourceProjectName != metadata.SourceProjectName || stored.SourceProjectID != metadata.SourceProjectID ||
				stored.PulledAt != fixed.Format(time.RFC3339Nano) || stored.CanonicalPayload != "Finance"+extension ||
				stored.LocalBaselineFingerprint != fingerprint(content) || stored.CompositionStatus != CompositionStatusUnknown {
				t.Fatalf("stored metadata = %#v", stored)
			}
			if _, err := os.Stat(filepath.Join(result.ArtifactPath, "view.md")); err != nil {
				t.Fatalf("view.md: %v", err)
			}
		})
	}
}

func TestDatasourceManagerUsesNormalizedSourceIdentityForRefresh(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	first := validDatasourceMetadata("Finance", "ds-1")
	first.SourceServerOrigin = " HTTPS://TABLEAU.EXAMPLE.COM:443/path?ignored=yes "
	initial, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("v1"), Metadata: first})
	if err != nil {
		t.Fatal(err)
	}
	refresh := validDatasourceMetadata("Finance Renamed", "ds-1")
	refresh.SourceServerOrigin = "https://tableau.example.com"
	updated, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Renamed.tds", Content: []byte("v2"), Metadata: refresh})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ArtifactPath != initial.ArtifactPath {
		t.Fatalf("refresh moved artifact from %q to %q", initial.ArtifactPath, updated.ArtifactPath)
	}
	if len(updated.Warnings) != 1 || !strings.Contains(updated.Warnings[0], "clean") {
		t.Fatalf("warnings = %#v", updated.Warnings)
	}
	entries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "datasource"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("datasource artifact entries = %d", len(entries))
	}
}

func TestDatasourceManagerKeepsSameLUIDFromDifferentSitesDistinct(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	first := validDatasourceMetadata("Finance", "ds-1")
	second := validDatasourceMetadata("Finance", "ds-1")
	second.SourceSiteLUID = "site-2"

	one, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("one"), Metadata: first})
	if err != nil {
		t.Fatal(err)
	}
	two, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("two"), Metadata: second})
	if err != nil {
		t.Fatal(err)
	}
	if one.ArtifactPath == two.ArtifactPath {
		t.Fatalf("different sites share artifact path %q", one.ArtifactPath)
	}
}

func TestDatasourceManagerProtectsDirtyRefreshUnlessOverwrite(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	input := DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("remote-v1"), Metadata: validDatasourceMetadata("Finance", "ds-1")}
	first, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.CanonicalPath, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	input.Content = []byte("remote-v2")
	if _, err := manager.Pull(context.Background(), input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("dirty refresh error = %v", err)
	}
	unchanged, _ := os.ReadFile(first.CanonicalPath)
	if string(unchanged) != "local edit" {
		t.Fatalf("dirty payload changed to %q", unchanged)
	}
	input.Overwrite = true
	updated, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Warnings) != 1 || !strings.Contains(updated.Warnings[0], "overwrite") {
		t.Fatalf("warnings = %#v", updated.Warnings)
	}
	content, _ := os.ReadFile(updated.CanonicalPath)
	if string(content) != "remote-v2" {
		t.Fatalf("payload = %q", content)
	}
}

func TestDatasourceManagerRestoresPreviousArtifactWhenReplacementInstallFails(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	input := DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("v1"), Metadata: validDatasourceMetadata("Finance", "ds-1")}
	first, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	realRename := os.Rename
	renames := 0
	manager.operations.rename = func(oldPath, newPath string) error {
		renames++
		if renames == 2 {
			return errors.New("injected install failure")
		}
		return realRename(oldPath, newPath)
	}
	input.Content = []byte("v2")
	_, err = manager.Pull(context.Background(), input)
	if err == nil || !strings.Contains(err.Error(), "injected install failure") {
		t.Fatalf("replacement error = %v", err)
	}
	content, readErr := os.ReadFile(first.CanonicalPath)
	if readErr != nil || string(content) != "v1" {
		t.Fatalf("restored payload = %q, error = %v", content, readErr)
	}
}

func TestDatasourceManagerDoesNotUseWorkspaceLock(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	handle, err := lock.Acquire(filepath.Join(workspace, ".tadx.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = handle.Release() }()
	done := make(chan error, 1)
	go func() {
		_, pullErr := NewDatasourceManager(time.Now).Pull(context.Background(), DatasourcePull{
			Workspace: workspace,
			Filename:  "Finance.tds",
			Content:   []byte("remote"),
			Metadata:  validDatasourceMetadata("Finance", "ds-1"),
		})
		done <- pullErr
	}()
	select {
	case pullErr := <-done:
		if pullErr != nil {
			t.Fatal(pullErr)
		}
	case <-time.After(2 * time.Second):
		if err := handle.Release(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("Pull waited for the workspace lock")
	}
}

func TestDatasourceManagerRejectsOversizedMetadata(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	result, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("remote"), Metadata: validDatasourceMetadata("Finance", "ds-1")})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(result.ArtifactPath, "metadata.json"), make([]byte, maxDatasourceMetadataBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("Read error = %v", err)
	}
}

func TestDatasourceManagerRejectsSymlinkedArtifactRoot(t *testing.T) {
	container := t.TempDir()
	workspace := filepath.Join(container, "workspace")
	outside := filepath.Join(container, "outside")
	if err := os.MkdirAll(filepath.Join(workspace, "artifacts"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "artifacts", "datasource")); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	_, err := NewDatasourceManager(time.Now).Pull(context.Background(), DatasourcePull{
		Workspace: workspace,
		Filename:  "Finance.tds",
		Content:   []byte("remote"),
		Metadata:  validDatasourceMetadata("Finance", "ds-1"),
	})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Pull error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(outside, "metadata.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("outside path changed: %v", err)
	}
}

func TestDatasourceManagerRejectsSymlinkedCanonicalPayload(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewDatasourceManager(time.Now)
	result, err := manager.Pull(context.Background(), DatasourcePull{Workspace: workspace, Filename: "Finance.tds", Content: []byte("remote"), Metadata: validDatasourceMetadata("Finance", "ds-1")})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(workspace, "outside.tds")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(result.CanonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, result.CanonicalPath); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	if _, err := manager.Read(context.Background(), result.ArtifactPath); err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("Read error = %v", err)
	}
}

func TestDatasourceManagerRejectsIncompleteMetadata(t *testing.T) {
	metadata := validDatasourceMetadata("Finance", "ds-1")
	metadata.SourceProjectID = ""
	_, err := NewDatasourceManager(time.Now).Pull(context.Background(), DatasourcePull{
		Workspace: createDatasourceWorkspace(t),
		Filename:  "Finance.tds",
		Content:   []byte("remote"),
		Metadata:  metadata,
	})
	if err == nil || !strings.Contains(err.Error(), "source_project_id") {
		t.Fatalf("Pull error = %v", err)
	}
}

func createDatasourceWorkspace(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return workspace
}

func validDatasourceMetadata(name, tableauID string) DatasourceMetadata {
	return DatasourceMetadata{
		Name:               name,
		TableauID:          tableauID,
		SourceServerOrigin: "https://tableau.example.com",
		SourceSiteLUID:     "site-1",
		SourceEnvironment:  "production",
		SourceSite:         "primary",
		SourceProjectName:  "Department/Ops",
		SourceProjectID:    "project-1",
	}
}

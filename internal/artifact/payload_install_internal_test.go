package artifact

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// downloadedPayload writes content where a pull stages a download, inside the
// workspace's disposable state, and returns its path and identity.
func downloadedPayload(t *testing.T, workspace, name, content string) (string, os.FileInfo) {
	t.Helper()
	path := filepath.Join(workspace, ".tadx", "tmp", "pull", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, downloadedPayloadIdentity(t, path)
}

// downloadedPayloadIdentity captures identity through a handle before moving
// the file. On Windows, path-based Stat defers its file ID lookup until SameFile
// runs, when the original path has already been removed by the rename.
func downloadedPayloadIdentity(t *testing.T, path string) os.FileInfo {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil {
		t.Fatal(statErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	return info
}

// standaloneWorkbook is a workbook pull without acquired dependencies.
func standaloneWorkbook(t *testing.T, workspace string) WorkbookPull {
	return WorkbookPull{Workspace: workspace, Filename: "Finance.twb", PayloadPath: PayloadFile(t, []byte("workbook-v1")), Metadata: WorkbookMetadata{
		Name: "Finance", TableauID: "wb-1", SourceServerOrigin: "https://tableau.example.com", SourceSiteLUID: "site-1",
		SourceEnvironment: "dev", SourceProjectName: "Ops", SourceProjectID: "project-1",
	}}
}

func assertInstalledByRename(t *testing.T, download string, downloaded os.FileInfo, canonical string) {
	t.Helper()
	installed, err := os.Stat(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(downloaded, installed) {
		t.Fatalf("%s was copied instead of renamed into the artifact", filepath.Base(canonical))
	}
	if _, err := os.Stat(download); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("downloaded payload still present: %v", err)
	}
}

func assertNoArtifactStaging(t *testing.T, workspace string) {
	t.Helper()
	err := filepath.WalkDir(filepath.Join(workspace, "artifacts"), func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.Contains(entry.Name(), "-stage-") {
			t.Errorf("artifact staging left behind: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestWorkbookManagerInstallsTheDownloadedPayloadByRename(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	input := standaloneWorkbook(t, workspace)
	download, downloaded := downloadedPayload(t, workspace, "workbook", "workbook-v1")
	input.PayloadPath = download
	result, err := NewWorkbookManager(time.Now).Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	assertInstalledByRename(t, download, downloaded, result.CanonicalPath)
	assertNoArtifactStaging(t, workspace)
}

func TestFlowManagerInstallsTheDownloadedPayloadByRename(t *testing.T) {
	workspace := createFlowWorkspace(t)
	download, downloaded := downloadedPayload(t, workspace, "flow", "flow-v1")
	result, err := NewFlowManager(time.Now).Pull(context.Background(), FlowPull{Workspace: workspace, Filename: "Daily.tfl", PayloadPath: download, Metadata: validFlowMetadata(), Lineage: LineageDocument{Complete: true, Direction: "both", Depth: 1}})
	if err != nil {
		t.Fatal(err)
	}
	assertInstalledByRename(t, download, downloaded, result.CanonicalPath)
	assertNoArtifactStaging(t, workspace)
}

func TestWorkbookBundleManagerInstallsEveryDownloadedPayloadByRename(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	input := validWorkbookBundle(t, workspace, "v1")
	workbookDownload, workbookDownloaded := downloadedPayload(t, workspace, "workbook", "workbook-v1")
	input.Workbook.PayloadPath = workbookDownload
	datasourceDownloads := make([]string, len(input.Datasources))
	datasourceDownloaded := make([]os.FileInfo, len(input.Datasources))
	for index := range input.Datasources {
		datasourceDownloads[index], datasourceDownloaded[index] = downloadedPayload(t, workspace, "datasource-"+input.Datasources[index].Metadata.TableauID, "datasource-v1")
		input.Datasources[index].PayloadPath = datasourceDownloads[index]
	}
	result, err := NewWorkbookBundleManager(time.Now).Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	assertInstalledByRename(t, workbookDownload, workbookDownloaded, result.Workbook.CanonicalPath)
	for index, datasource := range result.Datasources {
		assertInstalledByRename(t, datasourceDownloads[index], datasourceDownloaded[index], datasource.CanonicalPath)
	}
	assertNoArtifactStaging(t, workspace)
}

func TestPullsLeaveNoStagingWhenThePayloadCannotBeRead(t *testing.T) {
	missing := func(workspace string) string { return filepath.Join(workspace, "missing") }
	tests := map[string]func(t *testing.T, workspace string) error{
		"workbook": func(t *testing.T, workspace string) error {
			input := standaloneWorkbook(t, workspace)
			input.PayloadPath = missing(workspace)
			_, err := NewWorkbookManager(time.Now).Pull(context.Background(), input)
			return err
		},
		"flow": func(_ *testing.T, workspace string) error {
			_, err := NewFlowManager(time.Now).Pull(context.Background(), FlowPull{Workspace: workspace, Filename: "Daily.tfl", PayloadPath: missing(workspace), Metadata: validFlowMetadata(), Lineage: LineageDocument{Complete: true, Direction: "both", Depth: 1}})
			return err
		},
		"bundle": func(t *testing.T, workspace string) error {
			input := validWorkbookBundle(t, workspace, "v1")
			input.Datasources[1].PayloadPath = missing(workspace)
			_, err := NewWorkbookBundleManager(time.Now).Pull(context.Background(), input)
			return err
		},
	}
	for name, pull := range tests {
		t.Run(name, func(t *testing.T) {
			workspace := createDatasourceWorkspace(t)
			if err := pull(t, workspace); err == nil || !strings.Contains(err.Error(), "fingerprint downloaded") {
				t.Fatalf("error = %v", err)
			}
			entries, err := os.ReadDir(filepath.Join(workspace, "artifacts"))
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			for _, kind := range entries {
				contents, _ := os.ReadDir(filepath.Join(workspace, "artifacts", kind.Name()))
				for _, entry := range contents {
					t.Errorf("unexpected entry %s/%s after a failed pull", kind.Name(), entry.Name())
				}
			}
		})
	}
}

func TestDirtyRefusalLeavesTheDownloadedPayloadAndNoStaging(t *testing.T) {
	workspace := createDatasourceWorkspace(t)
	manager := NewWorkbookManager(time.Now)
	input := standaloneWorkbook(t, workspace)
	first, err := manager.Pull(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first.CanonicalPath, []byte("local edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	download, _ := downloadedPayload(t, workspace, "workbook", "workbook-v2")
	input.PayloadPath = download
	if _, err := manager.Pull(context.Background(), input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("error = %v", err)
	}
	if content, err := os.ReadFile(first.CanonicalPath); err != nil || string(content) != "local edit" {
		t.Fatalf("dirty payload = %q, %v", content, err)
	}
	if content, err := os.ReadFile(download); err != nil || string(content) != "workbook-v2" {
		t.Fatalf("a refused pull consumed its download: %q, %v", content, err)
	}
	assertNoArtifactStaging(t, workspace)
}

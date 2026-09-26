package app

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type workbookPreviewReader struct {
	workbookDependencyPullReader
	t *testing.T
}

func (r workbookPreviewReader) DownloadWorkbook(context.Context, string, *bool) (workbookops.Download, error) {
	r.t.Fatal("preview downloaded workbook")
	return workbookops.Download{}, nil
}
func (r workbookPreviewReader) DownloadPublishedDatasource(context.Context, string) (workbookops.DatasourceDownload, error) {
	r.t.Fatal("preview downloaded dependency")
	return workbookops.DatasourceDownload{}, nil
}
func (r workbookPreviewReader) CaptureWorkbookLineage(context.Context, workbookops.LineageRequest) (workbookops.LineageCapture, error) {
	r.t.Fatal("preview captured optional lineage")
	return workbookops.LineageCapture{}, nil
}

func TestWorkbookAcquisitionPreviewPreservesDependencyOverwriteScope(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	input := workbookops.PullInput{Preview: true, Workspace: root, WorkspaceName: "work", Environment: "source", Site: "source-site", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1", LUID: "wb-1", IncludePDS: true, Overwrite: true}
	actionReader, actionWriter := workbookPreviewReader{t: t}, artifactWriter{}
	preview, err := workbookops.Pull(context.Background(), actionReader, actionWriter, input)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Preview == nil || len(preview.Preview.Dependencies) != 1 || preview.Preview.Dependencies[0].LUID != "ds-1" || preview.Preview.Dependencies[0].Overwrite || !preview.Preview.Target.Overwrite {
		t.Fatalf("wrong acquisition scope: %#v", preview.Preview)
	}
	if _, err := os.Stat(filepath.Join(root, "artifacts")); !os.IsNotExist(err) {
		t.Fatalf("preview created artifacts: %v", err)
	}
	dependency, err := artifact.NewDatasourceManager(nil).Pull(context.Background(), artifact.DatasourcePull{Workspace: root, Filename: "Sales.tds", Content: []byte("<datasource/>"), Metadata: artifact.DatasourceMetadata{Name: "Sales", TableauID: "ds-1", SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: "Analytics", SourceProjectID: "project-1"}, Lineage: artifact.LineageDocument{Direction: "both", Depth: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dependency.CanonicalPath, []byte("local dependency edit"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := workbookops.Pull(context.Background(), actionReader, actionWriter, input); err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("parent overwrite bypassed dependency dirty guard: %v", err)
	}
	input.IncludePDS = false
	preview, err = workbookops.Pull(context.Background(), actionReader, actionWriter, input)
	if err != nil || preview.Preview == nil || len(preview.Preview.Dependencies) != 0 {
		t.Fatalf("unrequested dependencies included: %#v %v", preview.Preview, err)
	}
	data, err := os.ReadFile(dependency.CanonicalPath)
	if err != nil || string(data) != "local dependency edit" {
		t.Fatalf("preview changed dependency: %s %v", data, err)
	}
}

package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/identity"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type workbookDependencyPullReader struct{}

func (workbookDependencyPullReader) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	return workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Analytics"}, nil
}

func (workbookDependencyPullReader) DownloadWorkbook(context.Context, string, *bool) (workbookops.Download, error) {
	return workbookops.Download{Filename: "Finance.twb", Content: []byte(`<workbook/>`)}, nil
}

func (workbookDependencyPullReader) CaptureWorkbookLineage(context.Context, workbookops.LineageRequest) (workbookops.LineageCapture, error) {
	return workbookops.LineageCapture{
		RootMetadataID: "metadata-wb-1",
		Complete:       true,
		Nodes:          []workbookops.LineageNode{{MetadataID: "metadata-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"}},
	}, nil
}

func (workbookDependencyPullReader) PublishedDatasources(context.Context, string) ([]workbookops.PublishedDatasource, error) {
	return []workbookops.PublishedDatasource{{LUID: "ds-1", Name: "Sales"}}, nil
}

func (workbookDependencyPullReader) DownloadPublishedDatasource(context.Context, string) (workbookops.DatasourceDownload, error) {
	return workbookops.DatasourceDownload{
		LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics",
		Filename: "Sales.tds", Content: []byte(`<datasource><connection class="sqlserver"/></datasource>`),
	}, nil
}

type datasourcePublishPreviewDependency struct{}

func (datasourcePublishPreviewDependency) ResolveProject(context.Context, identity.Selector) (datasourceops.Project, error) {
	return datasourceops.Project{LUID: "target-project", Name: "Target", Path: "Target"}, nil
}

func (datasourcePublishPreviewDependency) FindDatasources(context.Context, string, string) ([]datasourceops.Record, error) {
	return nil, nil
}

func (datasourcePublishPreviewDependency) ResolvePublishedDatasource(context.Context, string, string) (datasourceops.Record, error) {
	return datasourceops.Record{}, nil
}

func (datasourcePublishPreviewDependency) Prepare(context.Context, datasourceops.PublishRequest) (datasourceops.PreparedPublish, error) {
	return nil, nil
}

func TestWorkbookPullDependencyArtifactPassesDatasourcePublishPreflight(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) }
	manager := artifact.NewDatasourceManager(now)
	pulled, err := workbookops.Pull(context.Background(), workbookDependencyPullReader{}, artifactWriter{
		workbooks: artifact.NewWorkbookManager(now),
		bundles:   artifact.NewWorkbookBundleManager(now),
	}, workbookops.PullInput{
		Environment: "source", Site: "source-site", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1",
		Workspace: workspace, Selector: identity.Selector{LUID: "wb-1"}, IncludePDS: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pulled.Artifact.Dependencies) != 1 {
		t.Fatalf("dependencies = %#v", pulled.Artifact.Dependencies)
	}
	dependencyPath := filepath.Join(workspace, filepath.FromSlash(pulled.Artifact.Dependencies[0].Path))
	stored, err := manager.Read(context.Background(), dependencyPath)
	if err != nil {
		t.Fatal(err)
	}
	if stored.CompositionStatus != artifact.CompositionStatusOrdinary || stored.TableauID != "ds-1" || stored.Name != "Sales" ||
		stored.SourceServerOrigin != "https://tableau.example.com" || stored.SourceSiteLUID != "site-1" ||
		stored.SourceEnvironment != "source" || stored.SourceSite != "source-site" ||
		stored.SourceProjectName != "Analytics" || stored.SourceProjectID != "project-1" {
		t.Fatalf("stored dependency artifact = %#v", stored)
	}
	nativeContent, err := os.ReadFile(stored.PayloadPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(nativeContent) != `<datasource><connection class="sqlserver"/></datasource>` {
		t.Fatalf("stored dependency content = %q", nativeContent)
	}

	dependency := datasourcePublishPreviewDependency{}
	preview, err := datasourceops.NewPublish(
		datasourceArtifactReader{manager: manager, displayPath: pulled.Artifact.Dependencies[0].Path},
		dependency,
		dependency,
	).Execute(context.Background(), datasourceops.PublishInput{
		ArtifactPath: dependencyPath, Environment: "target", Site: "target-site",
		ProjectSelector: identity.Selector{LUID: "target-project"}, Mode: datasourceops.ModeCreate,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Plan.CompositionStatus != artifact.CompositionStatusOrdinary || len(preview.Plan.ParentDataSourceURLs) != 0 {
		t.Fatalf("publish preview = %#v", preview.Plan)
	}
}

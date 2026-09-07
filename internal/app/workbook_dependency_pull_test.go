package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/identity"
)

type workbookDependencyPullReader struct{}

func (workbookDependencyPullReader) ResolveWorkbook(context.Context, identity.Selector) (workbookpull.Workbook, error) {
	return workbookpull.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Analytics"}, nil
}

func (workbookDependencyPullReader) DownloadWorkbook(context.Context, string, *bool) (workbookpull.Download, error) {
	return workbookpull.Download{Filename: "Finance.twb", Content: []byte(`<workbook/>`)}, nil
}

func (workbookDependencyPullReader) CaptureWorkbookLineage(context.Context, workbookpull.LineageRequest) (workbookpull.LineageCapture, error) {
	return workbookpull.LineageCapture{
		RootMetadataID: "metadata-wb-1",
		Complete:       true,
		Nodes:          []workbookpull.LineageNode{{MetadataID: "metadata-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"}},
	}, nil
}

func (workbookDependencyPullReader) PublishedDatasources(context.Context, string) ([]workbookpull.PublishedDatasource, error) {
	return []workbookpull.PublishedDatasource{{LUID: "ds-1", Name: "Sales"}}, nil
}

func (workbookDependencyPullReader) DownloadPublishedDatasource(context.Context, string) (workbookpull.DatasourceDownload, error) {
	return workbookpull.DatasourceDownload{
		LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics",
		Filename: "Sales.tds", Content: []byte(`<datasource><connection class="sqlserver"/></datasource>`),
	}, nil
}

type datasourcePublishPreviewDependency struct{}

func (datasourcePublishPreviewDependency) ResolveProject(context.Context, identity.Selector) (datasourcepublish.Project, error) {
	return datasourcepublish.Project{LUID: "target-project", Name: "Target", Path: "Target"}, nil
}

func (datasourcePublishPreviewDependency) FindDatasources(context.Context, string, string) ([]datasourcepublish.Datasource, error) {
	return nil, nil
}

func (datasourcePublishPreviewDependency) ResolvePublishedDatasource(context.Context, string, string) (datasourcepublish.Datasource, error) {
	return datasourcepublish.Datasource{}, nil
}

func (datasourcePublishPreviewDependency) Prepare(context.Context, datasourcepublish.PublishRequest) (datasourcepublish.PreparedPublish, error) {
	return nil, nil
}

func TestWorkbookPullDependencyArtifactPassesDatasourcePublishPreflight(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time { return time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC) }
	manager := artifact.NewDatasourceManager(now)
	pulled, err := workbookpull.New(workbookDependencyPullReader{}, artifactWriter{
		workbooks: artifact.NewWorkbookManager(now),
		bundles:   artifact.NewWorkbookBundleManager(now),
	}).Execute(context.Background(), workbookpull.Input{
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
	preview, err := datasourcepublish.New(
		datasourceArtifactReader{manager: manager, displayPath: pulled.Artifact.Dependencies[0].Path},
		dependency,
		dependency,
	).Execute(context.Background(), datasourcepublish.Input{
		ArtifactPath: dependencyPath, Environment: "target", Site: "target-site",
		ProjectSelector: identity.Selector{LUID: "target-project"}, Mode: datasourcepublish.ModeCreate,
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Plan.CompositionStatus != artifact.CompositionStatusOrdinary || len(preview.Plan.ParentDataSourceURLs) != 0 {
		t.Fatalf("publish preview = %#v", preview.Plan)
	}
}

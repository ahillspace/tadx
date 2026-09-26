package workbook_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type pullReader struct {
	workbook                workbookops.Record
	download                workbookops.Download
	lineage                 workbookops.LineageCapture
	publishedDatasources    []workbookops.PublishedDatasource
	datasourceDownloads     map[string]workbookops.DatasourceDownload
	resolveErr              error
	downloadErr             error
	lineageErr              error
	publishedDatasourcesErr error
	datasourceDownloadErr   error
	resolveCalls            int
	downloadCalls           int
	lineageRequests         []workbookops.LineageRequest
	referenceCalls          int
	datasourceCalls         []string
}

func (r *pullReader) ResolveWorkbook(context.Context, identity.Selector) (workbookops.Record, error) {
	r.resolveCalls++
	return r.workbook, r.resolveErr
}

func (r *pullReader) DownloadWorkbook(context.Context, string, *bool) (workbookops.Download, error) {
	r.downloadCalls++
	return r.download, r.downloadErr
}

func (r *pullReader) CaptureWorkbookLineage(_ context.Context, request workbookops.LineageRequest) (workbookops.LineageCapture, error) {
	r.lineageRequests = append(r.lineageRequests, request)
	if r.lineage.RootMetadataID == "" && !r.lineage.Complete && r.lineage.Direction == "" && r.lineage.Depth == 0 && len(r.lineage.Nodes) == 0 && len(r.lineage.Edges) == 0 && len(r.lineage.Warnings) == 0 && r.lineageErr == nil {
		return workbookops.LineageCapture{
			RootMetadataID: "meta-" + request.RESTLUID,
			Complete:       true,
			Nodes:          []workbookops.LineageNode{{MetadataID: "meta-" + request.RESTLUID, Kind: "workbook", RESTLUID: request.RESTLUID}},
		}, nil
	}
	return r.lineage, r.lineageErr
}

func (r *pullReader) PublishedDatasources(context.Context, string) ([]workbookops.PublishedDatasource, error) {
	r.referenceCalls++
	return r.publishedDatasources, r.publishedDatasourcesErr
}

func (r *pullReader) DownloadPublishedDatasource(_ context.Context, luid string) (workbookops.DatasourceDownload, error) {
	r.datasourceCalls = append(r.datasourceCalls, luid)
	return r.datasourceDownloads[luid], r.datasourceDownloadErr
}

type pullWriter struct {
	input             workbookops.PullArtifact
	result            workbookops.PullArtifactResult
	err               error
	calls             int
	bundleCalls       int
	datasourceInputs  []workbookops.DatasourceArtifact
	datasourceResults map[string]workbookops.DependencyArtifactResult
	datasourceErr     error
}

type pullRetryableReadError struct{}

func (pullRetryableReadError) Error() string            { return "Tableau unavailable" }
func (pullRetryableReadError) Retryable() bool          { return true }
func (pullRetryableReadError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func (w *pullWriter) WriteWorkbook(_ context.Context, input workbookops.PullArtifact) (workbookops.PullArtifactResult, error) {
	w.calls++
	w.input = input
	return w.result, w.err
}

func (w *pullWriter) WritePublishedDatasource(_ context.Context, input workbookops.DatasourceArtifact) (workbookops.DependencyArtifactResult, error) {
	w.datasourceInputs = append(w.datasourceInputs, input)
	if w.datasourceErr != nil {
		return workbookops.DependencyArtifactResult{}, w.datasourceErr
	}
	return w.datasourceResults[input.TableauID], nil
}

func (w *pullWriter) WriteBundle(_ context.Context, workbook workbookops.PullArtifact, datasources []workbookops.DatasourceArtifact) (workbookops.PullArtifactResult, error) {
	w.bundleCalls++
	w.input = workbook
	w.datasourceInputs = append(w.datasourceInputs, datasources...)
	if w.datasourceErr != nil {
		return workbookops.PullArtifactResult{}, w.datasourceErr
	}
	result := w.result
	for _, input := range datasources {
		result.Dependencies = append(result.Dependencies, w.datasourceResults[input.TableauID])
	}
	return result, w.err
}

func TestPullActionRecordsPortableWorkbookWithoutDatasourceDownloads(t *testing.T) {
	r := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download: workbookops.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
	}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"}}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Portability != "portable" || result.Artifact.DependenciesAcquired {
		t.Fatalf("artifact result = %#v", result.Artifact)
	}
	if len(result.Artifact.PublishedDatasources) != 0 || len(result.Artifact.Dependencies) != 0 {
		t.Fatalf("unexpected dependencies = %#v", result.Artifact)
	}
	if r.referenceCalls != 1 || len(r.datasourceCalls) != 0 || len(w.datasourceInputs) != 0 {
		t.Fatalf("calls: references=%d remote=%v writes=%d", r.referenceCalls, r.datasourceCalls, len(w.datasourceInputs))
	}
	if string(w.input.Content) != "native-workbook" || w.input.Portability != "portable" {
		t.Fatalf("workbook artifact input = %#v", w.input)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(compact.String(), "published_datasource_count: 0") {
		t.Fatalf("portable compact output did not report a definitive zero: %s", compact.String())
	}
}

func TestPullActionCapturesBoundedWorkbookLineageAutomatically(t *testing.T) {
	r := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download: workbookops.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineage: workbookops.LineageCapture{
			RootMetadataID: "meta-wb-1",
			Complete:       true,
			Nodes: []workbookops.LineageNode{
				{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"},
				{MetadataID: "meta-ds-1", Kind: "published_datasource", RESTLUID: "ds-1", Name: "Sales"},
			},
			Edges: []workbookops.LineageEdge{{FromMetadataID: "meta-ds-1", ToMetadataID: "meta-wb-1", Relationship: "upstream"}},
		},
	}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.lineageRequests) != 1 || r.lineageRequests[0] != (workbookops.LineageRequest{RESTLUID: "wb-1", Direction: "both", Depth: 1}) {
		t.Fatalf("lineage requests = %#v", r.lineageRequests)
	}
	if !w.input.Lineage.Complete || !w.input.LineageCountsKnown || len(w.input.Lineage.Nodes) != 2 || len(w.input.Lineage.Edges) != 1 {
		t.Fatalf("workbook lineage artifact input = %#v", w.input)
	}
	if result.Artifact.LineageStatus != "complete" || result.Artifact.LineageNodeCount == nil || *result.Artifact.LineageNodeCount != 2 || result.Artifact.LineageEdgeCount == nil || *result.Artifact.LineageEdgeCount != 1 {
		t.Fatalf("artifact lineage result = %#v", result.Artifact)
	}
	var compact bytes.Buffer
	if err := output.Render(&compact, result); err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"lineage", "meta-wb-1", "meta-ds-1"} {
		if strings.Contains(strings.ToLower(compact.String()), omitted) {
			t.Fatalf("compact output exposed lineage detail %q: %s", omitted, compact.String())
		}
	}
}

func TestPullActionPreservesDownloadWhenLineageCaptureIsUnavailable(t *testing.T) {
	r := &pullReader{
		workbook:   workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download:   workbookops.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineageErr: errors.New("credential secret and unbounded upstream diagnostic"),
	}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.calls != 1 || w.input.Lineage.Complete || w.input.LineageCountsKnown || w.input.Lineage.Direction != "both" || w.input.Lineage.Depth != 1 {
		t.Fatalf("workbook lineage artifact input = %#v", w.input)
	}
	if result.Artifact.LineageStatus != "unavailable" || result.Artifact.LineageNodeCount != nil || result.Artifact.LineageEdgeCount != nil {
		t.Fatalf("artifact lineage result = %#v", result.Artifact)
	}
	if compact := result.CompactOutput().(workbookops.PullCompactResult); len(compact.Warnings) != 0 {
		t.Fatalf("optional lineage warning leaked into routine pull: %#v", compact)
	}
	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, "Lineage capture was unavailable") || strings.Contains(joined, "credential secret") || strings.Contains(joined, "unbounded") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestPullActionPreservesPartialWorkbookLineageWhenCaptureFails(t *testing.T) {
	r := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download: workbookops.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineage: workbookops.LineageCapture{
			RootMetadataID: "meta-wb-1",
			Nodes: []workbookops.LineageNode{
				{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"},
				{MetadataID: "meta-ds-1", Kind: "published_datasource", RESTLUID: "ds-1", Name: "Sales"},
			},
			Edges:    []workbookops.LineageEdge{{FromMetadataID: "meta-ds-1", ToMetadataID: "meta-wb-1", Relationship: "upstream"}},
			Warnings: []string{"upstream database access was denied"},
		},
		lineageErr: errors.New("upstream database access was denied"),
	}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.LineageCountsKnown || w.input.Lineage.Complete || len(w.input.Lineage.Nodes) != 2 || len(w.input.Lineage.Edges) != 1 {
		t.Fatalf("workbook lineage artifact input = %#v", w.input)
	}
	if result.Artifact.LineageStatus != "incomplete" || result.Artifact.LineageNodeCount != nil || result.Artifact.LineageEdgeCount != nil {
		t.Fatalf("artifact lineage result = %#v", result.Artifact)
	}
	if !strings.Contains(strings.Join(result.Warnings, "\n"), "upstream database access was denied") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestPullActionAcquiresUniquePublishedDatasourcesWhenRequested(t *testing.T) {
	r := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download: workbookops.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		publishedDatasources: []workbookops.PublishedDatasource{
			{LUID: "ds-2", Name: "Inventory"},
			{LUID: "ds-1", Name: "Sales"},
			{LUID: "ds-1", Name: "Sales"},
		},
		datasourceDownloads: map[string]workbookops.DatasourceDownload{
			"ds-1": {LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Shared", Filename: "Sales.tdsx", Content: []byte("native-sales")},
			"ds-2": {LUID: "ds-2", Name: "Inventory", ProjectLUID: "project-1", ProjectPath: "Shared", Filename: "Inventory.tds", Content: []byte("native-inventory")},
		},
	}
	w := &pullWriter{
		result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"},
		datasourceResults: map[string]workbookops.DependencyArtifactResult{
			"ds-1": {LUID: "ds-1", Path: "artifacts/datasource/Sales-ds1", CanonicalPath: "artifacts/datasource/Sales-ds1/Sales.tdsx", BaselineFingerprint: "sha256:sales"},
			"ds-2": {LUID: "ds-2", Path: "artifacts/datasource/Inventory-ds2", CanonicalPath: "artifacts/datasource/Inventory-ds2/Inventory.tds", BaselineFingerprint: "sha256:inventory"},
		},
	}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}, IncludePDS: true, Overwrite: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.datasourceCalls, ",") != "ds-1,ds-2" {
		t.Fatalf("datasource download order = %v", r.datasourceCalls)
	}
	if w.bundleCalls != 1 || w.calls != 0 || len(w.datasourceInputs) != 2 || string(w.datasourceInputs[0].Content) != "native-sales" || string(w.datasourceInputs[1].Content) != "native-inventory" {
		t.Fatalf("datasource artifact inputs = %#v", w.datasourceInputs)
	}
	if w.datasourceInputs[0].Overwrite || w.datasourceInputs[1].Overwrite {
		t.Fatalf("workbook overwrite authorized dependency overwrite: %#v", w.datasourceInputs)
	}
	if result.Artifact.Portability != "source-site-bound" || !result.Artifact.DependenciesAcquired {
		t.Fatalf("artifact result = %#v", result.Artifact)
	}
	if len(result.Artifact.PublishedDatasources) != 2 || result.Artifact.PublishedDatasources[0].LUID != "ds-1" || result.Artifact.PublishedDatasources[0].LocalArtifactPath != "artifacts/datasource/Sales-ds1" {
		t.Fatalf("references = %#v", result.Artifact.PublishedDatasources)
	}
	if !w.input.DependenciesAcquired || len(w.input.PublishedDatasources) != 2 {
		t.Fatalf("workbook artifact input = %#v", w.input)
	}
}

func TestPullActionLeavesPortabilityUnknownWhenOptionalDetectionIsIncomplete(t *testing.T) {
	r := &pullReader{
		workbook:                workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download:                workbookops.Download{Filename: "Finance.twbx", Content: []byte("native")},
		publishedDatasourcesErr: errors.New("metadata indexing incomplete"),
	}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"}}

	result, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{Environment: "dev", Site: "test-site", Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Portability != "unknown" || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "portability remains unknown") || strings.Contains(result.Warnings[0], "metadata indexing incomplete") {
		t.Fatalf("result = %#v", result)
	}
	if w.calls != 1 || w.input.Portability != "unknown" {
		t.Fatalf("writer calls=%d input=%#v", w.calls, w.input)
	}
	if compact := result.CompactOutput().(workbookops.PullCompactResult); len(compact.Warnings) != 0 || compact.Artifact.Portability != "" {
		t.Fatalf("optional portability diagnostic leaked into routine pull: %#v", compact)
	}
}

func TestPullActionRequiresCompleteDetectionBeforeIncludePDSAcquisition(t *testing.T) {
	r := &pullReader{
		workbook:                workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download:                workbookops.Download{Filename: "Finance.twbx", Content: []byte("native")},
		publishedDatasourcesErr: pullRetryableReadError{},
	}
	w := &pullWriter{}

	_, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{Environment: "dev", Site: "test-site", Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}, IncludePDS: true})
	payload := errs.Structure(err).Error
	if payload.ID != "workbook.pull.references" || payload.Retryable == nil || !*payload.Retryable {
		t.Fatalf("structured error = %#v", payload)
	}
	if w.calls != 0 || len(w.datasourceInputs) != 0 {
		t.Fatalf("local writes occurred: workbook=%d datasources=%d", w.calls, len(w.datasourceInputs))
	}
}

func TestPullQuietPullPreservesNativeArtifactWarnings(t *testing.T) {
	r := &pullReader{workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"}, download: workbookops.Download{Filename: "Finance.twb", Content: []byte("native")}, lineageErr: errors.New("unavailable"), publishedDatasourcesErr: errors.New("unavailable")}
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: "artifact", Warnings: []string{"Local edits were replaced because --overwrite was set."}}}
	output, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(workbookops.PullCompactResult)
	if len(compact.Warnings) != 1 || compact.Warnings[0] != w.result.Warnings[0] {
		t.Fatalf("native warning hidden: %#v", compact)
	}
	if full := output.FullOutput().(workbookops.PullFullResult); len(full.Warnings) != 3 || full.Artifact.Portability != "unknown" {
		t.Fatalf("full diagnostics lost: %#v", full)
	}
}

func TestPullActionPullsOneResolvedWorkbookIntoArtifact(t *testing.T) {
	w := &pullWriter{result: workbookops.PullArtifactResult{Path: filepath.FromSlash("C:/workspace/artifacts/workbook/Finance"), BaselineFingerprint: "sha256:abc"}}
	actionReader, actionWriter := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
		download: workbookops.Download{Filename: "Finance.twbx", Content: []byte("native")},
	}, w
	include := false
	output, err := workbookops.Pull(context.Background(), actionReader, actionWriter, workbookops.PullInput{
		Environment: "production", Site: "marketing", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1", Workspace: filepath.FromSlash("C:/workspace"),
		Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}, IncludeExtract: &include,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.Workbook.LUID != "wb-1" || output.Artifact.BaselineFingerprint != "sha256:abc" || w.input.TableauID != "wb-1" {
		t.Fatalf("output = %#v, artifact input = %#v", output, w.input)
	}
	if w.input.ServerOrigin != "https://tableau.example.com" || w.input.SiteLUID != "site-1" {
		t.Fatalf("source identity was not preserved: %#v", w.input)
	}
}

func TestPullActionRejectsMissingSelectorAsUsageBeforeResolving(t *testing.T) {
	r := &pullReader{}
	w := &pullWriter{}
	_, err := workbookops.Pull(context.Background(), r, w, workbookops.PullInput{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, ProjectPath: "Ops"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("Execute() error = %#v", err)
	}
	if r.resolveCalls != 0 || r.downloadCalls != 0 || w.calls != 0 {
		t.Fatalf("collaborators invoked: resolve=%d download=%d write=%d", r.resolveCalls, r.downloadCalls, w.calls)
	}
}

func TestPullActionLeavesArtifactWriterUntouchedWhenDownloadFails(t *testing.T) {
	r := &pullReader{workbook: workbookops.Record{LUID: "wb-1"}, downloadErr: errors.New("download forbidden")}
	w := &pullWriter{}
	actionReader, actionWriter := r, w
	_, err := workbookops.Pull(context.Background(), actionReader, actionWriter, workbookops.PullInput{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, Selector: identity.Selector{LUID: "wb-1"}})
	if err == nil || !strings.Contains(err.Error(), "download forbidden") || r.downloadCalls != 1 || w.calls != 0 {
		t.Fatalf("error = %v, download calls = %d, write calls = %d", err, r.downloadCalls, w.calls)
	}
}

func TestPullActionPreservesRetryAdviceForWorkbookReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader *pullReader
	}{
		{name: "resolve", reader: &pullReader{resolveErr: pullRetryableReadError{}}},
		{name: "download", reader: &pullReader{workbook: workbookops.Record{LUID: "wb-1"}, downloadErr: pullRetryableReadError{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := workbookops.Pull(context.Background(), test.reader, &pullWriter{}, workbookops.PullInput{Environment: "production", Site: "marketing", Selector: identity.Selector{LUID: "wb-1"}})
			payload := errs.Structure(err).Error
			if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
				t.Fatalf("structured error = %#v", payload)
			}
		})
	}
}

func TestPullActionCompletesArtifactWriteErrorAdvice(t *testing.T) {
	actionReader, actionWriter := &pullReader{
		workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		download: workbookops.Download{Filename: "Finance.twb", Content: []byte("native")},
	}, &pullWriter{err: errors.New("artifact is dirty")}
	_, err := workbookops.Pull(context.Background(), actionReader, actionWriter, workbookops.PullInput{Environment: "production", Site: "marketing", Selector: identity.Selector{LUID: "wb-1"}})
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "" || payload.Operation != "workbook.pull" {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestPullActionGoldenOutput(t *testing.T) {
	value, err := workbookops.Pull( // The artifact manager returns runtime absolute paths.
		// Public output must project them relative to the selected workspace.

		context.Background(), &pullReader{
			workbook:             workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
			download:             workbookops.Download{Filename: "Finance.twbx", Content: []byte("native"), TableauRequestID: "request-1"},
			publishedDatasources: []workbookops.PublishedDatasource{{LUID: "ds-1", Name: "Sales"}},
			datasourceDownloads: map[string]workbookops.DatasourceDownload{
				"ds-1": {LUID: "ds-1", Name: "Sales", ProjectLUID: "datasource-project", ProjectPath: "Shared", Filename: "Sales.tdsx", Content: []byte("native-datasource")},
			},
		}, &pullWriter{result: workbookops.PullArtifactResult{

			Path:                filepath.FromSlash("C:/workspace/artifacts/workbook/Finance"),
			CanonicalPath:       filepath.FromSlash("C:/workspace/artifacts/workbook/Finance/Finance.twbx"),
			LineagePath:         filepath.FromSlash("C:/workspace/artifacts/workbook/Finance/lineage.json"),
			BaselineFingerprint: "sha256:abc",
			Warnings:            []string{"existing clean artifact replaced"},
		}, datasourceResults: map[string]workbookops.DependencyArtifactResult{
			"ds-1": {LUID: "ds-1", Name: "Sales", Path: "artifacts/datasource/Sales", CanonicalPath: filepath.FromSlash("C:/workspace/artifacts/datasource/Sales/Sales.tdsx"), BaselineFingerprint: "sha256:def"},
		}}, workbookops.PullInput{
			Environment: "production", Site: "marketing", Workspace: filepath.FromSlash("C:/workspace"), WorkspaceName: "logical workspace",
			Selector: identity.Selector{LUID: "wb-1"}, IncludePDS: true,
		})
	if err != nil {
		t.Fatal(err)
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/pull/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}

	actual.Reset()
	if err := output.RenderWithOptions(&actual, value, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	expected, err = os.ReadFile("testdata/pull/output_full.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("full golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}

func TestPullCompactOutputIsBoundedAndOmitsUnknownDatasourceCount(t *testing.T) {
	references := make([]workbookops.PublishedDatasourceRef, 100)
	for index := range references {
		references[index] = workbookops.PublishedDatasourceRef{LUID: fmt.Sprintf("ds-%03d", index), Name: strings.Repeat("name", 20)}
	}
	value := workbookops.PullOutput{
		Status:   "pulled",
		Workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		Artifact: workbookops.PullArtifactResult{Path: "artifact", Portability: "unknown", PublishedDatasources: references},
		Help:     []string{"next"},
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, value); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rendered.String(), "published_datasource_count") || strings.Contains(rendered.String(), "ds-099") || rendered.Len() > 500 {
		t.Fatalf("compact output is not bounded or claimed an unknown count: %s", rendered.String())
	}
}

func TestPullFullOutputBoundsDatasourceDetailsAndWarnings(t *testing.T) {
	references := make([]workbookops.PublishedDatasourceRef, 60)
	for index := range references {
		references[index] = workbookops.PublishedDatasourceRef{LUID: fmt.Sprintf("ds-%03d", index), Name: "Datasource"}
	}
	warnings := make([]string, 30)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("warning-%02d", index)
	}
	value := workbookops.PullOutput{
		Status:   "pulled",
		Workbook: workbookops.Record{LUID: "wb-1", Name: "Finance"},
		Artifact: workbookops.PullArtifactResult{Path: "artifact", Portability: "source-site-bound", PublishedDatasources: references},
		Warnings: warnings,
		Help:     []string{"next"},
	}
	var rendered bytes.Buffer
	if err := output.RenderWithOptions(&rendered, value, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"published_datasource_count: 60", "published_datasources[50]", "published_datasources_omitted: 10", "warnings[20]", "warnings_omitted: 10"} {
		if !strings.Contains(rendered.String(), want) {
			t.Fatalf("full output missing %q: %s", want, rendered.String())
		}
	}
	for _, omitted := range []string{"ds-059", "warning-29"} {
		if strings.Contains(rendered.String(), omitted) {
			t.Fatalf("full output exceeded its bound with %q: %s", omitted, rendered.String())
		}
	}
}

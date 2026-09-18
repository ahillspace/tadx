package pull_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	workbook                pull.Workbook
	download                pull.Download
	lineage                 pull.LineageCapture
	publishedDatasources    []pull.PublishedDatasource
	datasourceDownloads     map[string]pull.DatasourceDownload
	resolveErr              error
	downloadErr             error
	lineageErr              error
	publishedDatasourcesErr error
	datasourceDownloadErr   error
	resolveCalls            int
	downloadCalls           int
	lineageRequests         []pull.LineageRequest
	referenceCalls          int
	datasourceCalls         []string
}

func (r *reader) ResolveWorkbook(context.Context, identity.Selector) (pull.Workbook, error) {
	r.resolveCalls++
	return r.workbook, r.resolveErr
}

func (r *reader) DownloadWorkbook(context.Context, string, *bool) (pull.Download, error) {
	r.downloadCalls++
	return r.download, r.downloadErr
}

func (r *reader) CaptureWorkbookLineage(_ context.Context, request pull.LineageRequest) (pull.LineageCapture, error) {
	r.lineageRequests = append(r.lineageRequests, request)
	if r.lineage.RootMetadataID == "" && !r.lineage.Complete && r.lineage.Direction == "" && r.lineage.Depth == 0 && len(r.lineage.Nodes) == 0 && len(r.lineage.Edges) == 0 && len(r.lineage.Warnings) == 0 && r.lineageErr == nil {
		return pull.LineageCapture{
			RootMetadataID: "meta-" + request.RESTLUID,
			Complete:       true,
			Nodes:          []pull.LineageNode{{MetadataID: "meta-" + request.RESTLUID, Kind: "workbook", RESTLUID: request.RESTLUID}},
		}, nil
	}
	return r.lineage, r.lineageErr
}

func (r *reader) PublishedDatasources(context.Context, string) ([]pull.PublishedDatasource, error) {
	r.referenceCalls++
	return r.publishedDatasources, r.publishedDatasourcesErr
}

func (r *reader) DownloadPublishedDatasource(_ context.Context, luid string) (pull.DatasourceDownload, error) {
	r.datasourceCalls = append(r.datasourceCalls, luid)
	return r.datasourceDownloads[luid], r.datasourceDownloadErr
}

type writer struct {
	input             pull.Artifact
	result            pull.ArtifactResult
	err               error
	calls             int
	bundleCalls       int
	datasourceInputs  []pull.DatasourceArtifact
	datasourceResults map[string]pull.DependencyArtifactResult
	datasourceErr     error
}

type retryableReadError struct{}

func (retryableReadError) Error() string            { return "Tableau unavailable" }
func (retryableReadError) Retryable() bool          { return true }
func (retryableReadError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func (w *writer) WriteWorkbook(_ context.Context, input pull.Artifact) (pull.ArtifactResult, error) {
	w.calls++
	w.input = input
	return w.result, w.err
}

func (w *writer) WritePublishedDatasource(_ context.Context, input pull.DatasourceArtifact) (pull.DependencyArtifactResult, error) {
	w.datasourceInputs = append(w.datasourceInputs, input)
	if w.datasourceErr != nil {
		return pull.DependencyArtifactResult{}, w.datasourceErr
	}
	return w.datasourceResults[input.TableauID], nil
}

func (w *writer) WriteBundle(_ context.Context, workbook pull.Artifact, datasources []pull.DatasourceArtifact) (pull.ArtifactResult, error) {
	w.bundleCalls++
	w.input = workbook
	w.datasourceInputs = append(w.datasourceInputs, datasources...)
	if w.datasourceErr != nil {
		return pull.ArtifactResult{}, w.datasourceErr
	}
	result := w.result
	for _, input := range datasources {
		result.Dependencies = append(result.Dependencies, w.datasourceResults[input.TableauID])
	}
	return result, w.err
}

func TestActionRecordsPortableWorkbookWithoutDatasourceDownloads(t *testing.T) {
	r := &reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
	}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"}}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{
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

func TestActionCapturesBoundedWorkbookLineageAutomatically(t *testing.T) {
	r := &reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineage: pull.LineageCapture{
			RootMetadataID: "meta-wb-1",
			Complete:       true,
			Nodes: []pull.LineageNode{
				{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"},
				{MetadataID: "meta-ds-1", Kind: "published_datasource", RESTLUID: "ds-1", Name: "Sales"},
			},
			Edges: []pull.LineageEdge{{FromMetadataID: "meta-ds-1", ToMetadataID: "meta-wb-1", Relationship: "upstream"}},
		},
	}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{
		Environment: "dev", Site: "test-site", SiteLUID: "site-1", ServerOrigin: "https://tableau.example.com",
		Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.lineageRequests) != 1 || r.lineageRequests[0] != (pull.LineageRequest{RESTLUID: "wb-1", Direction: "both", Depth: 1}) {
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

func TestActionPreservesDownloadWhenLineageCaptureIsUnavailable(t *testing.T) {
	r := &reader{
		workbook:   pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download:   pull.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineageErr: errors.New("credential secret and unbounded upstream diagnostic"),
	}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{
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
	if compact := result.CompactOutput().(pull.CompactResult); len(compact.Warnings) != 0 {
		t.Fatalf("optional lineage warning leaked into routine pull: %#v", compact)
	}
	joined := strings.Join(result.Warnings, "\n")
	if !strings.Contains(joined, "Lineage capture was unavailable") || strings.Contains(joined, "credential secret") || strings.Contains(joined, "unbounded") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestActionPreservesPartialWorkbookLineageWhenCaptureFails(t *testing.T) {
	r := &reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		lineage: pull.LineageCapture{
			RootMetadataID: "meta-wb-1",
			Nodes: []pull.LineageNode{
				{MetadataID: "meta-wb-1", Kind: "workbook", RESTLUID: "wb-1", Name: "Finance"},
				{MetadataID: "meta-ds-1", Kind: "published_datasource", RESTLUID: "ds-1", Name: "Sales"},
			},
			Edges:    []pull.LineageEdge{{FromMetadataID: "meta-ds-1", ToMetadataID: "meta-wb-1", Relationship: "upstream"}},
			Warnings: []string{"upstream database access was denied"},
		},
		lineageErr: errors.New("upstream database access was denied"),
	}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook", LineagePath: "artifact/lineage.json"}}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{
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

func TestActionAcquiresUniquePublishedDatasourcesWhenRequested(t *testing.T) {
	r := &reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native-workbook")},
		publishedDatasources: []pull.PublishedDatasource{
			{LUID: "ds-2", Name: "Inventory"},
			{LUID: "ds-1", Name: "Sales"},
			{LUID: "ds-1", Name: "Sales"},
		},
		datasourceDownloads: map[string]pull.DatasourceDownload{
			"ds-1": {LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Shared", Filename: "Sales.tdsx", Content: []byte("native-sales")},
			"ds-2": {LUID: "ds-2", Name: "Inventory", ProjectLUID: "project-1", ProjectPath: "Shared", Filename: "Inventory.tds", Content: []byte("native-inventory")},
		},
	}
	w := &writer{
		result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"},
		datasourceResults: map[string]pull.DependencyArtifactResult{
			"ds-1": {LUID: "ds-1", Path: "artifacts/datasource/Sales-ds1", CanonicalPath: "artifacts/datasource/Sales-ds1/Sales.tdsx", BaselineFingerprint: "sha256:sales"},
			"ds-2": {LUID: "ds-2", Path: "artifacts/datasource/Inventory-ds2", CanonicalPath: "artifacts/datasource/Inventory-ds2/Inventory.tds", BaselineFingerprint: "sha256:inventory"},
		},
	}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{
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

func TestActionLeavesPortabilityUnknownWhenOptionalDetectionIsIncomplete(t *testing.T) {
	r := &reader{
		workbook:                pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download:                pull.Download{Filename: "Finance.twbx", Content: []byte("native")},
		publishedDatasourcesErr: errors.New("metadata indexing incomplete"),
	}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", BaselineFingerprint: "sha256:workbook"}}

	result, err := pull.New(r, w).Execute(context.Background(), pull.Input{Environment: "dev", Site: "test-site", Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Artifact.Portability != "unknown" || len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "portability remains unknown") || strings.Contains(result.Warnings[0], "metadata indexing incomplete") {
		t.Fatalf("result = %#v", result)
	}
	if w.calls != 1 || w.input.Portability != "unknown" {
		t.Fatalf("writer calls=%d input=%#v", w.calls, w.input)
	}
	if compact := result.CompactOutput().(pull.CompactResult); len(compact.Warnings) != 0 || compact.Artifact.Portability != "" {
		t.Fatalf("optional portability diagnostic leaked into routine pull: %#v", compact)
	}
}

func TestActionRequiresCompleteDetectionBeforeIncludePDSAcquisition(t *testing.T) {
	r := &reader{
		workbook:                pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download:                pull.Download{Filename: "Finance.twbx", Content: []byte("native")},
		publishedDatasourcesErr: retryableReadError{},
	}
	w := &writer{}

	_, err := pull.New(r, w).Execute(context.Background(), pull.Input{Environment: "dev", Site: "test-site", Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}, IncludePDS: true})
	payload := errs.Structure(err).Error
	if payload.ID != "workbook.pull.references" || payload.Retryable == nil || !*payload.Retryable {
		t.Fatalf("structured error = %#v", payload)
	}
	if w.calls != 0 || len(w.datasourceInputs) != 0 {
		t.Fatalf("local writes occurred: workbook=%d datasources=%d", w.calls, len(w.datasourceInputs))
	}
}

func TestQuietPullPreservesNativeArtifactWarnings(t *testing.T) {
	r := &reader{workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"}, download: pull.Download{Filename: "Finance.twb", Content: []byte("native")}, lineageErr: errors.New("unavailable"), publishedDatasourcesErr: errors.New("unavailable")}
	w := &writer{result: pull.ArtifactResult{Path: "artifact", Warnings: []string{"Local edits were replaced because --overwrite was set."}}}
	output, err := pull.New(r, w).Execute(context.Background(), pull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "wb-1"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(pull.CompactResult)
	if len(compact.Warnings) != 1 || compact.Warnings[0] != w.result.Warnings[0] {
		t.Fatalf("native warning hidden: %#v", compact)
	}
	if full := output.FullOutput().(pull.FullResult); len(full.Warnings) != 3 || full.Artifact.Portability != "unknown" {
		t.Fatalf("full diagnostics lost: %#v", full)
	}
}

func TestActionPullsOneResolvedWorkbookIntoArtifact(t *testing.T) {
	w := &writer{result: pull.ArtifactResult{Path: filepath.FromSlash("C:/workspace/artifacts/workbook/Finance"), BaselineFingerprint: "sha256:abc"}}
	action := pull.New(&reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native")},
	}, w)
	include := false
	output, err := action.Execute(context.Background(), pull.Input{
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

func TestActionRejectsMissingSelectorAsUsageBeforeResolving(t *testing.T) {
	r := &reader{}
	w := &writer{}
	_, err := pull.New(r, w).Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, ProjectPath: "Ops"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("Execute() error = %#v", err)
	}
	if r.resolveCalls != 0 || r.downloadCalls != 0 || w.calls != 0 {
		t.Fatalf("collaborators invoked: resolve=%d download=%d write=%d", r.resolveCalls, r.downloadCalls, w.calls)
	}
}

func TestActionLeavesArtifactWriterUntouchedWhenDownloadFails(t *testing.T) {
	r := &reader{workbook: pull.Workbook{LUID: "wb-1"}, downloadErr: errors.New("download forbidden")}
	w := &writer{}
	action := pull.New(r, w)
	_, err := action.Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, Selector: identity.Selector{LUID: "wb-1"}})
	if err == nil || !strings.Contains(err.Error(), "download forbidden") || r.downloadCalls != 1 || w.calls != 0 {
		t.Fatalf("error = %v, download calls = %d, write calls = %d", err, r.downloadCalls, w.calls)
	}
}

func TestActionPreservesRetryAdviceForWorkbookReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader *reader
	}{
		{name: "resolve", reader: &reader{resolveErr: retryableReadError{}}},
		{name: "download", reader: &reader{workbook: pull.Workbook{LUID: "wb-1"}, downloadErr: retryableReadError{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pull.New(test.reader, &writer{}).Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Selector: identity.Selector{LUID: "wb-1"}})
			payload := errs.Structure(err).Error
			if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
				t.Fatalf("structured error = %#v", payload)
			}
		})
	}
}

func TestActionCompletesArtifactWriteErrorAdvice(t *testing.T) {
	action := pull.New(&reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		download: pull.Download{Filename: "Finance.twb", Content: []byte("native")},
	}, &writer{err: errors.New("artifact is dirty")})
	_, err := action.Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Selector: identity.Selector{LUID: "wb-1"}})
	payload := errs.Structure(err).Error
	if payload.Retryable == nil || *payload.Retryable || payload.CorrectiveAction == "" || payload.Operation != "workbook.pull" {
		t.Fatalf("structured error = %#v", payload)
	}
}

func TestActionGoldenOutput(t *testing.T) {
	value, err := pull.New(&reader{
		workbook:             pull.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
		download:             pull.Download{Filename: "Finance.twbx", Content: []byte("native"), TableauRequestID: "request-1"},
		publishedDatasources: []pull.PublishedDatasource{{LUID: "ds-1", Name: "Sales"}},
		datasourceDownloads: map[string]pull.DatasourceDownload{
			"ds-1": {LUID: "ds-1", Name: "Sales", ProjectLUID: "datasource-project", ProjectPath: "Shared", Filename: "Sales.tdsx", Content: []byte("native-datasource")},
		},
	}, &writer{result: pull.ArtifactResult{
		// The artifact manager returns runtime absolute paths.
		// Public output must project them relative to the selected workspace.
		Path:                filepath.FromSlash("C:/workspace/artifacts/workbook/Finance"),
		CanonicalPath:       filepath.FromSlash("C:/workspace/artifacts/workbook/Finance/Finance.twbx"),
		LineagePath:         filepath.FromSlash("C:/workspace/artifacts/workbook/Finance/lineage.json"),
		BaselineFingerprint: "sha256:abc",
		Warnings:            []string{"existing clean artifact replaced"},
	}, datasourceResults: map[string]pull.DependencyArtifactResult{
		"ds-1": {LUID: "ds-1", Name: "Sales", Path: "artifacts/datasource/Sales", CanonicalPath: filepath.FromSlash("C:/workspace/artifacts/datasource/Sales/Sales.tdsx"), BaselineFingerprint: "sha256:def"},
	}}).Execute(context.Background(), pull.Input{
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
	expected, err := os.ReadFile("testdata/output.toon")
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
	expected, err = os.ReadFile("testdata/output_full.toon")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("full golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}

func TestCompactOutputIsBoundedAndOmitsUnknownDatasourceCount(t *testing.T) {
	references := make([]pull.PublishedDatasourceRef, 100)
	for index := range references {
		references[index] = pull.PublishedDatasourceRef{LUID: fmt.Sprintf("ds-%03d", index), Name: strings.Repeat("name", 20)}
	}
	value := pull.Output{
		Status:   "pulled",
		Workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		Artifact: pull.ArtifactResult{Path: "artifact", Portability: "unknown", PublishedDatasources: references},
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

func TestFullOutputBoundsDatasourceDetailsAndWarnings(t *testing.T) {
	references := make([]pull.PublishedDatasourceRef, 60)
	for index := range references {
		references[index] = pull.PublishedDatasourceRef{LUID: fmt.Sprintf("ds-%03d", index), Name: "Datasource"}
	}
	warnings := make([]string, 30)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("warning-%02d", index)
	}
	value := pull.Output{
		Status:   "pulled",
		Workbook: pull.Workbook{LUID: "wb-1", Name: "Finance"},
		Artifact: pull.ArtifactResult{Path: "artifact", Portability: "source-site-bound", PublishedDatasources: references},
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

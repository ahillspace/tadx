package pull_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	"github.com/ahillspace/tadx/internal/identity"
)

type pullReader struct {
	lineageErr     error
	lineage        datasourcepull.Lineage
	partialLineage bool
	calls          []string
}

func (r *pullReader) ResolveDatasource(_ context.Context, selector identity.Selector) (datasourcepull.Datasource, error) {
	r.calls = append(r.calls, "resolve:"+string(selector.LUID))
	return datasourcepull.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}, nil
}

func (r *pullReader) DownloadDatasource(_ context.Context, luid string) (datasourcepull.Download, error) {
	r.calls = append(r.calls, "download:"+luid)
	return datasourcepull.Download{Filename: "Sales.tds", Content: []byte(`<datasource><relation datasource-url="parent-sales"/></datasource>`), TableauRequestID: "request-1"}, nil
}

func (r *pullReader) CaptureLineage(_ context.Context, request datasourcepull.LineageRequest) (datasourcepull.Lineage, error) {
	r.calls = append(r.calls, "lineage:"+request.RESTLUID)
	if r.lineageErr != nil {
		return r.lineage, r.lineageErr
	}
	if r.partialLineage {
		return datasourcepull.Lineage{Complete: false, Direction: "both", Depth: 1, Nodes: []datasourcepull.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}}, nil
	}
	return datasourcepull.Lineage{Complete: true, Direction: "both", Depth: 1, Nodes: []datasourcepull.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}}, nil
}

type pullWriter struct {
	input    datasourcepull.Artifact
	warnings []string
}

func (w *pullWriter) WriteDatasource(_ context.Context, input datasourcepull.Artifact) (datasourcepull.ArtifactResult, error) {
	w.input = input
	return datasourcepull.ArtifactResult{
		Path: "artifacts/datasource/Sales", CanonicalPath: "artifacts/datasource/Sales/Sales.tds",
		LineagePath: "artifacts/datasource/Sales/lineage.json", BaselineFingerprint: "sha256:abc",
		CompositionStatus: "composed", ParentDataSourceURLs: []string{"parent-sales"}, LineageStatus: "complete",
		NodeCount: 1, CountsKnown: true,
		Warnings: w.warnings,
	}, nil
}

func TestPullPreservesNativePackageAndCompositionReferences(t *testing.T) {
	r := &pullReader{}
	w := &pullWriter{}
	output, err := datasourcepull.New(r, w).Execute(context.Background(), datasourcepull.Input{
		Environment: "dev", Site: "sandbox", ServerOrigin: "https://tableau.example", SiteLUID: "site-1",
		Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(r.calls, []string{"resolve:ds-1", "download:ds-1", "lineage:ds-1"}) {
		t.Fatalf("calls = %#v", r.calls)
	}
	if string(w.input.Content) != `<datasource><relation datasource-url="parent-sales"/></datasource>` || w.input.Filename != "Sales.tds" {
		t.Fatalf("artifact input = %#v", w.input)
	}
	if output.Artifact.CompositionStatus != "composed" || !reflect.DeepEqual(output.Artifact.ParentDataSourceURLs, []string{"parent-sales"}) {
		t.Fatalf("output = %#v", output)
	}
}

func TestPullKeepsSuccessfulDownloadWhenLineageIsUnavailable(t *testing.T) {
	r := &pullReader{lineageErr: errors.New("metadata disabled")}
	w := &pullWriter{}
	output, err := datasourcepull.New(r, w).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.Lineage.Complete || len(output.Warnings) != 1 || !strings.Contains(output.Warnings[0], "Lineage capture was incomplete") {
		t.Fatalf("output = %#v, lineage = %#v", output, w.input.Lineage)
	}
	if compact := output.CompactOutput().(datasourcepull.CompactResult); len(compact.Warnings) != 0 {
		t.Fatalf("optional lineage was noisy: %#v", compact)
	}
}

func TestPullPreservesPartialLineageWhenCaptureFails(t *testing.T) {
	r := &pullReader{
		lineage: datasourcepull.Lineage{
			Nodes: []datasourcepull.LineageNode{
				{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"},
				{MetadataID: "metadata-table-1", Kind: "table", Name: "Sales"},
			},
			Edges:    []datasourcepull.LineageEdge{{FromMetadataID: "metadata-table-1", ToMetadataID: "metadata-ds-1", Relationship: "upstream"}},
			Warnings: []string{"upstream database access was denied"},
		},
		lineageErr: errors.New("upstream database access was denied"),
	}
	w := &pullWriter{}
	output, err := datasourcepull.New(r, w).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.Lineage.Complete || len(w.input.Lineage.Nodes) != 2 || len(w.input.Lineage.Edges) != 1 || output.Artifact.CountsKnown {
		t.Fatalf("output = %#v, lineage = %#v", output, w.input.Lineage)
	}
	if !strings.Contains(strings.Join(output.Warnings, "\n"), "upstream database access was denied") {
		t.Fatalf("warnings = %#v", output.Warnings)
	}
}

func TestQuietDatasourcePullPreservesNativeWarnings(t *testing.T) {
	w := &pullWriter{warnings: []string{"dirty datasource artifact replaced because --overwrite was provided"}}
	output, err := datasourcepull.New(&pullReader{lineageErr: errors.New("unavailable")}, w).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(datasourcepull.CompactResult)
	if len(compact.Warnings) != 1 || compact.Warnings[0] != w.warnings[0] {
		t.Fatalf("native warning hidden: %#v", compact)
	}
	if full := output.FullOutput().(datasourcepull.FullResult); len(full.Warnings) != 2 || full.Artifact.LineageStatus != "incomplete" {
		t.Fatalf("full diagnostics lost: %#v", full)
	}
}

func TestPullRejectsAbsoluteArtifactPaths(t *testing.T) {
	w := &absolutePullWriter{}
	_, err := datasourcepull.New(&pullReader{}, w).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err == nil || !strings.Contains(err.Error(), "absolute canonical path") {
		t.Fatalf("error = %v", err)
	}
}

type absolutePullWriter struct{}

func (*absolutePullWriter) WriteDatasource(context.Context, datasourcepull.Artifact) (datasourcepull.ArtifactResult, error) {
	return datasourcepull.ArtifactResult{Path: "artifacts/datasource/Sales", CanonicalPath: `C:\workspace\Sales.tds`}, nil
}

// pathPullWriter returns a caller-supplied artifact path so path normalization can be exercised directly.
type pathPullWriter struct{ path string }

func (w *pathPullWriter) WriteDatasource(context.Context, datasourcepull.Artifact) (datasourcepull.ArtifactResult, error) {
	return datasourcepull.ArtifactResult{Path: w.path}, nil
}

func TestPullSingleSourcesLineageStatusAndCountsKnown(t *testing.T) {
	cases := []struct {
		name       string
		partial    bool
		lineageErr error
		wantStatus string
		wantKnown  bool
	}{
		{name: "complete", wantStatus: "complete", wantKnown: true},
		{name: "partial without error", partial: true, wantStatus: "incomplete", wantKnown: false},
		{name: "capture error", lineageErr: errors.New("metadata disabled"), wantStatus: "incomplete", wantKnown: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			r := &pullReader{partialLineage: test.partial, lineageErr: test.lineageErr}
			output, err := datasourcepull.New(r, &pullWriter{}).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
			if err != nil {
				t.Fatal(err)
			}
			if output.Artifact.LineageStatus != test.wantStatus || output.Artifact.CountsKnown != test.wantKnown {
				t.Fatalf("lineage_status = %q, counts_known = %v; want %q, %v", output.Artifact.LineageStatus, output.Artifact.CountsKnown, test.wantStatus, test.wantKnown)
			}
			// The two fields must never disagree: incomplete implies counts unknown.
			if (output.Artifact.LineageStatus == "incomplete") == output.Artifact.CountsKnown {
				t.Fatalf("lineage_status %q and counts_known %v are contradictory", output.Artifact.LineageStatus, output.Artifact.CountsKnown)
			}
		})
	}
}

func TestPullNormalizesTraversalWithinWorkspaceAndRejectsEscapes(t *testing.T) {
	output, err := datasourcepull.New(&pullReader{}, &pathPullWriter{path: "a/b/../c"}).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatalf("expected a/b/../c to be accepted, got error %v", err)
	}
	if output.Artifact.Path != "a/c" {
		t.Fatalf("normalized path = %q, want %q", output.Artifact.Path, "a/c")
	}
	for _, escaping := range []string{"../x", "a/../../x"} {
		_, err := datasourcepull.New(&pullReader{}, &pathPullWriter{path: escaping}).Execute(context.Background(), datasourcepull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
		if err == nil || !strings.Contains(err.Error(), "escaping") {
			t.Fatalf("path %q: error = %v, want escaping rejection", escaping, err)
		}
	}
}

package datasource_test

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	"reflect"
	"strings"
	"testing"
)

type pullPullReader struct {
	lineageErr     error
	lineage        datasourceops.Lineage
	partialLineage bool
	calls          []string
}

func (r *pullPullReader) ResolveDatasource(_ context.Context, selector identity.Selector) (datasourceops.Record, error) {
	r.calls = append(r.calls, "resolve:"+string(selector.LUID))
	return datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}, nil
}

func (r *pullPullReader) DownloadDatasource(_ context.Context, luid string) (datasourceops.Download, error) {
	r.calls = append(r.calls, "download:"+luid)
	return datasourceops.Download{Filename: "Sales.tds", Content: []byte(`<datasource><relation datasource-url="parent-sales"/></datasource>`), TableauRequestID: "request-1"}, nil
}

func (r *pullPullReader) CaptureLineage(_ context.Context, request datasourceops.LineageRequest) (datasourceops.Lineage, error) {
	r.calls = append(r.calls, "lineage:"+request.RESTLUID)
	if r.lineageErr != nil {
		return r.lineage, r.lineageErr
	}
	if r.partialLineage {
		return datasourceops.Lineage{Complete: false, Direction: "both", Depth: 1, Nodes: []datasourceops.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}}, nil
	}
	return datasourceops.Lineage{Complete: true, Direction: "both", Depth: 1, Nodes: []datasourceops.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}}, nil
}

type pullPullWriter struct {
	input    datasourceops.PullArtifact
	warnings []string
}

func (w *pullPullWriter) WriteDatasource(_ context.Context, input datasourceops.PullArtifact) (datasourceops.PullArtifactResult, error) {
	w.input = input
	return datasourceops.PullArtifactResult{
		Path: "artifacts/datasource/Sales", CanonicalPath: "artifacts/datasource/Sales/Sales.tds",
		LineagePath: "artifacts/datasource/Sales/lineage.json", BaselineFingerprint: "sha256:abc",
		CompositionStatus: "composed", ParentDataSourceURLs: []string{"parent-sales"}, LineageStatus: "complete",
		NodeCount: 1, CountsKnown: true,
		Warnings: w.warnings,
	}, nil
}

func TestPullPullPreservesNativePackageAndCompositionReferences(t *testing.T) {
	r := &pullPullReader{}
	w := &pullPullWriter{}
	output, err := datasourceops.Pull(context.Background(), r, w, datasourceops.PullInput{
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

func TestPullPullKeepsSuccessfulDownloadWhenLineageIsUnavailable(t *testing.T) {
	r := &pullPullReader{lineageErr: errors.New("metadata disabled")}
	w := &pullPullWriter{}
	output, err := datasourceops.Pull(context.Background(), r, w, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.Lineage.Complete || len(output.Warnings) != 1 || !strings.Contains(output.Warnings[0], "Lineage capture was incomplete") {
		t.Fatalf("output = %#v, lineage = %#v", output, w.input.Lineage)
	}
	if compact := output.CompactOutput().(datasourceops.PullCompactResult); len(compact.Warnings) != 0 {
		t.Fatalf("optional lineage was noisy: %#v", compact)
	}
}

func TestPullPullPreservesPartialLineageWhenCaptureFails(t *testing.T) {
	r := &pullPullReader{
		lineage: datasourceops.Lineage{
			Nodes: []datasourceops.LineageNode{
				{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"},
				{MetadataID: "metadata-table-1", Kind: "table", Name: "Sales"},
			},
			Edges:    []datasourceops.LineageEdge{{FromMetadataID: "metadata-table-1", ToMetadataID: "metadata-ds-1", Relationship: "upstream"}},
			Warnings: []string{"upstream database access was denied"},
		},
		lineageErr: errors.New("upstream database access was denied"),
	}
	w := &pullPullWriter{}
	output, err := datasourceops.Pull(context.Background(), r, w, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
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

func TestPullQuietDatasourcePullPreservesNativeWarnings(t *testing.T) {
	w := &pullPullWriter{warnings: []string{"dirty datasource artifact replaced because --overwrite was provided"}}
	output, err := datasourceops.Pull(context.Background(), &pullPullReader{lineageErr: errors.New("unavailable")}, w, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(datasourceops.PullCompactResult)
	if len(compact.Warnings) != 1 || compact.Warnings[0] != w.warnings[0] {
		t.Fatalf("native warning hidden: %#v", compact)
	}
	if full := output.FullOutput().(datasourceops.PullFullResult); len(full.Warnings) != 2 || full.Artifact.LineageStatus != "incomplete" {
		t.Fatalf("full diagnostics lost: %#v", full)
	}
}

func TestPullPullRejectsAbsoluteArtifactPaths(t *testing.T) {
	w := &pullAbsolutePullWriter{}
	_, err := datasourceops.Pull(context.Background(), &pullPullReader{}, w, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err == nil || !strings.Contains(err.Error(), "absolute canonical path") {
		t.Fatalf("error = %v", err)
	}
}

type pullAbsolutePullWriter struct{}

func (*pullAbsolutePullWriter) WriteDatasource(context.Context, datasourceops.PullArtifact) (datasourceops.PullArtifactResult, error) {
	return datasourceops.PullArtifactResult{Path: "artifacts/datasource/Sales", CanonicalPath: `C:\workspace\Sales.tds`}, nil
}

// pullPathPullWriter returns a caller-supplied artifact path so path normalization can be exercised directly.
type pullPathPullWriter struct{ path string }

func (w *pullPathPullWriter) WriteDatasource(context.Context, datasourceops.PullArtifact) (datasourceops.PullArtifactResult, error) {
	return datasourceops.PullArtifactResult{Path: w.path}, nil
}

func TestPullPullSingleSourcesLineageStatusAndCountsKnown(t *testing.T) {
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
			r := &pullPullReader{partialLineage: test.partial, lineageErr: test.lineageErr}
			output, err := datasourceops.Pull(context.Background(), r, &pullPullWriter{}, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
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

func TestPullPullNormalizesTraversalWithinWorkspaceAndRejectsEscapes(t *testing.T) {
	output, err := datasourceops.Pull(context.Background(), &pullPullReader{}, &pullPathPullWriter{path: "a/b/../c"}, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatalf("expected a/b/../c to be accepted, got error %v", err)
	}
	if output.Artifact.Path != "a/c" {
		t.Fatalf("normalized path = %q, want %q", output.Artifact.Path, "a/c")
	}
	for _, escaping := range []string{"../x", "a/../../x"} {
		_, err := datasourceops.Pull(context.Background(), &pullPullReader{}, &pullPathPullWriter{path: escaping}, datasourceops.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "ds-1"}})
		if err == nil || !strings.Contains(err.Error(), "escaping") {
			t.Fatalf("path %q: error = %v, want escaping rejection", escaping, err)
		}
	}
}

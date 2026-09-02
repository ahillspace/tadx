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
	lineageErr error
	calls      []string
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
		return datasourcepull.Lineage{}, r.lineageErr
	}
	return datasourcepull.Lineage{Complete: true, Direction: "both", Depth: 1, Nodes: []datasourcepull.LineageNode{{MetadataID: "metadata-ds-1", Kind: "published_datasource", RESTLUID: "ds-1"}}}, nil
}

type pullWriter struct {
	input datasourcepull.Artifact
}

func (w *pullWriter) WriteDatasource(_ context.Context, input datasourcepull.Artifact) (datasourcepull.ArtifactResult, error) {
	w.input = input
	return datasourcepull.ArtifactResult{
		Path: "artifacts/datasource/Sales", CanonicalPath: "artifacts/datasource/Sales/Sales.tds",
		LineagePath: "artifacts/datasource/Sales/lineage.json", BaselineFingerprint: "sha256:abc",
		CompositionStatus: "composed", ParentDataSourceURLs: []string{"parent-sales"}, LineageStatus: "complete",
		NodeCount: 1, CountsKnown: true,
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

package pull_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct{ resource lineagepull.Resource }

func (r resolver) ResolveLineageResource(context.Context, string, identity.Selector) (lineagepull.Resource, error) {
	return r.resource, nil
}

func TestOutputGolden(t *testing.T) {
	output := lineagepull.Output{
		Status:    "pulled",
		Resource:  lineagepull.Resource{Kind: "flow", LUID: "flow-1", MetadataID: "metadata-flow-1", Name: "Daily", ProjectPath: "Ops"},
		Artifact:  lineagepull.ArtifactResult{Path: "artifacts/lineage/flow/Daily", LineagePath: "artifacts/lineage/flow/Daily/lineage.json", Fingerprint: "sha256:abc"},
		Direction: "both", Depth: 1, Complete: true, CountsKnown: true,
		Nodes:    []lineagepull.Node{{MetadataID: "metadata-flow-1", Kind: "flow", RESTLUID: "flow-1", Name: "Daily"}, {MetadataID: "metadata-datasource-1", Kind: "published_datasource", RESTLUID: "datasource-1", Name: "Sales"}},
		Edges:    []lineagepull.Edge{{FromMetadataID: "metadata-flow-1", ToMetadataID: "metadata-datasource-1", Relationship: "uses"}},
		Warnings: []string{"One bounded warning."}, Provenance: lineagepull.Provenance{Environment: "dev", Site: "sandbox", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1"}, RequestIDs: []string{"request-1"},
		Help: []string{"tadx catalog lineage pull --kind flow --id flow-1 --direction both"},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func TestOutputReportsUnknownCountsAndBoundOmissions(t *testing.T) {
	warnings := make([]string, 21)
	requests := make([]string, 21)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("warning-%02d", index)
		requests[index] = fmt.Sprintf("request-%02d", index)
	}
	output, err := lineagepull.New(resolver{resource: lineagepull.Resource{Kind: "flow", LUID: "flow-1", Name: "Daily"}}, reader{graph: lineagepull.Graph{Complete: true, Warnings: warnings, RequestIDs: requests}}, &writer{}).Execute(context.Background(), lineagepull.Input{Workspace: "workspace", Kind: "flow", Selector: identity.Selector{LUID: "flow-1"}})
	if err != nil {
		t.Fatal(err)
	}
	full := output.FullOutput().(lineagepull.FullResult)
	if len(full.Warnings) != 20 || len(full.RequestIDs) != 20 || full.WarningsOmitted != 1 || full.RequestIDsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
	unknown := (lineagepull.Output{CountsKnown: false}).CompactOutput().(lineagepull.CompactResult)
	if unknown.Artifact.NodeCount != nil || unknown.Artifact.EdgeCount != nil {
		t.Fatalf("compact = %#v", unknown)
	}
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type reader struct {
	graph lineagepull.Graph
	err   error
}

func (r reader) CaptureLineage(context.Context, lineagepull.CaptureRequest) (lineagepull.Graph, error) {
	return r.graph, r.err
}

type writer struct{ input lineagepull.Artifact }

func (w *writer) WriteLineage(_ context.Context, input lineagepull.Artifact) (lineagepull.ArtifactResult, error) {
	w.input = input
	return lineagepull.ArtifactResult{Path: "artifacts/lineage/flow/Daily", LineagePath: "artifacts/lineage/flow/Daily/lineage.json", Fingerprint: "sha256:abc"}, nil
}

func TestActionCreatesCompactAndBoundedFullProjections(t *testing.T) {
	nodes := make([]lineagepull.Node, lineagepull.FullNodeLimit+1)
	for index := range nodes {
		nodes[index] = lineagepull.Node{MetadataID: "node-" + indexText(index), Kind: "flow"}
	}
	nodes[0].RESTLUID = "flow-1"
	graph := lineagepull.Graph{RootMetadataID: nodes[0].MetadataID, Complete: true, Nodes: nodes, RequestIDs: []string{"request-1"}}
	w := &writer{}
	output, err := lineagepull.New(resolver{resource: lineagepull.Resource{Kind: "flow", LUID: "flow-1", Name: "Daily", ProjectPath: "Department/Ops"}}, reader{graph: graph}, w).Execute(context.Background(), lineagepull.Input{
		Environment: "dev", Site: "sandbox", ServerOrigin: "https://tableau.example.com", SiteLUID: "site-1", Workspace: "workspace", Kind: "flow", Selector: identity.Selector{LUID: "flow-1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(lineagepull.CompactResult)
	if compact.Resource.LUID != "flow-1" || compact.Artifact.Direction != "both" || compact.Artifact.Depth != 1 || compact.Artifact.NodeCount == nil || *compact.Artifact.NodeCount != len(nodes) || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
	full := output.FullOutput().(lineagepull.FullResult)
	if len(full.Nodes) != lineagepull.FullNodeLimit || full.OmittedNodeCount != 1 || full.Nodes[0].MetadataID == full.Nodes[0].RESTLUID || len(full.RequestIDs) != 1 || full.Artifact.Direction != "both" || full.Artifact.LineagePath == "" {
		t.Fatalf("full = %#v", full)
	}
	if w.input.Resource.MetadataID != nodes[0].MetadataID || w.input.Direction != "both" || w.input.Depth != 1 {
		t.Fatalf("artifact input = %#v", w.input)
	}
}

func TestActionPersistsExplicitIncompleteCapture(t *testing.T) {
	w := &writer{}
	output, err := lineagepull.New(resolver{resource: lineagepull.Resource{Kind: "workbook", LUID: "wb-1", Name: "Book"}}, reader{err: errors.New("permission limited")}, w).Execute(context.Background(), lineagepull.Input{Workspace: "workspace", Kind: "workbook", Selector: identity.Selector{LUID: "wb-1"}, Direction: "upstream", Depth: 2})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.Complete || len(output.Warnings) == 0 || output.CompactOutput().(lineagepull.CompactResult).Artifact.Complete {
		t.Fatalf("output = %#v, artifact = %#v", output, w.input)
	}
}

func TestActionValidatesRootDirectionAndDepthBeforeDependencies(t *testing.T) {
	action := lineagepull.New(resolver{}, reader{}, &writer{})
	for _, input := range []lineagepull.Input{
		{Workspace: "workspace", Kind: "sheet", Selector: identity.Selector{LUID: "x"}},
		{Workspace: "workspace", Kind: "flow"},
		{Workspace: "workspace", Kind: "flow", Selector: identity.Selector{LUID: "x"}, Direction: "sideways"},
		{Workspace: "workspace", Kind: "flow", Selector: identity.Selector{LUID: "x"}, Depth: 4},
	} {
		if _, err := action.Execute(context.Background(), input); err == nil {
			t.Fatalf("expected validation error for %#v", input)
		}
	}
}

func indexText(index int) string {
	const digits = "0123456789"
	return string([]byte{digits[(index/100)%10], digits[(index/10)%10], digits[index%10]})
}

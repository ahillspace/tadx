package pull_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type reader struct{}

func (reader) ResolveFlow(context.Context, identity.Selector) (flowpull.Flow, error) {
	return flowpull.Flow{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Ops", FileType: "tflx"}, nil
}

func TestOutputGoldenKeepsAutomaticLineageDetailsFullOnly(t *testing.T) {
	output := flowpull.Output{Status: "pulled", Flow: flowpull.Flow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops", FileType: "tflx"}, Artifact: flowpull.ArtifactResult{Path: "artifacts/flow/Daily", CanonicalPath: "artifacts/flow/Daily/Daily.tflx", BaselineFingerprint: "sha256:abc", LineagePath: "artifacts/flow/Daily/lineage.json", LineageStatus: "complete", NodeCount: 2, EdgeCount: 1, CountsKnown: true}, Warnings: []string{"One bounded warning."}, RequestID: "request-1", Help: []string{"tadx content flow publish --artifact artifacts/flow/Daily"}}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
	var compact bytes.Buffer
	if err := render.RenderWithOptions(&compact, output, render.Options{}); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"lineage_path", "node_count", "edge_count", "baseline_fingerprint", "tableau_request_id"} {
		if strings.Contains(compact.String(), forbidden) {
			t.Fatalf("compact output contains %q: %s", forbidden, compact.String())
		}
	}
}

func TestOutputRepresentsUnknownLineageCountsAndBoundsWarnings(t *testing.T) {
	warnings := make([]string, 21)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("warning-%02d", index)
	}
	full := (flowpull.Output{Artifact: flowpull.ArtifactResult{LineageStatus: "incomplete"}, Warnings: warnings}).FullOutput().(flowpull.FullResult)
	if full.Artifact.NodeCount != nil || full.Artifact.EdgeCount != nil || len(full.Warnings) != 20 || full.WarningsOmitted != 1 {
		t.Fatalf("full = %#v", full)
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
func (reader) DownloadFlow(context.Context, string) (flowpull.Download, error) {
	return flowpull.Download{Filename: "Daily.tflx", Content: []byte("native\x00flow")}, nil
}
func (reader) CaptureLineage(context.Context, flowpull.LineageRequest) (flowpull.Lineage, error) {
	return flowpull.Lineage{Complete: true, Nodes: []flowpull.LineageNode{{MetadataID: "m-1", Kind: "flow", RESTLUID: "f-1"}}}, nil
}

type writer struct {
	input  flowpull.Artifact
	result flowpull.ArtifactResult
}

func (w *writer) WriteFlow(_ context.Context, input flowpull.Artifact) (flowpull.ArtifactResult, error) {
	w.input = input
	if w.result.Path != "" {
		return w.result, nil
	}
	return flowpull.ArtifactResult{Path: "artifacts/flow/Daily", LineagePath: "artifacts/flow/Daily/lineage.json", LineageStatus: "complete", NodeCount: 1}, nil
}

func TestActionPullsNativeFlowWithBoundedLineage(t *testing.T) {
	w := &writer{}
	output, err := flowpull.New(reader{}, w).Execute(context.Background(), flowpull.Input{Environment: "dev", Site: "site", Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(w.input.Content) != "native\x00flow" || w.input.Filename != "Daily.tflx" {
		t.Fatalf("artifact input = %#v", w.input)
	}
	compact := output.CompactOutput().(flowpull.CompactResult)
	if compact.Artifact.Path != "artifacts/flow/Daily" {
		t.Fatalf("compact = %#v", output.CompactOutput())
	}
	if output.FullOutput().(flowpull.FullResult).Artifact.LineageStatus != "complete" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestActionRejectsAbsoluteWriterPaths(t *testing.T) {
	w := &writer{result: flowpull.ArtifactResult{Path: "artifacts/flow/Daily", CanonicalPath: `C:\workspace\artifacts\flow\Daily\Daily.tflx`}}
	_, err := flowpull.New(reader{}, w).Execute(context.Background(), flowpull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err == nil || !strings.Contains(err.Error(), "absolute canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestActionNormalizesWriterPathsAndPropagatesWarnings(t *testing.T) {
	w := &writer{result: flowpull.ArtifactResult{Path: `artifacts\flow\Daily`, CanonicalPath: `artifacts\flow\Daily\Daily.tflx`, LineagePath: `artifacts\flow\Daily\lineage.json`, Warnings: []string{"writer warning"}}}
	output, err := flowpull.New(reader{}, w).Execute(context.Background(), flowpull.Input{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.Artifact.CanonicalPath != "artifacts/flow/Daily/Daily.tflx" || len(output.Warnings) != 1 {
		t.Fatalf("output = %#v", output)
	}
}

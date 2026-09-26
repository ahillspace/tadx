package flow_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	flowpull "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type pullReader struct{}

type pullUnavailableLineageReader struct{ pullReader }

func (pullUnavailableLineageReader) CaptureLineage(context.Context, flowpull.PullLineageRequest) (flowpull.PullLineage, error) {
	return flowpull.PullLineage{}, fmt.Errorf("metadata unavailable")
}

type pullPartialLineageReader struct{ pullReader }

func (pullPartialLineageReader) CaptureLineage(context.Context, flowpull.PullLineageRequest) (flowpull.PullLineage, error) {
	return flowpull.PullLineage{
		Nodes: []flowpull.PullLineageNode{
			{MetadataID: "metadata-flow-1", Kind: "flow", RESTLUID: "f-1"},
			{MetadataID: "metadata-table-1", Kind: "table", Name: "Sales"},
		},
		Edges:    []flowpull.PullLineageEdge{{FromMetadataID: "metadata-table-1", ToMetadataID: "metadata-flow-1", Relationship: "upstream"}},
		Warnings: []string{"upstream database access was denied"},
	}, fmt.Errorf("upstream database access was denied")
}

func TestPullPullPreservesPartialLineageWhenCaptureFails(t *testing.T) {
	w := &pullWriter{}
	output, err := flowpull.Pull(context.Background(), pullPartialLineageReader{}, w, flowpull.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
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

func TestPullQuietFlowPullPreservesNativeWarnings(t *testing.T) {
	w := &pullWriter{result: flowpull.PullArtifactResult{Path: "artifacts/flow/Daily", Warnings: []string{"Local flow edits were replaced because --overwrite was set."}}}
	output, err := flowpull.Pull(context.Background(), pullUnavailableLineageReader{}, w, flowpull.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	compact := output.CompactOutput().(flowpull.PullCompactResult)
	if len(compact.Warnings) != 1 || compact.Warnings[0] != w.result.Warnings[0] {
		t.Fatalf("native warning hidden: %#v", compact)
	}
	if full := output.FullOutput().(flowpull.PullFullResult); len(full.Warnings) != 2 || full.Artifact.LineageStatus != "incomplete" {
		t.Fatalf("full diagnostics lost: %#v", full)
	}
}

func (pullReader) ResolveFlow(context.Context, identity.Selector) (flowpull.Record, error) {
	return flowpull.Record{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Ops", FileType: "tflx"}, nil
}

func TestPullOutputGoldenKeepsAutomaticLineageDetailsFullOnly(t *testing.T) {
	output := flowpull.PullOutput{Status: "pulled", Flow: flowpull.Record{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops", FileType: "tflx"}, Artifact: flowpull.PullArtifactResult{Path: "artifacts/flow/Daily", CanonicalPath: "artifacts/flow/Daily/Daily.tflx", BaselineFingerprint: "sha256:abc", LineagePath: "artifacts/flow/Daily/lineage.json", LineageStatus: "complete", NodeCount: 2, EdgeCount: 1, CountsKnown: true}, Warnings: []string{"One bounded warning."}, RequestID: "request-1", Help: []string{"tadx content flow publish --artifact artifacts/flow/Daily"}}
	pullAssertGolden(t, "compact.toon", output, false)
	pullAssertGolden(t, "full.toon", output, true)
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

func TestPullOutputRepresentsUnknownLineageCountsAndBoundsWarnings(t *testing.T) {
	warnings := make([]string, 21)
	for index := range warnings {
		warnings[index] = fmt.Sprintf("warning-%02d", index)
	}
	full := (flowpull.PullOutput{Artifact: flowpull.PullArtifactResult{LineageStatus: "incomplete"}, Warnings: warnings}).FullOutput().(flowpull.PullFullResult)
	if full.Artifact.NodeCount != nil || full.Artifact.EdgeCount != nil || len(full.Warnings) != 20 || full.WarningsOmitted != 1 {
		t.Fatalf("full = %#v", full)
	}
}

func pullAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "pull", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}
func (pullReader) DownloadFlow(context.Context, string) (flowpull.PullDownload, error) {
	return flowpull.PullDownload{Filename: "Daily.tflx", Content: []byte("native\x00flow")}, nil
}
func (pullReader) CaptureLineage(context.Context, flowpull.PullLineageRequest) (flowpull.PullLineage, error) {
	return flowpull.PullLineage{Complete: true, Nodes: []flowpull.PullLineageNode{{MetadataID: "m-1", Kind: "flow", RESTLUID: "f-1"}}}, nil
}

type pullWriter struct {
	input  flowpull.PullArtifact
	result flowpull.PullArtifactResult
}

func (w *pullWriter) WriteFlow(_ context.Context, input flowpull.PullArtifact) (flowpull.PullArtifactResult, error) {
	w.input = input
	if w.result.Path != "" {
		return w.result, nil
	}
	return flowpull.PullArtifactResult{Path: "artifacts/flow/Daily", LineagePath: "artifacts/flow/Daily/lineage.json", LineageStatus: "complete", NodeCount: 1}, nil
}

func TestPullActionPullsNativeFlowWithBoundedLineage(t *testing.T) {
	w := &pullWriter{}
	output, err := flowpull.Pull(context.Background(), pullReader{}, w, flowpull.PullInput{Environment: "dev", Site: "site", Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if string(w.input.Content) != "native\x00flow" || w.input.Filename != "Daily.tflx" {
		t.Fatalf("artifact input = %#v", w.input)
	}
	compact := output.CompactOutput().(flowpull.PullCompactResult)
	if compact.Artifact.Path != "artifacts/flow/Daily" {
		t.Fatalf("compact = %#v", output.CompactOutput())
	}
	if output.FullOutput().(flowpull.PullFullResult).Artifact.LineageStatus != "complete" {
		t.Fatalf("full = %#v", output.FullOutput())
	}
}

func TestPullActionRejectsAbsoluteWriterPaths(t *testing.T) {
	w := &pullWriter{result: flowpull.PullArtifactResult{Path: "artifacts/flow/Daily", CanonicalPath: `C:\workspace\artifacts\flow\Daily\Daily.tflx`}}
	_, err := flowpull.Pull(context.Background(), pullReader{}, w, flowpull.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err == nil || !strings.Contains(err.Error(), "absolute canonical path") {
		t.Fatalf("error = %v", err)
	}
}

func TestPullActionNormalizesWriterPathsAndPropagatesWarnings(t *testing.T) {
	w := &pullWriter{result: flowpull.PullArtifactResult{Path: `artifacts\flow\Daily`, CanonicalPath: `artifacts\flow\Daily\Daily.tflx`, LineagePath: `artifacts\flow\Daily\lineage.json`, Warnings: []string{"writer warning"}}}
	output, err := flowpull.Pull(context.Background(), pullReader{}, w, flowpull.PullInput{Workspace: "workspace", Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.Artifact.CanonicalPath != "artifacts/flow/Daily/Daily.tflx" || len(output.Warnings) != 1 {
		t.Fatalf("output = %#v", output)
	}
}

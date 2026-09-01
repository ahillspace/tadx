package publish_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type artifactReader struct{ artifact flowpublish.Artifact }

func (r artifactReader) ReadFlow(context.Context, string) (flowpublish.Artifact, error) {
	return r.artifact, nil
}

func TestOutputGolden(t *testing.T) {
	output := flowpublish.Output{Plan: flowpublish.Plan{Mode: "preview", Operation: "flow.publish", ArtifactPath: "artifacts/flow/Daily", ArtifactFingerprint: "sha256:abc", Filename: "Daily.tflx", FlowName: "Daily", Target: flowpublish.Target{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ProjectPath: "Ops"}, Substeps: []string{"resolve exact destination", "publish flow"}}, Applied: true, Result: &flowpublish.Result{Status: "succeeded", FlowLUID: "flow-1", FlowName: "Daily", ProjectLUID: "project-1", TableauRequestID: "request-1"}, Help: []string{"tadx content flow get --id flow-1"}}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
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

type resolver struct {
	project                 flowpublish.Project
	collisions              []flowpublish.Flow
	resolveCalls, findCalls int
}

func (r *resolver) ResolveProject(context.Context, identity.Selector) (flowpublish.Project, error) {
	r.resolveCalls++
	return r.project, nil
}
func (r *resolver) FindFlows(context.Context, string, string) ([]flowpublish.Flow, error) {
	r.findCalls++
	return r.collisions, nil
}

type prepared struct{ committed bool }

func (p *prepared) Commit(context.Context) (flowpublish.Result, error) {
	p.committed = true
	return flowpublish.Result{Status: "succeeded", FlowLUID: "new-flow"}, nil
}

type publisher struct {
	calls    int
	prepared *prepared
}

func (p *publisher) Prepare(context.Context, flowpublish.PublishRequest) (flowpublish.PreparedPublish, error) {
	p.calls++
	p.prepared = &prepared{}
	return p.prepared, nil
}

func TestPreviewDoesNotPrepareOrCommitPublish(t *testing.T) {
	r := &resolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}}
	p := &publisher{}
	output, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifacts/flow/Daily", PayloadPath: "Daily.tflx", Filename: "Daily.tflx", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", Site: "site", ArtifactPath: "artifacts/flow/Daily", Name: "Copy", ProjectSelector: identity.Selector{LUID: "p-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Applied || p.calls != 0 || output.Plan.Target.ProjectLUID != "p-1" {
		t.Fatalf("output=%#v calls=%d", output, p.calls)
	}
}

func TestApplyPreparesThenRevalidatesAndCommits(t *testing.T) {
	r := &resolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}}
	p := &publisher{}
	output, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", Site: "site", ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !output.Applied || p.calls != 1 || !p.prepared.committed || r.resolveCalls != 2 || r.findCalls != 2 {
		t.Fatalf("output=%#v resolver=%#v publisher=%#v", output, r, p)
	}
}

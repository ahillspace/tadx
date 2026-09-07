package publish_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	flowpublish "github.com/ahillspace/tadx/actions/flow/publish"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type artifactReader struct{ artifact flowpublish.Artifact }

func (r artifactReader) ReadFlow(context.Context, string) (flowpublish.Artifact, error) {
	return r.artifact, nil
}

func TestOutputGolden(t *testing.T) {
	output := flowpublish.Output{Plan: flowpublish.Plan{Mode: "execute", Operation: "flow.publish", ArtifactPath: "artifacts/flow/Daily", ArtifactFingerprint: "sha256:abc", Filename: "Daily.tflx", FlowName: "Daily", Target: flowpublish.Target{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ProjectPath: "Ops"}, Substeps: []string{"resolve exact destination", "publish flow"}}, Result: &flowpublish.Result{Status: "succeeded", FlowLUID: "flow-1", FlowName: "Daily", ProjectLUID: "project-1", TableauRequestID: "request-1"}, Help: []string{"tadx content flow inspect --id flow-1"}}
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
	output, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifacts/flow/Daily", PayloadPath: "Daily.tflx", Filename: "Daily.tflx", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", Site: "site", ArtifactPath: "artifacts/flow/Daily", Name: "Copy", ProjectSelector: identity.Selector{LUID: "p-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || p.calls != 0 || output.Plan.Target.ProjectLUID != "p-1" {
		t.Fatalf("output=%#v calls=%d", output, p.calls)
	}
}

func TestPublishDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		r := &resolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}}
		p := &publisher{}
		out, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", TargetResolved: resolved, ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, true)
		if (err == nil) != resolved || p.calls != 0 {
			t.Fatalf("resolved=%t output=%#v err=%v publisher=%#v", resolved, out, err, p)
		}
		if !resolved && r.resolveCalls != 0 {
			t.Fatalf("unresolved target reached Tableau: %d reads", r.resolveCalls)
		}
	}
}

func TestPublishRevalidatesThenPreparesAndCommits(t *testing.T) {
	r := &resolver{project: flowpublish.Project{LUID: "p-1", Path: "Ops"}}
	p := &publisher{}
	output, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", Site: "site", ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || p.calls != 1 || !p.prepared.committed || r.resolveCalls != 2 || r.findCalls != 2 {
		t.Fatalf("output=%#v resolver=%#v publisher=%#v", output, r, p)
	}
}

// driftingResolver returns a different destination project on its second
// resolution, simulating the destination changing between planning and mutation.
type driftingResolver struct {
	first, second flowpublish.Project
	collisions    []flowpublish.Flow
	resolveCalls  int
}

func (r *driftingResolver) ResolveProject(context.Context, identity.Selector) (flowpublish.Project, error) {
	r.resolveCalls++
	if r.resolveCalls == 1 {
		return r.first, nil
	}
	return r.second, nil
}
func (r *driftingResolver) FindFlows(context.Context, string, string) ([]flowpublish.Flow, error) {
	return r.collisions, nil
}

func TestPublishDoesNotPrepareWhenRevalidationFails(t *testing.T) {
	r := &driftingResolver{first: flowpublish.Project{LUID: "p-1", Path: "Ops"}, second: flowpublish.Project{LUID: "p-2", Path: "Ops"}}
	p := &publisher{}
	_, err := flowpublish.New(artifactReader{artifact: flowpublish.Artifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), flowpublish.Input{Environment: "dev", Site: "site", ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, false)
	if err == nil {
		t.Fatal("expected revalidation to fail when the destination changed during execution")
	}
	if p.calls != 0 {
		t.Fatalf("prepare must not upload when revalidation fails: prepare calls=%d", p.calls)
	}
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "flow.publish.target_changed" {
		t.Fatalf("unexpected error: %#v", err)
	}
}

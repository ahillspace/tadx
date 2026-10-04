package flow

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type publishArtifactReader struct{ artifact PublishArtifact }

func (r publishArtifactReader) ReadFlow(context.Context, string) (PublishArtifact, error) {
	return r.artifact, nil
}

func TestPublishOutputGolden(t *testing.T) {
	output := PublishOutput{Plan: PublishPlan{Mode: "execute", Operation: "flow.publish", ArtifactPath: "artifacts/flow/Daily", ArtifactFingerprint: "sha256:abc", Filename: "Daily.tflx", FlowName: "Daily", Target: PublishTarget{Environment: "dev", Site: "sandbox", ProjectLUID: "project-1", ProjectPath: "Ops"}, Substeps: []string{"resolve exact destination", "publish flow"}}, Result: &PublishResult{Status: "succeeded", FlowLUID: "flow-1", FlowName: "Daily", ProjectLUID: "project-1", TableauRequestID: "request-1"}, Help: []string{"tadx content flow inspect --id flow-1"}}
	publishAssertGolden(t, "compact.toon", output, false)
	publishAssertGolden(t, "full.toon", output, true)
}

func publishAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "publish", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type publishResolver struct {
	project                 Project
	collisions              []Record
	resolveCalls, findCalls int
	phaseCalls              int
}

func (r *publishResolver) BeginProjectResolution(ctx context.Context) context.Context {
	r.phaseCalls++
	return ctx
}

func (r *publishResolver) ResolveProject(context.Context, identity.Selector) (Project, error) {
	r.resolveCalls++
	return r.project, nil
}
func (r *publishResolver) FindFlows(context.Context, string, string) ([]Record, error) {
	r.findCalls++
	return r.collisions, nil
}

type publishPrepared struct{ committed bool }

func (p *publishPrepared) Commit(context.Context) (PublishResult, error) {
	p.committed = true
	return PublishResult{Status: "succeeded", FlowLUID: "new-flow"}, nil
}

type publishPublisher struct {
	calls    int
	prepared *publishPrepared
}

func (p *publishPublisher) Prepare(context.Context, PublishRequest) (PreparedPublish, error) {
	p.calls++
	p.prepared = &publishPrepared{}
	return p.prepared, nil
}

func TestPublishPreviewDoesNotPrepareOrCommitPublish(t *testing.T) {
	r := &publishResolver{project: Project{LUID: "p-1", Path: "Ops"}}
	p := &publishPublisher{}
	output, err := newPublisher(publishArtifactReader{artifact: PublishArtifact{Path: "artifacts/flow/Daily", PayloadPath: "Daily.tflx", Filename: "Daily.tflx", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), PublishInput{Environment: "dev", Site: "site", ArtifactPath: "artifacts/flow/Daily", Name: "Copy", ProjectSelector: identity.Selector{LUID: "p-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result != nil || p.calls != 0 || output.Plan.Target.ProjectLUID != "p-1" {
		t.Fatalf("output=%#v calls=%d", output, p.calls)
	}
}

func TestPublishPublishDefaultSiteRequiresResolvedTarget(t *testing.T) {
	for _, resolved := range []bool{false, true} {
		r := &publishResolver{project: Project{LUID: "p-1", Path: "Ops"}}
		p := &publishPublisher{}
		out, err := newPublisher(publishArtifactReader{artifact: PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), PublishInput{Environment: "dev", TargetResolved: resolved, ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, true)
		if (err == nil) != resolved || p.calls != 0 {
			t.Fatalf("resolved=%t output=%#v err=%v publisher=%#v", resolved, out, err, p)
		}
		if !resolved && r.resolveCalls != 0 {
			t.Fatalf("unresolved target reached Tableau: %d reads", r.resolveCalls)
		}
	}
}

func TestPublishPublishRevalidatesThenPreparesAndCommits(t *testing.T) {
	r := &publishResolver{project: Project{LUID: "p-1", Path: "Ops"}}
	p := &publishPublisher{}
	output, err := newPublisher(publishArtifactReader{artifact: PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), PublishInput{Environment: "dev", Site: "site", ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if output.Result == nil || p.calls != 1 || !p.prepared.committed || r.phaseCalls != 2 || r.resolveCalls != 2 || r.findCalls != 2 {
		t.Fatalf("output=%#v resolver=%#v publisher=%#v", output, r, p)
	}
}

// driftingResolver returns a different destination project on its second
// resolution, simulating the destination changing between planning and mutation.
type publishDriftingResolver struct {
	first, second Project
	collisions    []Record
	resolveCalls  int
}

func (r *publishDriftingResolver) BeginProjectResolution(ctx context.Context) context.Context {
	return ctx
}

func (r *publishDriftingResolver) ResolveProject(context.Context, identity.Selector) (Project, error) {
	r.resolveCalls++
	if r.resolveCalls == 1 {
		return r.first, nil
	}
	return r.second, nil
}
func (r *publishDriftingResolver) FindFlows(context.Context, string, string) ([]Record, error) {
	return r.collisions, nil
}

func TestPublishPublishDoesNotPrepareWhenRevalidationFails(t *testing.T) {
	r := &publishDriftingResolver{first: Project{LUID: "p-1", Path: "Ops"}, second: Project{LUID: "p-2", Path: "Ops"}}
	p := &publishPublisher{}
	_, err := newPublisher(publishArtifactReader{artifact: PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Size: 10, Name: "Daily", Fingerprint: "sha256:x"}}, r, p).Execute(context.Background(), PublishInput{Environment: "dev", Site: "site", ArtifactPath: "artifact", ProjectSelector: identity.Selector{LUID: "p-1"}}, false)
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

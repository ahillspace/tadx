package move_test

import (
	"bytes"
	"context"
	flowmove "github.com/ahillspace/tadx/actions/flow/move"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
)

type resolver struct {
	flow                    flowmove.Flow
	project                 flowmove.Project
	flowCalls, projectCalls int
}

func TestOutputGolden(t *testing.T) {
	output := flowmove.Output{Plan: flowmove.Plan{Mode: "preview", Operation: "flow.move", Environment: "dev", Site: "sandbox", Source: flowmove.Flow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Old"}, Destination: flowmove.Project{LUID: "project-2", Name: "New", Path: "New"}}, Applied: true, Result: &flowmove.Result{Status: "succeeded", FlowLUID: "flow-1", ProjectLUID: "project-2", TableauRequestID: "request-1"}, Help: []string{"tadx content flow get --id flow-1"}}
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

func (r *resolver) ResolveFlow(context.Context, identity.Selector) (flowmove.Flow, error) {
	r.flowCalls++
	return r.flow, nil
}
func (r *resolver) ResolveProject(context.Context, identity.Selector) (flowmove.Project, error) {
	r.projectCalls++
	return r.project, nil
}

type mover struct{ calls int }

func (m *mover) MoveFlow(context.Context, string, string) (flowmove.Result, error) {
	m.calls++
	return flowmove.Result{Status: "succeeded", FlowLUID: "f-1", ProjectLUID: "p-2"}, nil
}
func TestMovePreviewsThenRevalidatesOnApply(t *testing.T) {
	r := &resolver{flow: flowmove.Flow{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Old"}, project: flowmove.Project{LUID: "p-2", Path: "New"}}
	m := &mover{}
	a := flowmove.New(r, m)
	preview, err := a.Execute(context.Background(), flowmove.Input{Environment: "dev", Site: "site", FlowSelector: identity.Selector{LUID: "f-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || m.calls != 0 {
		t.Fatalf("preview=%#v", preview)
	}
	applied, err := a.Execute(context.Background(), flowmove.Input{Environment: "dev", Site: "site", FlowSelector: identity.Selector{LUID: "f-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || m.calls != 1 || r.flowCalls != 3 || r.projectCalls != 3 {
		t.Fatalf("applied=%#v resolver=%#v", applied, r)
	}
}

func TestMoveRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := flowmove.New(&resolver{}, &mover{}).Execute(context.Background(), flowmove.Input{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestMoveCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := flowmove.Output{Result: &flowmove.Result{Status: "succeeded", FlowLUID: "f-1", TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(flowmove.CompactResult)
	if compact.Result == nil || compact.Result.FlowLUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

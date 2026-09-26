package flow_test

import (
	"bytes"
	"context"
	flowmove "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
)

type moveResolver struct {
	flow                    flowmove.Record
	project                 flowmove.Project
	flowCalls, projectCalls int
}

func TestMoveOutputGolden(t *testing.T) {
	output := flowmove.MoveOutput{Plan: flowmove.MovePlan{Mode: "execute", Operation: "flow.move", Environment: "dev", Site: "sandbox", Source: flowmove.MoveFlow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Old"}, Destination: flowmove.Project{LUID: "project-2", Name: "New", Path: "New"}}, Result: &flowmove.MoveResult{Status: "succeeded", FlowLUID: "flow-1", ProjectLUID: "project-2", TableauRequestID: "request-1"}, Help: []string{"tadx content flow inspect --id flow-1"}}
	moveAssertGolden(t, "compact.toon", output, false)
	moveAssertGolden(t, "full.toon", output, true)
}

func moveAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "move", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func (r *moveResolver) ResolveFlow(context.Context, identity.Selector) (flowmove.Record, error) {
	r.flowCalls++
	return r.flow, nil
}
func (r *moveResolver) ResolveProject(context.Context, identity.Selector) (flowmove.Project, error) {
	r.projectCalls++
	return r.project, nil
}

type moveMover struct{ calls int }

func (m *moveMover) MoveFlow(context.Context, string, string) (flowmove.MoveResult, error) {
	m.calls++
	return flowmove.MoveResult{Status: "succeeded", FlowLUID: "f-1", ProjectLUID: "p-2"}, nil
}
func TestMoveMovePreviewsThenRevalidatesOnApply(t *testing.T) {
	r := &moveResolver{flow: flowmove.Record{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Old"}, project: flowmove.Project{LUID: "p-2", Path: "New"}}
	m := &moveMover{}
	preview, err := flowmove.Move(context.Background(), r, m, flowmove.MoveInput{Environment: "dev", Site: "site", FlowSelector: identity.Selector{LUID: "f-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || m.calls != 0 {
		t.Fatalf("preview=%#v", preview)
	}
	result, err := flowmove.Move(context.Background(), r, m, flowmove.MoveInput{Environment: "dev", Site: "site", FlowSelector: identity.Selector{LUID: "f-1"}, ProjectSelector: identity.Selector{LUID: "p-2"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || m.calls != 1 || r.flowCalls != 3 || r.projectCalls != 3 {
		t.Fatalf("result=%#v resolver=%#v", result, r)
	}
}

func TestMoveMoveRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := flowmove.Move(context.Background(), &moveResolver{}, &moveMover{}, flowmove.MoveInput{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestMoveMoveCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := flowmove.MoveOutput{Result: &flowmove.MoveResult{Status: "succeeded", FlowLUID: "f-1", TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(flowmove.MoveCompactResult)
	if compact.Result == nil || compact.Result.FlowLUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

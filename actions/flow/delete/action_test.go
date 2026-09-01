package delete_test

import (
	"bytes"
	"context"
	flowdelete "github.com/ahillspace/tadx/actions/flow/delete"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
)

type resolver struct {
	flow  flowdelete.Flow
	calls int
}

func TestOutputGolden(t *testing.T) {
	output := flowdelete.Output{Plan: flowdelete.Plan{Mode: "preview", Operation: "flow.delete", Environment: "dev", Site: "sandbox", Target: flowdelete.Flow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops"}}, Applied: true, Result: &flowdelete.Result{Status: "succeeded", FlowLUID: "flow-1", TableauRequestID: "request-1"}, Help: []string{"tadx content flow list"}}
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

func (r *resolver) ResolveFlow(context.Context, identity.Selector) (flowdelete.Flow, error) {
	r.calls++
	return r.flow, nil
}

type deleter struct{ calls int }

func (d *deleter) DeleteFlow(context.Context, string) (flowdelete.Result, error) {
	d.calls++
	return flowdelete.Result{Status: "succeeded", FlowLUID: "f-1"}, nil
}
func TestDeletePreviewsThenRevalidatesOnApply(t *testing.T) {
	r := &resolver{flow: flowdelete.Flow{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Ops"}}
	d := &deleter{}
	a := flowdelete.New(r, d)
	preview, err := a.Execute(context.Background(), flowdelete.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Applied || d.calls != 0 {
		t.Fatalf("preview=%#v", preview)
	}
	applied, err := a.Execute(context.Background(), flowdelete.Input{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !applied.Applied || d.calls != 1 || r.calls != 3 {
		t.Fatalf("applied=%#v calls=%d", applied, r.calls)
	}
}

func TestDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := flowdelete.New(&resolver{}, &deleter{}).Execute(context.Background(), flowdelete.Input{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestDeleteCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := flowdelete.Output{Result: &flowdelete.Result{Status: "succeeded", FlowLUID: "f-1", TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(flowdelete.CompactResult)
	if compact.Result == nil || compact.Result.FlowLUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

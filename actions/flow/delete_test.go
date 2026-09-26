package flow_test

import (
	"bytes"
	"context"
	flowdelete "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
	"os"
	"path/filepath"
	"testing"
)

type deleteResolver struct {
	flow  flowdelete.Record
	calls int
}

func TestDeleteOutputGolden(t *testing.T) {
	output := flowdelete.DeleteOutput{Plan: flowdelete.DeletePlan{Mode: "execute", Operation: "flow.delete", Environment: "dev", Site: "sandbox", Target: flowdelete.DeleteFlow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops"}}, Result: &flowdelete.DeleteResult{Status: "succeeded", FlowLUID: "flow-1", TableauRequestID: "request-1"}, Help: []string{"tadx content flow list"}}
	deleteAssertGolden(t, "compact.toon", output, false)
	deleteAssertGolden(t, "full.toon", output, true)
}

func deleteAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "delete", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func (r *deleteResolver) ResolveFlow(context.Context, identity.Selector) (flowdelete.Record, error) {
	r.calls++
	return r.flow, nil
}

type deleteDeleter struct{ calls int }

func (d *deleteDeleter) DeleteFlow(context.Context, string) (flowdelete.DeleteResult, error) {
	d.calls++
	return flowdelete.DeleteResult{Status: "succeeded", FlowLUID: "f-1"}, nil
}
func TestDeleteDeletePreviewsThenRevalidatesOnApply(t *testing.T) {
	r := &deleteResolver{flow: flowdelete.Record{LUID: "f-1", Name: "Daily", ProjectLUID: "p-1", ProjectPath: "Ops"}}
	d := &deleteDeleter{}
	preview, err := flowdelete.Delete(context.Background(), r, d, flowdelete.DeleteInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || d.calls != 0 {
		t.Fatalf("preview=%#v", preview)
	}
	result, err := flowdelete.Delete(context.Background(), r, d, flowdelete.DeleteInput{Environment: "dev", Site: "site", Selector: identity.Selector{LUID: "f-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Result == nil || d.calls != 1 || r.calls != 3 {
		t.Fatalf("result=%#v calls=%d", result, r.calls)
	}
}

func TestDeleteDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := flowdelete.Delete(context.Background(), &deleteResolver{}, &deleteDeleter{}, flowdelete.DeleteInput{}, false)
	if err == nil {
		t.Fatal("expected explicit target error")
	}
}

func TestDeleteDeleteCompactOutputOmitsSuccessfulRequestID(t *testing.T) {
	output := flowdelete.DeleteOutput{Result: &flowdelete.DeleteResult{Status: "succeeded", FlowLUID: "f-1", TableauRequestID: "request-secret"}}
	compact := output.CompactOutput().(flowdelete.DeleteCompactResult)
	if compact.Result == nil || compact.Result.FlowLUID != "f-1" || compact.Details != "--full" {
		t.Fatalf("compact = %#v", compact)
	}
}

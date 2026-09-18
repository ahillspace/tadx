package inspect_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	flowget "github.com/ahillspace/tadx/actions/flow/inspect"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type resolver struct{ flow flowget.Flow }

func (r resolver) ResolveFlow(context.Context, identity.Selector) (flowget.Flow, error) {
	return r.flow, nil
}

func TestOutputGolden(t *testing.T) {
	required := true
	output := flowget.Output{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Flow: flowget.Flow{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops", FileType: "tflx", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Daily prep", OwnerLUID: "user-1", Tags: []string{"daily"}, Parameters: []flowget.Parameter{{Name: "Region", Type: "string", Required: &required}}, OutputSteps: []flowget.OutputStep{{LUID: "step-1", Name: "Publish"}}},
		Help: []string{"tadx content flow pull --id flow-1"},
	}
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

func TestActionGetsExactFlowWithBoundedDetails(t *testing.T) {
	parameters := make([]flowget.Parameter, 60)
	output, err := flowget.New(resolver{flow: flowget.Flow{LUID: "f-1", Name: "Daily", ProjectPath: "Department/Ops", FileType: "tflx", Parameters: parameters}}).Execute(context.Background(), flowget.Input{Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.CompactOutput().(flowget.CompactResult).Flow.ProjectPath != "Department/Ops" {
		t.Fatalf("compact = %#v", output.CompactOutput())
	}
	full := output.FullOutput().(flowget.FullResult)
	if len(full.Flow.Parameters) != 50 || full.Flow.ParametersOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}

func TestValidateInputAcceptsProjectLUIDSelector(t *testing.T) {
	input := flowget.Input{}
	input.SetSelectorWithProjectLUID("", "Daily", "", "project-1")
	if err := flowget.ValidateInput(input); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
}

func TestValidateInputRejectsConflictingProjectSelectors(t *testing.T) {
	input := flowget.Input{}
	input.SetSelectorWithProjectLUID("", "Daily", "Department/Ops", "project-1")
	if err := flowget.ValidateInput(input); err == nil {
		t.Fatal("ValidateInput() error = nil, want project selector conflict")
	}
}

func TestActionRejectsMismatchedProjectLUID(t *testing.T) {
	flow := flowget.Flow{LUID: "f-1", Name: "Daily", ProjectLUID: "project-other"}
	input := flowget.Input{}
	input.SetSelectorWithProjectLUID("", "Daily", "", "project-1")
	if _, err := flowget.New(resolver{flow: flow}).Execute(t.Context(), input); err == nil {
		t.Fatal("Execute() error = nil, want identity mismatch")
	}
}

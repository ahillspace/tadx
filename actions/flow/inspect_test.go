package flow_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	flowget "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/identity"
	render "github.com/ahillspace/tadx/internal/output"
)

type inspectResolver struct{ flow flowget.Record }

func (r inspectResolver) ResolveFlow(context.Context, identity.Selector) (flowget.Record, error) {
	return r.flow, nil
}

func TestInspectOutputGolden(t *testing.T) {
	required := true
	output := flowget.InspectOutput{
		Status: "found", Environment: "dev", Site: "sandbox", RequestID: "request-1",
		Flow: flowget.Record{LUID: "flow-1", Name: "Daily", ProjectLUID: "project-1", ProjectPath: "Ops", FileType: "tflx", UpdatedAt: "2026-09-01T00:00:00Z", Description: "Daily prep", OwnerLUID: "user-1", Tags: []string{"daily"}, Parameters: []flowget.InspectParameter{{Name: "Region", Type: "string", Required: &required}}, OutputSteps: []flowget.InspectOutputStep{{LUID: "step-1", Name: "Publish"}}},
		Help: []string{"tadx content flow pull --id flow-1"},
	}
	inspectAssertGolden(t, "compact.toon", output, false)
	inspectAssertGolden(t, "full.toon", output, true)
}

func inspectAssertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "inspect", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

func TestInspectActionGetsExactFlowWithBoundedDetails(t *testing.T) {
	parameters := make([]flowget.InspectParameter, 60)
	output, err := flowget.Inspect(context.Background(), inspectResolver{flow: flowget.Record{LUID: "f-1", Name: "Daily", ProjectPath: "Department/Ops", FileType: "tflx", Parameters: parameters}}, flowget.InspectInput{Selector: identity.Selector{LUID: "f-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if output.CompactOutput().(flowget.InspectCompactResult).Flow.ProjectPath != "Department/Ops" {
		t.Fatalf("compact = %#v", output.CompactOutput())
	}
	full := output.FullOutput().(flowget.InspectFullResult)
	if len(full.Flow.Parameters) != 50 || full.Flow.ParametersOmitted != 10 {
		t.Fatalf("full = %#v", full)
	}
}

func TestValidateInspectInputAcceptsProjectLUIDSelector(t *testing.T) {
	input := flowget.InspectInput{}
	input.SetSelectorWithProjectLUID("", "Daily", "", "project-1")
	if err := flowget.ValidateInspectInput(input); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
}

func TestValidateInspectInputRejectsConflictingProjectSelectors(t *testing.T) {
	input := flowget.InspectInput{}
	input.SetSelectorWithProjectLUID("", "Daily", "Department/Ops", "project-1")
	if err := flowget.ValidateInspectInput(input); err == nil {
		t.Fatal("ValidateInput() error = nil, want project selector conflict")
	}
}

func TestInspectActionRejectsMismatchedProjectLUID(t *testing.T) {
	flow := flowget.Record{LUID: "f-1", Name: "Daily", ProjectLUID: "project-other"}
	input := flowget.InspectInput{}
	input.SetSelectorWithProjectLUID("", "Daily", "", "project-1")
	if _, err := flowget.Inspect(t.Context(), inspectResolver{flow: flow}, input); err == nil {
		t.Fatal("Execute() error = nil, want identity mismatch")
	}
}

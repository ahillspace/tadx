package job

import (
	"context"
	"testing"

	jobactions "github.com/ahillspace/tadx/actions/job"
)

type inspectFake struct {
	input jobactions.InspectInput
}

func (f *inspectFake) Execute(_ context.Context, input jobactions.InspectInput) (jobactions.InspectOutput, error) {
	f.input = input
	return jobactions.InspectOutput{Status: "running"}, nil
}

type renderFake struct{ value any }

func (f *renderFake) Render(value any) error {
	f.value = value
	return nil
}

func TestInspectCommandAcceptsOperationID(t *testing.T) {
	inspector := &inspectFake{}
	renderer := &renderFake{}
	command := New(Dependencies{Inspect: inspector.Execute, Renderer: renderer})
	command.SetArgs([]string{"inspect", "--operation-id", "run-1"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if inspector.input.OperationID != "run-1" || inspector.input.ID != "" {
		t.Fatalf("inspect input = %#v", inspector.input)
	}
}

func TestInspectCommandDoesNotExposeNoWait(t *testing.T) {
	command := New(Dependencies{Inspect: (&inspectFake{}).Execute, Renderer: &renderFake{}})
	command.SetArgs([]string{"inspect", "--operation-id", "run-1", "--no-wait"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unknown flag")
	}
}

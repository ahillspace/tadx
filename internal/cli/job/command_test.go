package job

import (
	"context"
	"testing"

	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
)

type inspectFake struct {
	input jobinspect.Input
}

func (f *inspectFake) Execute(_ context.Context, input jobinspect.Input) (jobinspect.Output, error) {
	f.input = input
	return jobinspect.Output{Status: "running"}, nil
}

type renderFake struct{ value any }

func (f *renderFake) Render(value any) error {
	f.value = value
	return nil
}

func TestInspectCommandAcceptsOperationID(t *testing.T) {
	inspector := &inspectFake{}
	renderer := &renderFake{}
	command := New(Dependencies{Inspector: inspector, Renderer: renderer})
	command.SetArgs([]string{"inspect", "--operation-id", "run-1"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if inspector.input.OperationID != "run-1" || inspector.input.ID != "" {
		t.Fatalf("inspect input = %#v", inspector.input)
	}
}

func TestInspectCommandRejectsJobAndOperationIDTogether(t *testing.T) {
	inspector := &inspectFake{}
	command := New(Dependencies{Inspector: inspector, Renderer: &renderFake{}})
	command.SetArgs([]string{"inspect", "--id", "job-1", "--operation-id", "run-1"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want selector usage error")
	}
}

func TestInspectCommandDoesNotExposeNoWait(t *testing.T) {
	command := New(Dependencies{Inspector: &inspectFake{}, Renderer: &renderFake{}})
	command.SetArgs([]string{"inspect", "--operation-id", "run-1", "--no-wait"})
	if err := command.Execute(); err == nil {
		t.Fatal("Execute() error = nil, want unknown flag")
	}
}

var _ Inspector = (*inspectFake)(nil)

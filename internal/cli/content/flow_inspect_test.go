package content

import (
	"context"
	"testing"

	flowinspect "github.com/ahillspace/tadx/actions/flow/inspect"
)

type flowInspectCommands struct {
	input flowinspect.Input
}

func (c *flowInspectCommands) InspectFlow(_ context.Context, input flowinspect.Input) (flowinspect.Output, error) {
	c.input = input
	return flowinspect.Output{Status: "found"}, nil
}

type flowInspectRenderer struct{}

func (flowInspectRenderer) Render(any) error { return nil }

func TestFlowInspectAcceptsAuthoritativeProjectLUIDSelector(t *testing.T) {
	actions := &flowInspectCommands{}
	command := newFlowInspect(Dependencies{FlowInspector: actions, Renderer: flowInspectRenderer{}})
	command.SetArgs([]string{"--name", "Daily", "--project-id", "project-1"})
	if err := command.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	selector := actions.input.Selector
	if selector.Name != "Daily" || string(selector.ProjectLUID) != "project-1" || selector.ProjectPath != "" {
		t.Fatalf("selector = %#v", selector)
	}
}

func TestFlowInspectRejectsConflictingProjectSelectors(t *testing.T) {
	for _, args := range [][]string{
		{"--name", "Daily"},
		{"--name", "Daily", "--project", "Department/Ops", "--project-id", "project-1"},
		{"--id", "flow-1", "--project-id", "project-1"},
	} {
		command := newFlowInspect(Dependencies{FlowInspector: &flowInspectCommands{}, Renderer: flowInspectRenderer{}})
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%v) error = nil, want selector error", args)
		}
	}
}

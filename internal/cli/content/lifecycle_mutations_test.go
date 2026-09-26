package content

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"testing"

	"github.com/spf13/cobra"
)

type lifecycleMutationCommands struct {
	workbookMoveInput       workbookops.MoveInput
	workbookMovePreview     bool
	workbookUpdateInput     workbookops.UpdateInput
	workbookUpdatePreview   bool
	datasourceMoveInput     datasourceops.MoveInput
	datasourceMovePreview   bool
	datasourceUpdateInput   datasourceops.UpdateInput
	datasourceUpdatePreview bool
	flowUpdateInput         flowops.UpdateInput
	flowUpdatePreview       bool
	projectMoveInput        projectmove.Input
	projectMovePreview      bool
}

func (c *lifecycleMutationCommands) MoveWorkbook(_ context.Context, input workbookops.MoveInput, preview bool) (workbookops.MoveOutput, error) {
	c.workbookMoveInput, c.workbookMovePreview = input, preview
	return workbookops.MoveOutput{}, nil
}

func (c *lifecycleMutationCommands) UpdateWorkbook(_ context.Context, input workbookops.UpdateInput, preview bool) (workbookops.UpdateOutput, error) {
	c.workbookUpdateInput, c.workbookUpdatePreview = input, preview
	return workbookops.UpdateOutput{}, nil
}

func (c *lifecycleMutationCommands) MoveDatasource(_ context.Context, input datasourceops.MoveInput, preview bool) (datasourceops.MoveOutput, error) {
	c.datasourceMoveInput, c.datasourceMovePreview = input, preview
	return datasourceops.MoveOutput{}, nil
}

func (c *lifecycleMutationCommands) UpdateDatasource(_ context.Context, input datasourceops.UpdateInput, preview bool) (datasourceops.UpdateOutput, error) {
	c.datasourceUpdateInput, c.datasourceUpdatePreview = input, preview
	return datasourceops.UpdateOutput{}, nil
}

func (c *lifecycleMutationCommands) UpdateFlow(_ context.Context, input flowops.UpdateInput, preview bool) (flowops.UpdateOutput, error) {
	c.flowUpdateInput, c.flowUpdatePreview = input, preview
	return flowops.UpdateOutput{}, nil
}

func (c *lifecycleMutationCommands) MoveProject(_ context.Context, input projectmove.Input, preview bool) (projectmove.Output, error) {
	c.projectMoveInput, c.projectMovePreview = input, preview
	return projectmove.Output{}, nil
}

func lifecycleMutationDependencies(actions *lifecycleMutationCommands) Dependencies {
	return Dependencies{
		WorkbookMover: actions, WorkbookUpdater: actions,
		DatasourceMover: actions, DatasourceUpdater: actions,
		FlowUpdater: actions, ProjectMover: actions,
		Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true,
	}
}

func TestLifecycleMutationCommandsProjectFlagsAndPreview(t *testing.T) {
	actions := &lifecycleMutationCommands{}
	deps := lifecycleMutationDependencies(actions)
	tests := []struct {
		name    string
		command *cobra.Command
		args    []string
		assert  func(*testing.T)
	}{
		{name: "workbook move", command: newWorkbookMove(deps), args: []string{"--environment", "dev", "--name", "Sales", "--project", "Old", "--destination-project-id", "project-2", "--preview"}, assert: func(t *testing.T) {
			if actions.workbookMoveInput.Environment != "dev" || actions.workbookMoveInput.WorkbookSelector.Name != "Sales" || actions.workbookMoveInput.WorkbookSelector.ProjectPath != "Old" || actions.workbookMoveInput.ProjectSelector.LUID != "project-2" || !actions.workbookMovePreview {
				t.Fatalf("input=%#v preview=%t", actions.workbookMoveInput, actions.workbookMovePreview)
			}
		}},
		{name: "workbook update", command: newWorkbookUpdate(deps), args: []string{"--environment", "dev", "--id", "workbook-1", "--new-name", "Renamed", "--owner-id", "owner-2", "--preview"}, assert: func(t *testing.T) {
			if actions.workbookUpdateInput.Environment != "dev" || actions.workbookUpdateInput.Selector.LUID != "workbook-1" || actions.workbookUpdateInput.Name == nil || *actions.workbookUpdateInput.Name != "Renamed" || actions.workbookUpdateInput.OwnerLUID == nil || *actions.workbookUpdateInput.OwnerLUID != "owner-2" || !actions.workbookUpdatePreview {
				t.Fatalf("input=%#v preview=%t", actions.workbookUpdateInput, actions.workbookUpdatePreview)
			}
		}},
		{name: "datasource move", command: newDatasourceMove(deps), args: []string{"--environment", "dev", "--id", "datasource-1", "--destination-project", "New", "--preview"}, assert: func(t *testing.T) {
			if actions.datasourceMoveInput.Environment != "dev" || actions.datasourceMoveInput.DatasourceSelector.LUID != "datasource-1" || actions.datasourceMoveInput.ProjectSelector.ProjectPath != "New" || !actions.datasourceMovePreview {
				t.Fatalf("input=%#v preview=%t", actions.datasourceMoveInput, actions.datasourceMovePreview)
			}
		}},
		{name: "datasource update", command: newDatasourceUpdate(deps), args: []string{"--environment", "dev", "--name", "Sales", "--project", "Old", "--owner-id", "owner-2", "--preview"}, assert: func(t *testing.T) {
			if actions.datasourceUpdateInput.Environment != "dev" || actions.datasourceUpdateInput.Selector.Name != "Sales" || actions.datasourceUpdateInput.Selector.ProjectPath != "Old" || actions.datasourceUpdateInput.OwnerLUID == nil || *actions.datasourceUpdateInput.OwnerLUID != "owner-2" || !actions.datasourceUpdatePreview {
				t.Fatalf("input=%#v preview=%t", actions.datasourceUpdateInput, actions.datasourceUpdatePreview)
			}
		}},
		{name: "flow update", command: newFlowUpdate(deps), args: []string{"--environment", "dev", "--id", "flow-1", "--owner-id", "owner-2", "--preview"}, assert: func(t *testing.T) {
			if actions.flowUpdateInput.Environment != "dev" || actions.flowUpdateInput.Selector.LUID != "flow-1" || actions.flowUpdateInput.OwnerLUID == nil || *actions.flowUpdateInput.OwnerLUID != "owner-2" || !actions.flowUpdatePreview {
				t.Fatalf("input=%#v preview=%t", actions.flowUpdateInput, actions.flowUpdatePreview)
			}
		}},
		{name: "project move", command: newProjectMove(deps), args: []string{"--environment", "dev", "--project", "Old/Child", "--parent-id", "parent-2", "--preview"}, assert: func(t *testing.T) {
			if actions.projectMoveInput.Environment != "dev" || actions.projectMoveInput.ProjectSelector.ProjectPath != "Old/Child" || actions.projectMoveInput.ParentSelector.LUID != "parent-2" || !actions.projectMovePreview {
				t.Fatalf("input=%#v preview=%t", actions.projectMoveInput, actions.projectMovePreview)
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.command.SetArgs(test.args)
			if err := test.command.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			test.assert(t)
		})
	}
}

func TestLifecycleMutationCommandsExposeExactCapabilityAnnotations(t *testing.T) {
	deps := lifecycleMutationDependencies(&lifecycleMutationCommands{})
	tests := []struct {
		command    *cobra.Command
		capability string
	}{
		{newWorkbookMove(deps), "workbook.move"}, {newWorkbookUpdate(deps), "workbook.update"},
		{newDatasourceMove(deps), "datasource.move"}, {newDatasourceUpdate(deps), "datasource.update"},
		{newFlowUpdate(deps), "flow.update"}, {newProjectMove(deps), "project.move"},
	}
	for _, test := range tests {
		if got := test.command.Annotations["tadx.capability"]; got != test.capability {
			t.Fatalf("%s annotation = %q", test.capability, got)
		}
	}
}

func TestProjectMoveCommandProjectsTopLevelFlag(t *testing.T) {
	actions := &lifecycleMutationCommands{}
	command := newProjectMove(lifecycleMutationDependencies(actions))
	command.SetArgs([]string{"--environment", "dev", "--project-id", "project-1", "--top-level", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !actions.projectMoveInput.TopLevel || actions.projectMoveInput.ProjectSelector.LUID != "project-1" || actions.projectMoveInput.ParentSelector.LUID != "" || actions.projectMoveInput.ParentSelector.ProjectPath != "" || !actions.projectMovePreview {
		t.Fatalf("input=%#v preview=%t", actions.projectMoveInput, actions.projectMovePreview)
	}
}

func TestLifecycleMutationCommandsRequireEnvironmentAndExactSelectors(t *testing.T) {
	deps := lifecycleMutationDependencies(&lifecycleMutationCommands{})
	tests := []struct {
		name    string
		command *cobra.Command
		args    []string
	}{
		{"workbook move environment", newWorkbookMove(deps), []string{"--id", "wb-1", "--destination-project-id", "p-2"}},
		{"workbook move selector", newWorkbookMove(deps), []string{"--environment", "dev", "--destination-project-id", "p-2"}},
		{"workbook update environment", newWorkbookUpdate(deps), []string{"--id", "wb-1", "--new-name", "New"}},
		{"workbook update selector", newWorkbookUpdate(deps), []string{"--environment", "dev", "--new-name", "New"}},
		{"datasource move environment", newDatasourceMove(deps), []string{"--id", "ds-1", "--destination-project-id", "p-2"}},
		{"datasource move selector", newDatasourceMove(deps), []string{"--environment", "dev", "--destination-project-id", "p-2"}},
		{"datasource update environment", newDatasourceUpdate(deps), []string{"--id", "ds-1", "--owner-id", "u-2"}},
		{"datasource update selector", newDatasourceUpdate(deps), []string{"--environment", "dev", "--owner-id", "u-2"}},
		{"flow update environment", newFlowUpdate(deps), []string{"--id", "flow-1", "--owner-id", "u-2"}},
		{"flow update selector", newFlowUpdate(deps), []string{"--environment", "dev", "--owner-id", "u-2"}},
		{"project move environment", newProjectMove(deps), []string{"--project-id", "p-1", "--top-level"}},
		{"project move selector", newProjectMove(deps), []string{"--environment", "dev", "--top-level"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.command.SetArgs(test.args)
			if err := test.command.ExecuteContext(context.Background()); err == nil {
				t.Fatal("expected usage error")
			}
		})
	}
}

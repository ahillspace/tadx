package content

import (
	"context"
	"testing"

	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	projectupdate "github.com/ahillspace/tadx/actions/project/update"
)

type projectMutationCommands struct {
	createInput projectcreate.Input
	createApply bool
	updateInput projectupdate.Input
	updateApply bool
}

func (c *projectMutationCommands) CreateProject(_ context.Context, input projectcreate.Input, apply bool) (projectcreate.Output, error) {
	c.createInput, c.createApply = input, apply
	return projectcreate.Output{}, nil
}

func (c *projectMutationCommands) UpdateProject(_ context.Context, input projectupdate.Input, apply bool) (projectupdate.Output, error) {
	c.updateInput, c.updateApply = input, apply
	return projectupdate.Output{}, nil
}

func TestProjectCreateParsesExplicitParentAndMutation(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectCreate(Dependencies{ProjectCreator: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--name", "Operations", "--description", "Direct operations", "--content-permissions", "LockedToProject", "--parent", "Department", "--apply"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.createInput.Environment != "dev" || actions.createInput.Name != "Operations" || actions.createInput.ParentSelector.ProjectPath != "Department" || actions.createInput.ContentPermissions != "LockedToProject" || !actions.createApply {
		t.Fatalf("input=%#v apply=%t", actions.createInput, actions.createApply)
	}
}

func TestProjectUpdatePreservesExplicitEmptyDescription(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectUpdate(Dependencies{ProjectUpdater: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--id", "project-1", "--description", "", "--apply"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.updateInput.Selector.LUID != "project-1" || actions.updateInput.Description == nil || *actions.updateInput.Description != "" || !actions.updateApply {
		t.Fatalf("input=%#v apply=%t", actions.updateInput, actions.updateApply)
	}
}

func TestProjectMutationsRequireExplicitEnvironment(t *testing.T) {
	create := newProjectCreate(Dependencies{ProjectCreator: &projectMutationCommands{}, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	create.SetArgs([]string{"--name", "Operations"})
	if err := create.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project create environment error")
	}
	update := newProjectUpdate(Dependencies{ProjectUpdater: &projectMutationCommands{}, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	update.SetArgs([]string{"--id", "project-1", "--name", "Operations"})
	if err := update.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project update environment error")
	}
}

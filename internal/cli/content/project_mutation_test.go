package content

import (
	"bytes"
	"context"
	"strings"
	"testing"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/spf13/cobra"
)

type projectMutationCommands struct {
	inspectCalls  int
	inspectInput  projectops.InspectInput
	createInput   projectops.CreateInput
	createPreview bool
	updateInput   projectops.UpdateInput
	updatePreview bool
	deleteInput   projectops.DeleteInput
	deletePreview bool
}

func (c *projectMutationCommands) InspectProject(_ context.Context, input projectops.InspectInput) (projectops.InspectOutput, error) {
	c.inspectCalls++
	c.inspectInput = input
	return projectops.InspectOutput{Status: "found", Project: projectops.InspectProject{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}}, nil
}

func (c *projectMutationCommands) CreateProject(_ context.Context, input projectops.CreateInput, preview bool) (projectops.CreateOutput, error) {
	c.createInput, c.createPreview = input, preview
	return projectops.CreateOutput{}, nil
}

func (c *projectMutationCommands) UpdateProject(_ context.Context, input projectops.UpdateInput, preview bool) (projectops.UpdateOutput, error) {
	c.updateInput, c.updatePreview = input, preview
	return projectops.UpdateOutput{}, nil
}

func (c *projectMutationCommands) DeleteProject(_ context.Context, input projectops.DeleteInput, preview bool) (projectops.DeleteOutput, error) {
	c.deleteInput, c.deletePreview = input, preview
	return projectops.DeleteOutput{}, nil
}

func TestProjectCreateParsesExplicitParentAndMutation(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectCreate(Dependencies{ProjectCreator: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--name", "Operations", "--description", "Direct operations", "--content-permissions", "LockedToProject", "--parent", "Department"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.createInput.Environment != "dev" || actions.createInput.Name != "Operations" || actions.createInput.ParentSelector.ProjectPath != "Department" || actions.createInput.ContentPermissions != "LockedToProject" || actions.createPreview {
		t.Fatalf("input=%#v preview=%t", actions.createInput, actions.createPreview)
	}
}

func TestProjectUpdatePreservesExplicitEmptyDescription(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectUpdate(Dependencies{ProjectUpdater: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--project-id", "project-1", "--description", "", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.updateInput.Selector.LUID != "project-1" || actions.updateInput.Description == nil || *actions.updateInput.Description != "" || !actions.updatePreview {
		t.Fatalf("input=%#v preview=%t", actions.updateInput, actions.updatePreview)
	}
}

func TestProjectDeleteRequiresExactProjectID(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectDelete(Dependencies{ProjectDeleter: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--project-id", "project-1", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.deleteInput.Environment != "dev" || actions.deleteInput.ProjectLUID != "project-1" || !actions.deletePreview {
		t.Fatalf("input=%#v preview=%t", actions.deleteInput, actions.deletePreview)
	}

	command = newProjectDelete(Dependencies{ProjectDeleter: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"--environment", "dev", "--project", "Department/Operations"})
	if err := command.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project delete selector error")
	}
}

func TestProjectInspectAcceptsCanonicalProjectID(t *testing.T) {
	actions := &projectMutationCommands{}
	command := newProjectInspect(Dependencies{ProjectInspector: actions, Renderer: &workbookDeleteRenderer{}})
	command.SetArgs([]string{"--project-id", "project-1"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.inspectInput.Selector.LUID != "project-1" {
		t.Fatalf("input=%#v", actions.inspectInput)
	}
}

func TestProjectCommandsAcceptPrimaryIDFlag(t *testing.T) {
	tests := []struct {
		name    string
		command func(*projectMutationCommands) *cobra.Command
		args    []string
	}{
		{
			name: "inspect",
			command: func(actions *projectMutationCommands) *cobra.Command {
				return newProjectInspect(Dependencies{ProjectInspector: actions, Renderer: &workbookDeleteRenderer{}})
			},
			args: []string{"--id", "project-1"},
		},
		{
			name: "update",
			command: func(actions *projectMutationCommands) *cobra.Command {
				return newProjectUpdate(Dependencies{ProjectUpdater: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
			},
			args: []string{"--environment", "dev", "--id", "project-1", "--name", "Operations"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := &projectMutationCommands{}
			command := test.command(actions)
			command.SetArgs(test.args)
			err := command.ExecuteContext(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if test.name == "inspect" && actions.inspectCalls != 1 {
				t.Fatalf("inspect calls = %d", actions.inspectCalls)
			}
			if test.name == "update" && actions.updateInput.Selector.LUID != "project-1" {
				t.Fatalf("input = %#v", actions.updateInput)
			}
		})
	}
}

func TestProjectHelpShowsCanonicalProjectSelectors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := newProjectInspect(Dependencies{})
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{"--help"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "--project-id string") || !strings.Contains(stdout.String(), "--id string") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestProjectMutationsRequireExplicitEnvironment(t *testing.T) {
	create := newProjectCreate(Dependencies{ProjectCreator: &projectMutationCommands{}, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	create.SetArgs([]string{"--name", "Operations"})
	if err := create.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project create environment error")
	}
	update := newProjectUpdate(Dependencies{ProjectUpdater: &projectMutationCommands{}, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	update.SetArgs([]string{"--project-id", "project-1", "--name", "Operations"})
	if err := update.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project update environment error")
	}
	deleteCommand := newProjectDelete(Dependencies{ProjectDeleter: &projectMutationCommands{}, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	deleteCommand.SetArgs([]string{"--project-id", "project-1"})
	if err := deleteCommand.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected project delete environment error")
	}
}

package content

import (
	"bytes"
	"context"
	"strings"
	"testing"

	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	projectinspect "github.com/ahillspace/tadx/actions/project/inspect"
	projectupdate "github.com/ahillspace/tadx/actions/project/update"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/spf13/cobra"
)

type projectMutationCommands struct {
	inspectCalls  int
	inspectInput  projectinspect.Input
	createInput   projectcreate.Input
	createPreview bool
	updateInput   projectupdate.Input
	updatePreview bool
}

func (c *projectMutationCommands) InspectProject(_ context.Context, input projectinspect.Input) (projectinspect.Output, error) {
	c.inspectCalls++
	c.inspectInput = input
	return projectinspect.Output{Status: "found", Project: projectinspect.Project{LUID: "project-1", Name: "Operations", Path: "Department/Operations"}}, nil
}

func (c *projectMutationCommands) CreateProject(_ context.Context, input projectcreate.Input, preview bool) (projectcreate.Output, error) {
	c.createInput, c.createPreview = input, preview
	return projectcreate.Output{}, nil
}

func (c *projectMutationCommands) UpdateProject(_ context.Context, input projectupdate.Input, preview bool) (projectupdate.Output, error) {
	c.updateInput, c.updatePreview = input, preview
	return projectupdate.Output{}, nil
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

func TestProjectIDAliasProducesCleanTOONWithoutWarnings(t *testing.T) {
	actions := &projectMutationCommands{}
	var stdout, stderr bytes.Buffer
	getCommand := newProjectInspect(Dependencies{ProjectInspector: actions, Renderer: projectTOONRenderer{writer: &stdout}})
	getCommand.SetOut(&stdout)
	getCommand.SetErr(&stderr)
	getCommand.SetArgs([]string{"--id", "project-1"})
	if err := getCommand.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.inspectInput.Selector.LUID != "project-1" {
		t.Fatalf("input=%#v", actions.inspectInput)
	}
	if flag := getCommand.Flags().Lookup("id"); flag == nil || !flag.Hidden || flag.Deprecated != "" {
		t.Fatalf("legacy --id flag must be silent and hidden: %#v", flag)
	}
	if flag := getCommand.Flags().Lookup("project-id"); flag == nil || flag.Hidden {
		t.Fatalf("canonical --project-id flag is not visible: %#v", flag)
	}
	if stderr.Len() != 0 || strings.Contains(stdout.String(), "deprecated") || !strings.Contains(stdout.String(), "status: found") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	updateCommand := newProjectUpdate(Dependencies{ProjectUpdater: actions, Renderer: &workbookDeleteRenderer{}, MutationsEnabled: true})
	updateCommand.SetArgs([]string{"--environment", "dev", "--id", "project-1", "--name", "Operations"})
	if err := updateCommand.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.updateInput.Selector.LUID != "project-1" {
		t.Fatalf("input=%#v", actions.updateInput)
	}
	if flag := updateCommand.Flags().Lookup("id"); flag == nil || !flag.Hidden || flag.Deprecated != "" {
		t.Fatalf("legacy --id flag must be silent and hidden: %#v", flag)
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
	if !strings.Contains(stdout.String(), "--project-id string") || strings.Contains(stdout.String(), "--id string") || stderr.Len() != 0 {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestProjectLUIDAliasesConflictDeterministically(t *testing.T) {
	tests := []struct {
		name    string
		command func(*projectMutationCommands) *cobra.Command
		args    []string
	}{
		{
			name: "get canonical then legacy",
			command: func(actions *projectMutationCommands) *cobra.Command {
				return newProjectInspect(Dependencies{ProjectInspector: actions, Renderer: &workbookDeleteRenderer{}})
			},
			args: []string{"--project-id", "canonical", "--id", "legacy"},
		},
		{
			name: "get legacy then canonical",
			command: func(actions *projectMutationCommands) *cobra.Command {
				return newProjectInspect(Dependencies{ProjectInspector: actions, Renderer: &workbookDeleteRenderer{}})
			},
			args: []string{"--id", "legacy", "--project-id", "canonical"},
		},
		{
			name: "update",
			command: func(actions *projectMutationCommands) *cobra.Command {
				return newProjectUpdate(Dependencies{ProjectUpdater: actions, Renderer: &workbookDeleteRenderer{}})
			},
			args: []string{"--environment", "dev", "--project-id", "canonical", "--id", "legacy", "--name", "Operations"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := &projectMutationCommands{}
			command := test.command(actions)
			command.SetArgs(test.args)
			err := command.ExecuteContext(context.Background())
			if err == nil || !strings.Contains(err.Error(), "use at most one of --project-id or --id") {
				t.Fatalf("error = %v", err)
			}
			if actions.inspectCalls != 0 || actions.updateInput.Selector.LUID != "" {
				t.Fatalf("action invoked: %#v", actions)
			}
		})
	}
}

type projectTOONRenderer struct{ writer *bytes.Buffer }

func (r projectTOONRenderer) Render(value any) error {
	return output.Render(r.writer, value)
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
}

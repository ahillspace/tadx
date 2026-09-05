package content

import (
	"context"
	"errors"

	projectcreate "github.com/ahillspace/tadx/actions/project/create"
	projectdelete "github.com/ahillspace/tadx/actions/project/delete"
	projectinspect "github.com/ahillspace/tadx/actions/project/inspect"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	projectupdate "github.com/ahillspace/tadx/actions/project/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type ProjectLister interface {
	ListProjects(context.Context, projectlist.Input) (projectlist.Output, error)
}

type ProjectInspector interface {
	InspectProject(context.Context, projectinspect.Input) (projectinspect.Output, error)
}

type ProjectCreator interface {
	CreateProject(context.Context, projectcreate.Input, bool) (projectcreate.Output, error)
}

type ProjectUpdater interface {
	UpdateProject(context.Context, projectupdate.Input, bool) (projectupdate.Output, error)
}

type ProjectDeleter interface {
	DeleteProject(context.Context, projectdelete.Input, bool) (projectdelete.Output, error)
}

func newProject(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "project", Short: "Inspect Tableau projects"}
	command.AddCommand(newProjectList(deps), newProjectInspect(deps))
	if deps.ProjectCreator != nil {
		command.AddCommand(newProjectCreate(deps))
	}
	if deps.ProjectUpdater != nil {
		command.AddCommand(newProjectUpdate(deps))
	}
	if deps.ProjectDeleter != nil {
		command.AddCommand(newProjectDelete(deps))
	}
	if deps.ProjectMover != nil {
		command.AddCommand(newProjectMove(deps))
	}
	return command
}

func newProjectCreate(deps Dependencies) *cobra.Command {
	var input projectcreate.Input
	var parentLUID, parentPath string
	var preview bool
	command := &cobra.Command{
		Use: "create", Short: "Create one project.",
		Annotations: map[string]string{"tadx.capability": "project.create"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("project.create")(command, args); err != nil {
				return err
			}
			if input.Environment == "" || input.Name == "" {
				return clierr.Usage("project.create", errors.New("--environment and --name are required"))
			}
			if parentLUID != "" && parentPath != "" {
				return clierr.Usage("project.create", errors.New("use at most one of --parent-id or --parent"))
			}
			input.SetParentSelector(parentLUID, parentPath)
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.ProjectCreator.CreateProject(command.Context(), input, preview)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.Name, "name", "", "exact new project name")
	command.Flags().StringVar(&input.Description, "description", "", "project description")
	command.Flags().StringVar(&input.ContentPermissions, "content-permissions", "", "explicit Tableau content permission mode")
	command.Flags().StringVar(&parentLUID, "parent-id", "", "authoritative parent project LUID")
	command.Flags().StringVar(&parentPath, "parent", "", "exact slash-delimited parent project path")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newProjectUpdate(deps Dependencies) *cobra.Command {
	var input projectupdate.Input
	var projectLUID, projectPath, name, description, contentPermissions string
	var preview bool
	command := &cobra.Command{
		Use: "update", Short: "Update one exact project.",
		Annotations: map[string]string{"tadx.capability": "project.update"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("project.update")(command, args); err != nil {
				return err
			}
			if input.Environment == "" || (projectLUID == "") == (projectPath == "") {
				return clierr.Usage("project.update", errors.New("--environment and exactly one of --project-id or --project are required"))
			}
			if !command.Flags().Changed("name") && !command.Flags().Changed("description") && !command.Flags().Changed("content-permissions") {
				return clierr.Usage("project.update", errors.New("at least one metadata change is required"))
			}
			input.SetSelector(projectLUID, projectPath)
			if command.Flags().Changed("name") {
				input.Name = &name
			}
			if command.Flags().Changed("description") {
				input.Description = &description
			}
			if command.Flags().Changed("content-permissions") {
				input.ContentPermissions = &contentPermissions
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.ProjectUpdater.UpdateProject(command.Context(), input, preview)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&name, "name", "", "replacement project name")
	command.Flags().StringVar(&description, "description", "", "replacement project description")
	command.Flags().StringVar(&contentPermissions, "content-permissions", "", "replacement Tableau content permission mode")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newProjectDelete(deps Dependencies) *cobra.Command {
	var input projectdelete.Input
	var preview bool
	command := &cobra.Command{
		Use: "delete", Short: "Delete one exact project.",
		Annotations: map[string]string{"tadx.capability": "project.delete"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("project.delete")(command, args); err != nil {
				return err
			}
			if input.Environment == "" || input.ProjectLUID == "" {
				return clierr.Usage("project.delete", errors.New("--environment and --project-id are required"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.ProjectDeleter.DeleteProject(command.Context(), input, preview)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&input.ProjectLUID, "project-id", "", "authoritative project LUID")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newProjectList(deps Dependencies) *cobra.Command {
	var input projectlist.Input
	var topLevel bool
	command := &cobra.Command{
		Use: "list", Short: "List one bounded project page.", Annotations: map[string]string{"tadx.capability": "project.list"}, Args: noContentArgs("project.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			if command.Flags().Changed("top-level") {
				input.TopLevel = &topLevel
			}
			result, err := deps.ProjectLister.ListProjects(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact project-name filter")
	command.Flags().StringVar(&input.ParentLUID, "parent-id", "", "authoritative direct parent project LUID filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().BoolVar(&topLevel, "top-level", false, "filter by top-level project status")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum projects to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}

func newProjectInspect(deps Dependencies) *cobra.Command {
	var input projectinspect.Input
	var projectLUID, projectPath string
	command := &cobra.Command{
		Use: "inspect", Short: "Inspect one exact project.", Annotations: map[string]string{"tadx.capability": "project.inspect"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("project.inspect")(command, args); err != nil {
				return err
			}
			if (projectLUID == "") == (projectPath == "") {
				return clierr.Usage("project.inspect", errors.New("use exactly one of --project-id or --project"))
			}
			input.SetSelector(projectLUID, projectPath)
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.ProjectInspector.InspectProject(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&projectLUID, "project-id", "", "authoritative project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}
func noContentArgs(operation string) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}

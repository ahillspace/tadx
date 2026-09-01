package content

import (
	"context"
	"errors"

	projectget "github.com/ahillspace/tadx/actions/project/get"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type ProjectLister interface {
	ListProjects(context.Context, projectlist.Input) (projectlist.Output, error)
}

type ProjectGetter interface {
	GetProject(context.Context, projectget.Input) (projectget.Output, error)
}

func newProject(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "project", Short: "Inspect Tableau projects"}
	command.AddCommand(newProjectList(deps), newProjectGet(deps))
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
	return command
}

func newProjectGet(deps Dependencies) *cobra.Command {
	var input projectget.Input
	var luid, projectPath string
	command := &cobra.Command{
		Use: "get", Short: "Inspect one exact project.", Annotations: map[string]string{"tadx.capability": "project.get"},
		Args: func(command *cobra.Command, args []string) error {
			if err := noContentArgs("project.get")(command, args); err != nil {
				return err
			}
			if (luid == "") == (projectPath == "") {
				return clierr.Usage("project.get", errors.New("use exactly one of --id or --project"))
			}
			input.SetSelector(luid, projectPath)
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.ProjectGetter.GetProject(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative project LUID")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
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

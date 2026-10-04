package content

import (
	"errors"

	projectops "github.com/ahillspace/tadx/actions/project"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newProjectMove(deps Dependencies) *cobra.Command {
	var input projectops.MoveInput
	var projectLUID, projectPath, parentLUID, parentPath string
	var preview bool
	command := mutationCommand("project.move", "move", "Move one exact project in the hierarchy.", func(command *cobra.Command) error {
		if (projectLUID == "") == (projectPath == "") {
			return clierr.Usage("project.move", errors.New("use exactly one of --id or --project"))
		}
		if err := requireWriteEnvironment("project.move", input.Environment); err != nil {
			return err
		}
		parentSelected := parentLUID != "" || parentPath != ""
		if input.TopLevel == parentSelected || (parentLUID != "" && parentPath != "") {
			return clierr.Usage("project.move", errors.New("use --top-level or exactly one of --parent-id or --parent"))
		}
		input.SetProjectSelector(projectLUID, projectPath)
		if parentSelected {
			input.SetParentSelector(parentLUID, parentPath)
		}
		result, err := deps.ProjectMover.MoveProject(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&projectLUID, "id", "", "authoritative project LUID")
	command.Flags().StringVar(&projectLUID, "project-id", "", "legacy alias for --id")
	command.MarkFlagsMutuallyExclusive("id", "project-id")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&parentLUID, "parent-id", "", "authoritative destination parent project LUID")
	command.Flags().StringVar(&parentPath, "parent", "", "exact destination parent project path")
	command.Flags().BoolVar(&input.TopLevel, "top-level", false, "move the project to the top level")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

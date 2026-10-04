package content

import (
	"errors"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newWorkbookMove(deps Dependencies) *cobra.Command {
	var input workbookops.MoveInput
	var luid, name, sourceProject, destinationLUID, destinationPath string
	var preview bool
	command := mutationCommand("workbook.move", "move", "Move one exact workbook.", func(command *cobra.Command) error {
		if err := selectorArgs("workbook.move", &luid, &name, &sourceProject, input.SetWorkbookSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("workbook.move", input.Environment); err != nil {
			return err
		}
		if (destinationLUID == "") == (destinationPath == "") {
			return clierr.Usage("workbook.move", errors.New("use exactly one of --destination-project-id or --destination-project"))
		}
		input.SetProjectSelector(destinationLUID, destinationPath)
		result, err := deps.WorkbookMover.MoveWorkbook(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	contentTargetFlags(command, &input.Environment, &luid, &name, &sourceProject, "workbook")
	destinationProjectFlags(command, &destinationLUID, &destinationPath)
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newWorkbookUpdate(deps Dependencies) *cobra.Command {
	var input workbookops.UpdateInput
	var luid, name, projectPath, newName, ownerLUID, description string
	var preview bool
	command := mutationCommand("workbook.update", "update", "Update one exact workbook.", func(command *cobra.Command) error {
		if err := selectorArgs("workbook.update", &luid, &name, &projectPath, input.SetSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("workbook.update", input.Environment); err != nil {
			return err
		}
		if !command.Flags().Changed("new-name") && !command.Flags().Changed("owner-id") && !command.Flags().Changed("description") {
			return clierr.Usage("workbook.update", errors.New("at least one of --new-name, --owner-id, or --description is required"))
		}
		if command.Flags().Changed("description") {
			input.Description = &description
		}
		if command.Flags().Changed("new-name") {
			input.Name = &newName
		}
		if command.Flags().Changed("owner-id") {
			input.OwnerLUID = &ownerLUID
		}
		result, err := deps.WorkbookUpdater.UpdateWorkbook(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	contentTargetFlags(command, &input.Environment, &luid, &name, &projectPath, "workbook")
	updateFlags(command, &newName, &ownerLUID, true)
	command.Flags().StringVar(&description, "description", "", "replace the workbook description; an explicit empty value clears it")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

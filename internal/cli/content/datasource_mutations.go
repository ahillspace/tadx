package content

import (
	"errors"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newDatasourceMove(deps Dependencies) *cobra.Command {
	var input datasourceops.MoveInput
	var luid, name, sourceProject, destinationLUID, destinationPath string
	var preview bool
	command := mutationCommand("datasource.move", "move", "Move one exact published datasource.", func(command *cobra.Command) error {
		if err := selectorArgs("datasource.move", &luid, &name, &sourceProject, input.SetDatasourceSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("datasource.move", input.Environment); err != nil {
			return err
		}
		if (destinationLUID == "") == (destinationPath == "") {
			return clierr.Usage("datasource.move", errors.New("use exactly one of --destination-project-id or --destination-project"))
		}
		input.SetProjectSelector(destinationLUID, destinationPath)
		result, err := deps.DatasourceMover.MoveDatasource(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	contentTargetFlags(command, &input.Environment, &luid, &name, &sourceProject, "datasource")
	destinationProjectFlags(command, &destinationLUID, &destinationPath)
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newDatasourceUpdate(deps Dependencies) *cobra.Command {
	var input datasourceops.UpdateInput
	var luid, name, projectPath, newName, ownerLUID string
	var preview bool
	command := mutationCommand("datasource.update", "update", "Update one exact published datasource.", func(command *cobra.Command) error {
		if err := selectorArgs("datasource.update", &luid, &name, &projectPath, input.SetSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("datasource.update", input.Environment); err != nil {
			return err
		}
		if !command.Flags().Changed("new-name") && !command.Flags().Changed("owner-id") {
			return clierr.Usage("datasource.update", errors.New("at least one of --new-name or --owner-id is required"))
		}
		if command.Flags().Changed("new-name") {
			input.Name = &newName
		}
		if command.Flags().Changed("owner-id") {
			input.OwnerLUID = &ownerLUID
		}
		result, err := deps.DatasourceUpdater.UpdateDatasource(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	contentTargetFlags(command, &input.Environment, &luid, &name, &projectPath, "datasource")
	updateFlags(command, &newName, &ownerLUID, true)
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

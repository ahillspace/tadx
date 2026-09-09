package content

import (
	"errors"

	datasourcemove "github.com/ahillspace/tadx/actions/datasource/move"
	datasourceupdate "github.com/ahillspace/tadx/actions/datasource/update"
	flowupdate "github.com/ahillspace/tadx/actions/flow/update"
	projectmove "github.com/ahillspace/tadx/actions/project/move"
	workbookmove "github.com/ahillspace/tadx/actions/workbook/move"
	workbookupdate "github.com/ahillspace/tadx/actions/workbook/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newWorkbookMove(deps Dependencies) *cobra.Command {
	var input workbookmove.Input
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
	var input workbookupdate.Input
	var luid, name, projectPath, newName, ownerLUID string
	var preview bool
	command := mutationCommand("workbook.update", "update", "Update one exact workbook.", func(command *cobra.Command) error {
		if err := selectorArgs("workbook.update", &luid, &name, &projectPath, input.SetSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("workbook.update", input.Environment); err != nil {
			return err
		}
		if !command.Flags().Changed("new-name") && !command.Flags().Changed("owner-id") {
			return clierr.Usage("workbook.update", errors.New("at least one of --new-name or --owner-id is required"))
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
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newDatasourceMove(deps Dependencies) *cobra.Command {
	var input datasourcemove.Input
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
	var input datasourceupdate.Input
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

func newFlowUpdate(deps Dependencies) *cobra.Command {
	var input flowupdate.Input
	var luid, name, projectPath, ownerLUID string
	var preview bool
	command := mutationCommand("flow.update", "update", "Update one exact flow owner.", func(command *cobra.Command) error {
		if err := selectorArgs("flow.update", &luid, &name, &projectPath, input.SetSelector)(command, nil); err != nil {
			return err
		}
		if err := requireWriteEnvironment("flow.update", input.Environment); err != nil {
			return err
		}
		if !command.Flags().Changed("owner-id") {
			return clierr.Usage("flow.update", errors.New("--owner-id is required"))
		}
		input.OwnerLUID = &ownerLUID
		result, err := deps.FlowUpdater.UpdateFlow(command.Context(), input, preview)
		if err != nil {
			return clierr.WithOutput(result, err)
		}
		return deps.Renderer.Render(result)
	})
	contentTargetFlags(command, &input.Environment, &luid, &name, &projectPath, "flow")
	command.Flags().StringVar(&ownerLUID, "owner-id", "", "replacement owner LUID")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote mutation without performing it")
	return command
}

func newProjectMove(deps Dependencies) *cobra.Command {
	var input projectmove.Input
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

func mutationCommand(capabilityID, use, short string, run func(*cobra.Command) error) *cobra.Command {
	return &cobra.Command{
		Use:         use,
		Short:       short,
		Annotations: map[string]string{"tadx.capability": capabilityID},
		Args:        noContentArgs(capabilityID),
		RunE: func(command *cobra.Command, _ []string) error {
			return run(command)
		},
	}
}

func requireWriteEnvironment(operation, environment string) error {
	if environment == "" {
		return clierr.Usage(operation, errors.New("--environment is required for a remote mutation"))
	}
	return nil
}

func contentTargetFlags(command *cobra.Command, environment, luid, name, projectPath *string, kind string) {
	command.Flags().StringVar(environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(luid, "id", "", "authoritative "+kind+" LUID")
	command.Flags().StringVar(name, "name", "", "exact "+kind+" name")
	command.Flags().StringVar(projectPath, "project", "", "exact slash-delimited project path")
}

func destinationProjectFlags(command *cobra.Command, luid, projectPath *string) {
	command.Flags().StringVar(luid, "destination-project-id", "", "authoritative destination project LUID")
	command.Flags().StringVar(projectPath, "destination-project", "", "exact destination project path")
}

func updateFlags(command *cobra.Command, name, ownerLUID *string, includeName bool) {
	if includeName {
		command.Flags().StringVar(name, "new-name", "", "replacement content name")
	}
	command.Flags().StringVar(ownerLUID, "owner-id", "", "replacement owner LUID")
}

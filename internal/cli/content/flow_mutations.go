package content

import (
	"errors"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newFlowUpdate(deps Dependencies) *cobra.Command {
	var input flowops.UpdateInput
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

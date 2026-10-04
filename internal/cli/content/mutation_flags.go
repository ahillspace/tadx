package content

import (
	"errors"

	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

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

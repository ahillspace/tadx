package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// assetHelpFacts records selectors shared by workbook, datasource, and flow commands.
func assetHelpFacts(command *cobra.Command, action string, scopedPull bool) {
	switch action {
	case "inspect", "pull", "update", "move", "delete":
		helpmeta.Group(command, "exactly-one", "id", "name")
		if action == "inspect" && command.Flags().Lookup("project-id") != nil {
			helpmeta.Constraint(command, "--name requires exactly one of --project or --project-id; --id excludes name/project selectors.")
		} else if action == "pull" && scopedPull {
			helpmeta.Constraint(command, "--project scopes an exact --name selector when supplied.")
		} else {
			helpmeta.Constraint(command, "--name requires --project; --id excludes both.")
		}
	case "publish":
		helpmeta.Group(command, "exactly-one", "artifact", "file", "id", "artifact-name")
		helpmeta.Group(command, "exactly-one", "project-id", "project")
	}
	if action == "move" {
		helpmeta.Group(command, "exactly-one", "destination-project-id", "destination-project")
	}
}

package agent

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "agent" {
		helpmeta.Summary(command, "Install and remove bundled agent guidance")
	}
	switch path {
	case "agent uninstall":
		helpmeta.Required(command, "target")
	}
}

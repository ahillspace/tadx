package update

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "update" {
		helpmeta.Summary(command, "Update the CLI and guidance")
	}
	switch path {
	case "update":
		helpmeta.FlagNote(command, "target", "at most 32 entries")
	}
}

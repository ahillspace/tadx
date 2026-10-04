package cli

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// applyCompletionHelpFacts attaches this command family's presentation-only contracts.
func applyCompletionHelpFacts(command *cobra.Command, path string) {
	if path == "completion" {
		helpmeta.Summary(command, "Shell completion setup")
	}
}

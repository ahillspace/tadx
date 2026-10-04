package doctor

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "doctor" {
		helpmeta.Summary(command, "Diagnose configuration and connectivity")
	}
}

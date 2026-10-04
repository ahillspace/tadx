package auth

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "auth" {
		helpmeta.Summary(command, "PAT login, checks, and logout")
	}
	switch path {
	case "auth login", "auth logout":
		helpmeta.Required(command, "environment")
	}
}

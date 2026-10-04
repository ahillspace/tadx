package cli

import (
	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// applySearchHelpFacts attaches this command family's presentation-only contracts.
func applySearchHelpFacts(command *cobra.Command, path string) {
	if path == "search" {
		helpmeta.Summary(command, "Find content, users, groups, and Pulse objects")
	}
	switch path {
	case "search":
		types, _ := searchaction.Types("")
		helpmeta.Choices(command, "type", append(types, "content", "admin", "pulse")...)
		helpmeta.Value(command, "limit", "1..2000")
	}
}

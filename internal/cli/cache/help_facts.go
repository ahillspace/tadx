package cache

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "cache" {
		helpmeta.Summary(command, "Refresh and inspect local Tableau inventory")
	}
	switch path {
	case "cache refresh":
		helpmeta.Choices(command, "scope", "users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions")
		helpmeta.FlagNote(command, "scope", "values must be unique; comma-separated values are also accepted")
	}
}

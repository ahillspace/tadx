package workspace

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "workspace" {
		helpmeta.Summary(command, "Registered local workspaces and downloaded files")
	}
	switch path {
	case "workspace clean":
		helpmeta.Choices(command, "class", "temporary", "cache", "logs", "all")
	case "workspace artifact delete", "workspace artifact move":
		helpmeta.Choices(command, "kind", "workbook", "datasource", "flow", "pulse-definition", "lineage")
	}
	switch path {
	case "workspace list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	case "workspace status":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	}
	switch path {
	case "workspace artifact move":
		helpmeta.Required(command, "source", "destination")
		helpmeta.FlagAnnotation(command, "source", "tadx.help.value", []string{"workspace-name"})
		helpmeta.FlagAnnotation(command, "destination", "tadx.help.value", []string{"workspace-name"})
		helpmeta.Constraint(command, "Select --artifact, or both --kind and --id; do not combine these selector forms.")
	case "workspace artifact delete":
		helpmeta.Required(command, "workspace")
		helpmeta.Constraint(command, "Select --artifact, or both --kind and --id; do not combine these selector forms.")
	case "workspace clean":
		helpmeta.Required(command, "workspace", "class")
	case "workspace register":
		helpmeta.Required(command, "path")
	case "workspace clone":
		helpmeta.Required(command, "name")
	}
}

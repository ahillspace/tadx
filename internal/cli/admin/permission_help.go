package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func permissionHelpFacts(command *cobra.Command, action string) {
	if action != "inspect" && action != "create" && action != "delete" {
		return
	}
	helpmeta.Choices(command, "kind", "workbook", "datasource", "flow", "project")
	helpmeta.Choices(command, "default-for", "workbooks", "datasources", "flows")
	helpmeta.Choices(command, "principal-type", "user", "group")
	helpmeta.Choices(command, "mode", "Allow", "Deny")
	helpmeta.FlagNote(command, "default-for", "requires --kind project")
	// The action's Long help already receives authoritative per-kind
	// capability lists through its composition-root dependency.
	if action == "inspect" {
		helpmeta.Required(command, "kind", "id")
	} else {
		helpmeta.Required(command, "kind", "id", "principal-type", "capability", "mode")
		helpmeta.Group(command, "exactly-one", "principal-id", "principal-username")
		helpmeta.Constraint(command, "--principal-username requires --principal-type user; --default-for requires --kind project.")
	}
}

func permissionHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin permission", Note: "Inspect requires --kind and --id. Create and delete also require --principal-type, a principal selector, --capability, and --mode.\nUse --principal-id, or --principal-username with --principal-type user. Repeat --capability for multiple rules.", Common: false, Examples: []string{"tadx admin permission create --env dev --kind workbook --id <workbook-luid> --principal-type group --principal-id <group-luid> --capability Read --mode Allow --preview"}},
	}
}

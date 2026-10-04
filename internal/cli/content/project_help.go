package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func projectHelpFacts(command *cobra.Command, action string) {
	if action == "create" || action == "update" {
		helpmeta.Choices(command, "content-permissions", "ManagedByOwner", "LockedToProject", "LockedToProjectWithoutNested")
	}
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch action {
	case "create":
		helpmeta.Required(command, "name")
		helpmeta.Group(command, "exclusive", "parent-id", "parent")
	case "inspect":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
	case "delete":
		helpmeta.Group(command, "exactly-one", "id", "project-id")
	case "update":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
		helpmeta.Group(command, "one-required", "new-name", "name", "description", "content-permissions")
	case "move":
		helpmeta.Group(command, "exactly-one", "id", "project-id", "project")
		helpmeta.Group(command, "exactly-one", "parent-id", "parent", "top-level")
	}
}

func projectHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "content project", Note: "Create requires --name. Inspect and update accept --id or --project; delete requires --id. Project paths use forward slashes.", Common: false, Examples: []string{"tadx content project create --env dev --name <project-name> --parent-id <parent-project-luid> --preview", "tadx content project move --project Ops/Reports --top-level --preview"}},
	}
}

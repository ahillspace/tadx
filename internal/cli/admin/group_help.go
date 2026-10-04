package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func groupHelpFacts(command *cobra.Command, action string) {
	if action == "create" || action == "update" {
		helpmeta.FlagNote(command, "minimum-site-role", "accepted roles depend on the Tableau site and version")
	}
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	switch action {
	case "create":
		helpmeta.Required(command, "name")
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		command.Annotations["tadx.help.batch-example"] = `{"items":[{"name":"analysts"},{"name":"publishers"}]}`
	case "inspect":
		helpmeta.Group(command, "exactly-one", "id", "name")
	case "delete":
		helpmeta.Required(command, "id")
	case "update":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "one-required", "new-name", "minimum-site-role", "external-user-enabled", "set-members")
		helpmeta.Constraint(command, "--member-id requires --set-members; --set-members without --member-id removes all direct members.")
	}
}

func groupHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin group", Note: "Create requires --name. Inspect accepts --id or --name. Update and delete require --id.", Common: false, Examples: []string{"tadx admin group update --env dev --id <group-luid> --set-members --member-id <user-luid> --member-id <second-user-luid> --preview"}},
	}
}

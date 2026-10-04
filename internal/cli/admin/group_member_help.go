package admin

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func groupMemberHelpFacts(command *cobra.Command, action string) {
	if action == "add" || action == "remove" {
		helpmeta.Required(command, "group-id")
		helpmeta.Group(command, "exactly-one", "user-id", "username")
	}
}

func groupMemberHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "admin group-member", Note: "Add and remove require --group-id and exactly one of --user-id or --username.", Common: false, Examples: []string{}},
	}
}

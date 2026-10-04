package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func flowHelpFacts(command *cobra.Command, action string) {
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	assetHelpFacts(command, action, false)
	if action == "update" {
		helpmeta.Required(command, "owner-id")
	}
}

func flowHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "content flow", Note: "Inspect requires --id, or --name with --project or --project-id. Pull requires --id, or --name with --project. Publish requires an artifact selector and a destination project.", Common: false, Examples: []string{"tadx content flow publish --env dev --workspace dev --id <flow-luid> --project-id <project-luid> --preview"}},
	}
}

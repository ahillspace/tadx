package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func workbookHelpFacts(command *cobra.Command, action string) {
	if action == "list" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	assetHelpFacts(command, action, true)
	if action == "update" {
		helpmeta.Group(command, "one-required", "new-name", "owner-id", "description")
	}
}

func workbookHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "content workbook", Note: "Inspect requires --id, or --name with --project or --project-id. Pull accepts --id or --name, with --project to scope names.\nPublish requires an artifact selector and a destination project.", Common: false, Examples: []string{"tadx content workbook pull --env dev --workspace dev --id <workbook-luid> --id <second-workbook-luid>", "tadx content workbook publish --env dev --workspace dev --id <workbook-luid> --project-id <project-luid> --preview"}},
	}
}

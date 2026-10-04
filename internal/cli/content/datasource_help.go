package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

func datasourceHelpFacts(command *cobra.Command, action string) {
	if action == "schema" {
		helpmeta.Choices(command, "role", "measure", "dimension", "date", "excluded")
		helpmeta.FlagNote(command, "field-id", "at most 10000 distinct identifiers")
	}
	if action == "list" {
		helpmeta.Value(command, "updated-after", "RFC3339")
		helpmeta.Value(command, "updated-before", "RFC3339")
		helpmeta.FlagNote(command, "type", "exact provider datasource type; not a search resource family")
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	}
	if action == "schema" {
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
		helpmeta.Required(command, "id")
	}
	assetHelpFacts(command, action, false)
	if action == "publish" {
		helpmeta.Group(command, "exactly-one", "create", "overwrite", "append", "replace")
	}
	if action == "update" {
		helpmeta.Group(command, "one-required", "new-name", "owner-id")
	}
}

func appendDatasourcePublishSyntaxNote(command *cobra.Command) {
	helpmeta.AppendNote(command, "Select exactly one publish mode: --create, --overwrite, --append, or --replace.")
}

func datasourceHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "content datasource", Note: "Inspect requires --id, or --name with --project or --project-id. Pull requires --id, or --name with --project. Schema requires --id.\nPublish requires exactly one of --create, --overwrite, --append, or --replace.", Common: false, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --role measure", "tadx content datasource publish --env dev --workspace dev --id <datasource-luid> --project-id <project-luid> --create --preview"}},
		{Path: "content datasource schema", Note: "Repeat --field-id for exact fields. Use --all or --limit, but not both.", Common: false, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>", "tadx content datasource schema --env dev --id <datasource-luid> --field-id <field-id> --field-id <second-field-id> --descriptions"}},
	}
}

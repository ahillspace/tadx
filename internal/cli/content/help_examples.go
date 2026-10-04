package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "content", Note: "Selectors use exact names, project paths, or authoritative LUIDs. Ambiguous names fail.", Common: true, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>"}},
		{Path: "content workbook", Note: "Inspect requires --id, or --name with --project or --project-id. Pull accepts --id or --name, with --project to scope names.\nPublish requires an artifact selector and a destination project.", Common: false, Examples: []string{"tadx content workbook pull --env dev --workspace dev --id <workbook-luid> --id <second-workbook-luid>", "tadx content workbook publish --env dev --workspace dev --id <workbook-luid> --project-id <project-luid> --preview"}},
		{Path: "content datasource", Note: "Inspect requires --id, or --name with --project or --project-id. Pull requires --id, or --name with --project. Schema requires --id.\nPublish requires exactly one of --create, --overwrite, --append, or --replace.", Common: false, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --role measure", "tadx content datasource publish --env dev --workspace dev --id <datasource-luid> --project-id <project-luid> --create --preview"}},
		{Path: "content datasource schema", Note: "Repeat --field-id for exact fields. Use --all or --limit, but not both.", Common: false, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>", "tadx content datasource schema --env dev --id <datasource-luid> --field-id <field-id> --field-id <second-field-id> --descriptions"}},
		{Path: "content flow", Note: "Inspect requires --id, or --name with --project or --project-id. Pull requires --id, or --name with --project. Publish requires an artifact selector and a destination project.", Common: false, Examples: []string{"tadx content flow publish --env dev --workspace dev --id <flow-luid> --project-id <project-luid> --preview"}},
		{Path: "content project", Note: "Create requires --name. Inspect and update accept --id or --project; delete requires --id. Project paths use forward slashes.", Common: false, Examples: []string{"tadx content project create --env dev --name <project-name> --parent-id <parent-project-luid> --preview", "tadx content project move --project Ops/Reports --top-level --preview"}},
	}
}

// ApplySyntaxNotes attaches content publication notes without changing commands.
func ApplySyntaxNotes(root *cobra.Command) {
	var addSyntax func(*cobra.Command)
	addSyntax = func(command *cobra.Command) {
		if command.Name() == "publish" && command.Parent() != nil && command.Parent().Parent() != nil && command.Parent().Parent().Name() == "content" {
			helpmeta.AppendNote(command, "Select one source: --artifact, --file, --id, or --artifact-name. Choose a destination with --project-id or --project.\nRepeat --artifact for a same-action batch. Managed artifact paths are workspace-relative and use forward slashes.")
			if command.Parent().Name() == "datasource" {
				helpmeta.AppendNote(command, "Select exactly one publish mode: --create, --overwrite, --append, or --replace.")
			}
		}
		for _, child := range command.Commands() {
			addSyntax(child)
		}
	}
	addSyntax(root)
}

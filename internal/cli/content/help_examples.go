package content

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// HelpExamples supplies content examples in the existing command order.
func HelpExamples() []helpmeta.ExampleSet {
	sets := []helpmeta.ExampleSet{
		{Path: "content", Note: "Selectors use exact names, project paths, or authoritative LUIDs. Ambiguous names fail.", Common: true, Examples: []string{"tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>"}},
	}
	sets = append(sets, workbookHelpExamples()...)
	sets = append(sets, datasourceHelpExamples()...)
	sets = append(sets, flowHelpExamples()...)
	return append(sets, projectHelpExamples()...)
}

// ApplySyntaxNotes attaches content publication notes without changing commands.
func ApplySyntaxNotes(root *cobra.Command) {
	var addSyntax func(*cobra.Command)
	addSyntax = func(command *cobra.Command) {
		if command.Name() == "publish" && command.Parent() != nil && command.Parent().Parent() != nil && command.Parent().Parent().Name() == "content" {
			helpmeta.AppendNote(command, "Select one source: --artifact, --file, --id, or --artifact-name. Choose a destination with --project-id or --project.\nRepeat --artifact for a same-action batch. Managed artifact paths are workspace-relative and use forward slashes.")
			if command.Parent().Name() == "datasource" {
				appendDatasourcePublishSyntaxNote(command)
			}
		}
		for _, child := range command.Commands() {
			addSyntax(child)
		}
	}
	addSyntax(root)
}

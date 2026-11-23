package content

import (
	"context"

	"github.com/ahillspace/tadx/actions/structure"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// StructureDescriber executes workbook.structure or datasource.structure.
type StructureDescriber interface {
	Operation() string
	Execute(context.Context, structure.Input) (structure.Report, error)
}

func newStructure(describer StructureDescriber, renderer Renderer, document string) *cobra.Command {
	var input structure.Input
	operation := describer.Operation()
	command := &cobra.Command{
		Use:   "structure <element>",
		Short: "Show what the Tableau schema allows at one " + document + " element; local only.",
		Long: "Show what the Tableau document schema allows at one " + document + " element, so XML written by hand loads in Tableau.\n\n" +
			"Name an element, such as table, or a chain that ends at it, such as \"worksheet > table\"; a name used in several places returns each declaration with the paths that reach it.\n" +
			"Children use the order notation of schema validation messages: ? optional, * any number, + one or more, {m,n} a count, | one of, & any order; any is an element the schema leaves unchecked.\n" +
			"--full adds the schema's documentation text. For example: tadx content " + document + " structure \"worksheet > table\", or tadx content " + document + " structure column --attribute role --full",
		Annotations: map[string]string{"tadx.capability": operation},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.ExactArgs(1)(command, args); err != nil {
				return clierr.Usage(operation, err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			input.Element = args[0]
			report, err := describer.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return renderer.Render(report)
		},
	}
	command.Flags().StringVar(&input.Attribute, "attribute", "", "show only this attribute of the element")
	command.Flags().StringVar(&input.SchemaVersion, "schema-version", "", "Tableau release whose schema to read, such as 2025.1 (default newest)")
	return command
}

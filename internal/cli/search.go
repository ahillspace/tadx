package cli

import (
	"errors"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newSearch(searcher Searcher, renderer Renderer) *cobra.Command {
	var input searchaction.Input
	command := &cobra.Command{
		Use:         "search [term]",
		Short:       "Search Tableau resources.",
		Annotations: map[string]string{CapabilityAnnotation: "search.run"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(command, args); err != nil {
				return clierr.Usage("search", err)
			}
			if len(args) == 1 {
				input.Terms = args[0]
			}
			if input.Terms == "" && input.Type == "" {
				return clierr.Usage("search", errors.New("a search term or --type is required"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := searcher.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Type, "type", "", "resource type or family: workbook, datasource, flow, project, user, group, definition, metric, content, admin, or pulse")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "use local catalog data without contacting Tableau")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "continue from a prior result cursor")
	command.Flags().IntVar(&input.Limit, "limit", 20, "maximum records to return")
	return command
}

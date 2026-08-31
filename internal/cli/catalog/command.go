// Package catalog contains thin Cobra plumbing for local catalog commands.
package catalog

import (
	"context"

	search "github.com/ahillspace/tadx/actions/catalog/search"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Searcher executes catalog.search.
type Searcher interface {
	Execute(context.Context, search.Input) (search.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains catalog command wiring.
type Dependencies struct {
	Searcher Searcher
	Renderer Renderer
	Use      string
	Short    string
}

// New creates the catalog domain.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "catalog", Short: "Search normalized local Tableau inventory"}
	use := deps.Use
	if use == "" {
		use = "search [text]"
	}
	short := deps.Short
	if short == "" {
		short = "Search cached inventory."
	}
	var input search.Input
	searchCommand := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "catalog.search"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.MaximumNArgs(1)(command, args); err != nil {
				return clierr.Usage("catalog.search", err)
			}
			if len(args) == 1 {
				input.Text = args[0]
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Searcher.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	searchCommand.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	searchCommand.Flags().StringVar(&input.Site, "site", "", "exact source site content URL")
	searchCommand.Flags().StringVar(&input.Kind, "kind", "", "exact resource kind")
	searchCommand.Flags().StringVar(&input.ProjectPath, "project", "", "exact slash-delimited project path")
	searchCommand.Flags().StringVar(&input.Owner, "owner", "", "exact owner")
	searchCommand.Flags().StringVar(&input.LUID, "id", "", "authoritative Tableau LUID")
	searchCommand.Flags().StringVar(&input.Cursor, "cursor", "", "continue from a prior result cursor")
	searchCommand.Flags().IntVar(&input.Limit, "limit", 20, "maximum records to return")
	command.AddCommand(searchCommand)
	return command
}

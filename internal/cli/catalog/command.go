// Package catalog contains thin Cobra plumbing for local catalog commands.
package catalog

import (
	"context"

	catalogget "github.com/ahillspace/tadx/actions/catalog/get"
	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	search "github.com/ahillspace/tadx/actions/catalog/search"
	catalogstatus "github.com/ahillspace/tadx/actions/catalog/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Searcher executes catalog.search.
type Searcher interface {
	Execute(context.Context, search.Input) (search.Output, error)
}

// Refresher executes catalog.refresh.
type Refresher interface {
	Execute(context.Context, catalogrefresh.Input) (catalogrefresh.Output, error)
}

// Getter executes catalog.get.
type Getter interface {
	Execute(context.Context, catalogget.Input) (catalogget.Output, error)
}

// Statuser executes catalog.status.
type Statuser interface {
	Execute(context.Context, catalogstatus.Input) (catalogstatus.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains catalog command wiring.
type Dependencies struct {
	Searcher     Searcher
	Refresher    Refresher
	Getter       Getter
	Statuser     Statuser
	Renderer     Renderer
	Use          string
	Short        string
	RefreshUse   string
	RefreshShort string
	GetUse       string
	GetShort     string
	StatusUse    string
	StatusShort  string
}

// New creates the catalog domain.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "catalog", Short: "Inspect and refresh normalized local Tableau inventory"}
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
	if deps.Refresher != nil {
		command.AddCommand(newRefreshCommand(deps))
	}
	if deps.Getter != nil {
		command.AddCommand(newGetCommand(deps))
	}
	if deps.Statuser != nil {
		command.AddCommand(newStatusCommand(deps))
	}
	return command
}

func newRefreshCommand(deps Dependencies) *cobra.Command {
	input := catalogrefresh.Input{}
	use := deps.RefreshUse
	if use == "" {
		use = "refresh"
	}
	short := deps.RefreshShort
	if short == "" {
		short = "Refresh one complete local catalog generation."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "catalog.refresh"}, Args: noArgs("catalog.refresh"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Refresher.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Site, "site", "", "exact source site content URL")
	command.Flags().StringSliceVar(&input.Scopes, "scope", nil, "inventory scope; repeat for users, groups, projects, workbooks, datasources, flows, views, or permissions; omit for all")
	return command
}

func newGetCommand(deps Dependencies) *cobra.Command {
	var input catalogget.Input
	use := deps.GetUse
	if use == "" {
		use = "get"
	}
	short := deps.GetShort
	if short == "" {
		short = "Inspect one exact cached catalog record."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "catalog.get"}, Args: noArgs("catalog.get"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Getter.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Site, "site", "", "exact source site content URL")
	command.Flags().StringVar(&input.Kind, "kind", "", "exact resource kind")
	command.Flags().StringVar(&input.Name, "name", "", "exact resource name")
	command.Flags().StringVar(&input.ProjectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&input.LUID, "id", "", "authoritative Tableau LUID")
	return command
}

func newStatusCommand(deps Dependencies) *cobra.Command {
	var input catalogstatus.Input
	use := deps.StatusUse
	if use == "" {
		use = "status"
	}
	short := deps.StatusShort
	if short == "" {
		short = "Report current local catalog generation status."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "catalog.status"}, Args: noArgs("catalog.status"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Statuser.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Site, "site", "", "exact source site content URL")
	return command
}

func noArgs(operation string) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}

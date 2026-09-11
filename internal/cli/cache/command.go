// Package cache contains thin Cobra plumbing for local cache commands.
package cache

import (
	"context"

	cacherefresh "github.com/ahillspace/tadx/actions/cache/refresh"
	cachestatus "github.com/ahillspace/tadx/actions/cache/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Refresher executes cache.refresh.
type Refresher interface {
	Execute(context.Context, cacherefresh.Input) (cacherefresh.Output, error)
}

// Statuser executes cache.status.
type Statuser interface {
	Execute(context.Context, cachestatus.Input) (cachestatus.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains cache command wiring.
type Dependencies struct {
	Refresher    Refresher
	Statuser     Statuser
	Renderer     Renderer
	RefreshUse   string
	RefreshShort string
	StatusUse    string
	StatusShort  string
}

// New creates the cache domain.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "cache", Short: "Inspect and refresh normalized local Tableau inventory"}
	if deps.Refresher != nil {
		command.AddCommand(newRefreshCommand(deps))
	}
	if deps.Statuser != nil {
		command.AddCommand(newStatusCommand(deps))
	}
	return command
}

func newRefreshCommand(deps Dependencies) *cobra.Command {
	input := cacherefresh.Input{}
	use := deps.RefreshUse
	if use == "" {
		use = "refresh"
	}
	short := deps.RefreshShort
	if short == "" {
		short = "Refresh one complete local cache generation."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "cache.refresh"}, Args: noArgs("cache.refresh"),
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

func newStatusCommand(deps Dependencies) *cobra.Command {
	var input cachestatus.Input
	use := deps.StatusUse
	if use == "" {
		use = "status"
	}
	short := deps.StatusShort
	if short == "" {
		short = "Report current local cache generation status."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "cache.status"}, Args: noArgs("cache.status"),
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

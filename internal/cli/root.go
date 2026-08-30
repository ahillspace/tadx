// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"

	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

// Lister executes capability list.
type Lister interface {
	Execute(context.Context, capabilitylist.Input) (capabilitylist.Output, error)
}

// Getter executes capability get.
type Getter interface {
	Execute(context.Context, capabilityget.Input) (capabilityget.Output, error)
}

// Renderer writes structured command output.
type Renderer interface {
	Render(any) error
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Lister           Lister
	Getter           Getter
	Renderer         Renderer
	MutationsEnabled bool
	ListUse          string
	ListShort        string
	GetUse           string
	GetShort         string
}

// NewRoot creates the root command. It contains no domain behavior.
func NewRoot(deps Dependencies) *cobra.Command {
	root := &cobra.Command{
		Use:           "tadx",
		Short:         "Deterministic Tableau lifecycle and development CLI",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.AddCommand(capabilitycli.New(capabilitycli.Dependencies{
		Lister:           deps.Lister,
		Getter:           deps.Getter,
		Renderer:         deps.Renderer,
		MutationsEnabled: deps.MutationsEnabled,
		ListUse:          deps.ListUse,
		ListShort:        deps.ListShort,
		GetUse:           deps.GetUse,
		GetShort:         deps.GetShort,
	}))
	root.CompletionOptions.DisableDefaultCmd = true
	setFlagErrorHandlers(root)
	return root
}

func setFlagErrorHandlers(command *cobra.Command) {
	command.SetFlagErrorFunc(func(command *cobra.Command, cause error) error {
		return &errs.Error{Kind: errs.KindUsage, Operation: command.CommandPath(), Summary: cause.Error(), Cause: cause}
	})
	for _, child := range command.Commands() {
		setFlagErrorHandlers(child)
	}
}

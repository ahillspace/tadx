// Package auth contains thin Cobra plumbing for authentication commands.
package auth

import (
	"context"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Checker executes auth.check.
type Checker interface {
	Execute(context.Context, authcheck.Input) (authcheck.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains auth command wiring.
type Dependencies struct {
	Checker  Checker
	Renderer Renderer
	Use      string
	Short    string
}

// New creates the auth domain.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Inspect Tableau authentication"}
	use := deps.Use
	if use == "" {
		use = "check"
	}
	short := deps.Short
	if short == "" {
		short = "Verify PAT authentication."
	}
	var environment string
	check := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "auth.check"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("auth.check", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Checker.Execute(command.Context(), authcheck.Input{Environment: environment})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	check.Flags().StringVar(&environment, "environment", "", "exact environment alias; defaults to configured read environment")
	command.AddCommand(check)
	return command
}

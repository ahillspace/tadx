// Package auth contains thin Cobra plumbing for authentication commands.
package auth

import (
	"context"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Checker executes auth.check.
type Checker interface {
	Execute(context.Context, authcheck.Input) (authcheck.Output, error)
}

// Statuser inspects local PAT-reference readiness without contacting Tableau.
type Statuser interface {
	Execute(context.Context, authstatus.Input) (authstatus.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains auth command wiring.
type Dependencies struct {
	Checker     Checker
	Statuser    Statuser
	Renderer    Renderer
	Use         string
	Short       string
	StatusUse   string
	StatusShort string
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
	if deps.Statuser != nil {
		statusUse := deps.StatusUse
		if statusUse == "" {
			statusUse = "status"
		}
		statusShort := deps.StatusShort
		if statusShort == "" {
			statusShort = "Report local authentication readiness."
		}
		var statusEnvironment string
		status := &cobra.Command{
			Use: statusUse, Short: statusShort, Annotations: map[string]string{"tadx.capability": "auth.status"},
			Args: func(command *cobra.Command, args []string) error {
				if err := cobra.NoArgs(command, args); err != nil {
					return clierr.Usage("auth.status", err)
				}
				return nil
			},
			RunE: func(command *cobra.Command, _ []string) error {
				result, err := deps.Statuser.Execute(command.Context(), authstatus.Input{Environment: statusEnvironment})
				if err != nil {
					return err
				}
				return deps.Renderer.Render(result)
			},
		}
		status.Flags().StringVar(&statusEnvironment, "environment", "", "exact environment alias; defaults to configured read environment")
		command.AddCommand(status)
	}
	return command
}

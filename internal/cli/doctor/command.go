// Package doctor contains thin Cobra plumbing for doctor.run.
package doctor

import (
	"context"

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

// Runner executes doctor.run.
type Runner interface {
	Execute(context.Context, doctorrun.Input) (doctorrun.Output, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains doctor command wiring.
type Dependencies struct {
	Runner   Runner
	Renderer Renderer
	Use      string
	Short    string
}

// New creates the top-level doctor command.
func New(deps Dependencies) *cobra.Command {
	use := deps.Use
	if use == "" {
		use = "doctor"
	}
	short := deps.Short
	if short == "" {
		short = "Diagnose TADX configuration and service readiness."
	}
	var input doctorrun.Input
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "doctor.run"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("doctor.run", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			if deps.Runner == nil || deps.Renderer == nil {
				return &errs.Error{ID: "doctor.run.unconfigured", Kind: errs.KindRuntime, Operation: "doctor.run", Summary: "Doctor command is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the doctor runner and renderer before retrying."}
			}
			result, err := deps.Runner.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Workspace, "workspace", "", "logical workspace name; uses deterministic resolution when omitted")
	return command
}

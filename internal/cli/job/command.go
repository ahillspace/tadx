// Package job contains thin Cobra plumbing for exact Tableau job controls.
package job

import (
	"context"

	jobactions "github.com/ahillspace/tadx/actions/job"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains exact-job command wiring.
type Dependencies struct {
	Inspect                  func(context.Context, jobactions.InspectInput) (jobactions.InspectOutput, error)
	Wait                     func(context.Context, jobactions.WaitInput) (jobactions.WaitOutput, error)
	Cancel                   func(context.Context, jobactions.CancelInput) (jobactions.CancelOutput, error)
	Renderer                 Renderer
	InspectUse, InspectShort string
	WaitUse, WaitShort       string
	CancelUse, CancelShort   string
}

// New creates the top-level job command and only exposes configured actions.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "job", Short: "Inspect, recover, and control supported Tableau jobs"}
	if deps.Inspect != nil {
		command.AddCommand(newInspectCommand(deps))
	}
	if deps.Wait != nil {
		command.AddCommand(newWaitCommand(deps))
	}
	if deps.Cancel != nil {
		command.AddCommand(newCancelCommand(deps))
	}
	return command
}

func newInspectCommand(deps Dependencies) *cobra.Command {
	input := jobactions.InspectInput{}
	use, short := deps.InspectUse, deps.InspectShort
	if use == "" {
		use = "inspect"
	}
	if short == "" {
		short = "Inspect one exact Tableau job without changing remote state."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "job.inspect"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("job.inspect", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Inspect(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Site, "site", "", "exact site content URL; must match the selected environment")
	command.Flags().StringVarP(&input.ID, "id", "i", "", "exact Tableau job ID")
	command.Flags().StringVar(&input.OperationID, "operation-id", "", "exact detached operation ID")
	return command
}

func newWaitCommand(deps Dependencies) *cobra.Command {
	input := jobactions.WaitInput{}
	use, short := deps.WaitUse, deps.WaitShort
	if use == "" {
		use = "wait"
	}
	if short == "" {
		short = "Recover or begin observing one exact job without resubmitting it."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "job.wait"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("job.wait", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Wait(command.Context(), input)
			if err != nil {
				if result.Job.ID != "" {
					return clierr.WithOutput(result, err)
				}
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; receipt target is authoritative when omitted")
	command.Flags().StringVar(&input.Site, "site", "", "exact site content URL; receipt target is authoritative when omitted")
	command.Flags().StringVarP(&input.ID, "id", "i", "", "exact Tableau job ID")
	command.Flags().StringVar(&input.Receipt, "receipt", "", "durable accepted-job receipt path")
	return command
}

func newCancelCommand(deps Dependencies) *cobra.Command {
	input := jobactions.CancelInput{}
	use, short := deps.CancelUse, deps.CancelShort
	if use == "" {
		use = "cancel"
	}
	if short == "" {
		short = "Cancel one exact supported Tableau job, then confirm its state."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "job.cancel"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("job.cancel", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Cancel(command.Context(), input)
			if err != nil {
				if result.Job.ID != "" {
					return clierr.WithOutput(result, err)
				}
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias")
	command.Flags().StringVar(&input.Site, "site", "", "exact site content URL; must match the selected environment")
	command.Flags().StringVarP(&input.ID, "id", "i", "", "exact Tableau job ID")
	command.Flags().BoolVar(&input.Preview, "preview", false, "inspect the supported cancellation plan without changing Tableau")
	_ = command.MarkFlagRequired("id")
	return command
}

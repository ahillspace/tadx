// Package agent contains thin Cobra plumbing for agent skills.
package agent

import (
	"context"
	install "github.com/ahillspace/tadx/actions/agent/install"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// Installer runs one installation or preview.
type Installer interface {
	Execute(context.Context, install.Input) (install.Output, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	Installer  Installer
	Renderer   Renderer
	Use, Short string
}

// New creates the agent command group.
func New(deps Dependencies) *cobra.Command {
	var input install.Input
	use, short := deps.Use, deps.Short
	if use == "" {
		use = "install"
	}
	if short == "" {
		short = "Install bundled skills for Claude, Codex, or Cursor."
	}
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "agent.install"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("agent.install", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Installer.Execute(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Target, "target", "", "agent target: claude, codex, or cursor (required)")
	command.Flags().BoolVar(&input.Preview, "preview", false, "inspect installation without writing files")
	command.Flags().BoolVar(&input.Force, "force", false, "replace divergent skills and retain recoverable backups")
	group := &cobra.Command{Use: "agent", Short: "Install bundled agent skills"}
	group.AddCommand(command)
	return group
}

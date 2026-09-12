// Package update contains thin Cobra plumbing for coordinated updates.
package update

import (
	"context"
	action "github.com/ahillspace/tadx/actions/update"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Updater interface {
	Execute(context.Context, action.Input) (action.Output, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	Updater  Updater
	Renderer Renderer
	Use      string
	Short    string
}

func New(deps Dependencies) *cobra.Command {
	in := action.Input{}
	use := deps.Use
	if use == "" {
		use = "update"
	}
	short := deps.Short
	if short == "" {
		short = "Update TADX and bundled agent Guidance from the latest published release."
	}
	cmd := &cobra.Command{Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "update"}, Args: func(cmd *cobra.Command, args []string) error {
		if err := cobra.NoArgs(cmd, args); err != nil {
			return clierr.Usage("update", err)
		}
		return nil
	}, RunE: func(cmd *cobra.Command, _ []string) error {
		out, err := deps.Updater.Execute(cmd.Context(), in)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	cmd.Flags().BoolVar(&in.Check, "check", false, "check the latest release without changing the installation")
	cmd.Flags().StringArrayVar(&in.Targets, "target", nil, "agent target to refresh (repeatable; default: auto-detect)")
	agentcli.AnnotateTargetHelp(cmd.Flags().Lookup("target"), true)
	return cmd
}

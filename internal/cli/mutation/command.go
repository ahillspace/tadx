package mutation

import (
	"context"
	mutationset "github.com/ahillspace/tadx/actions/mutation/set"
	mutationstatus "github.com/ahillspace/tadx/actions/mutation/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Status interface {
	Execute(context.Context) (mutationstatus.Output, error)
}
type Setter interface {
	Execute(context.Context, bool) (mutationset.Output, error)
}
type Renderer interface{ Render(any) error }

func New(status Status, set Setter, renderer Renderer) *cobra.Command {
	root := &cobra.Command{Use: "mutation", Short: "Inspect or persist remote mutation execution policy."}
	get := &cobra.Command{Use: "status", Short: "Show effective mutation policy and whether it comes from this process or saved settings.", Annotations: map[string]string{"tadx.capability": "mutation.status"}, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		out, err := status.Execute(c.Context())
		if err != nil {
			return err
		}
		return renderer.Render(out)
	}}
	var enabled bool
	put := &cobra.Command{Use: "set", Short: "Persist user mutation policy until changed; process environment overrides the saved value.", Annotations: map[string]string{"tadx.capability": "mutation.set"}, Args: func(c *cobra.Command, args []string) error {
		if err := cobra.NoArgs(c, args); err != nil {
			return clierr.Usage("mutation.set", err)
		}
		return nil
	}, RunE: func(c *cobra.Command, _ []string) error {
		out, err := set.Execute(c.Context(), enabled)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return renderer.Render(out)
	}}
	put.Flags().BoolVar(&enabled, "enabled", false, "persist true to allow remote writes, or false to disable them; agents need explicit permission for this setting and persistent scope")
	_ = put.MarkFlagRequired("enabled")
	root.AddCommand(get, put)
	return root
}

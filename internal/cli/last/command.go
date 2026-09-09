package last

import (
	"context"
	lastaction "github.com/ahillspace/tadx/actions/last"
	"github.com/spf13/cobra"
)

type Reader interface {
	Execute(context.Context) (lastaction.Output, error)
}
type Renderer interface{ Render(any) error }

func New(reader Reader, renderer Renderer) *cobra.Command {
	return &cobra.Command{Use: "last", Short: "Display the previous execution's saved full output and timestamp. Never re-execute it.", Annotations: map[string]string{"tadx.capability": "last"}, Args: cobra.NoArgs, RunE: func(c *cobra.Command, _ []string) error {
		out, err := reader.Execute(c.Context())
		if err != nil {
			return err
		}
		return renderer.Render(out)
	}}
}

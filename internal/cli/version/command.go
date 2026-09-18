// Package version contains thin Cobra plumbing for version.get.
package version

import (
	"context"

	versionget "github.com/ahillspace/tadx/actions/version/get"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Getter interface {
	Execute(context.Context, versionget.Input) (versionget.Output, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	Getter   Getter
	Renderer Renderer
	Use      string
	Short    string
}

func New(deps Dependencies) *cobra.Command {
	var input versionget.Input
	use := deps.Use
	if use == "" {
		use = "version"
	}
	short := deps.Short
	if short == "" {
		short = "Show the installed TADX version."
	}
	command := &cobra.Command{Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "version.get"}, Args: func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage("version.get", err)
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		out, err := deps.Getter.Execute(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	command.Flags().BoolVar(&input.Check, "check", false, "check GitHub for the latest release")
	return command
}

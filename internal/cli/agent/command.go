// Package agent contains thin Cobra plumbing for agent skills.
package agent

import (
	"context"
	install "github.com/ahillspace/tadx/actions/agent/install"
	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Installer runs one installation or preview.
type Installer interface {
	Execute(context.Context, install.Input) (install.Output, error)
}
type Renderer interface{ Render(any) error }
type Uninstaller interface {
	Execute(context.Context, uninstall.Input) (uninstall.Output, error)
}
type Dependencies struct {
	Installer   Installer
	Uninstaller Uninstaller
	Renderer    Renderer
	Use, Short  string
}

// New creates the agent command group.
func New(deps Dependencies) *cobra.Command {
	var input install.Input
	use, short := deps.Use, deps.Short
	if use == "" {
		use = "install"
	}
	if short == "" {
		short = "Install bundled skills for supported coding agents."
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
				if len(result.Skills) > 0 {
					if renderErr := deps.Renderer.Render(result); renderErr != nil {
						return renderErr
					}
				}
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Target, "target", "auto", "agent target: auto detects configured agents; or "+agenttarget.Summary())
	AnnotateTargetHelp(command.Flags().Lookup("target"), true)
	command.Flags().BoolVar(&input.Preview, "preview", false, "inspect installation without writing files")
	command.Flags().BoolVar(&input.Force, "force", false, "compatibility flag; TADX-owned skills are always refreshed")
	group := &cobra.Command{Use: "agent", Short: "Manage bundled agent Guidance"}
	group.AddCommand(command)
	group.AddCommand(newUninstall(deps))
	return group
}

func newUninstall(deps Dependencies) *cobra.Command {
	var input uninstall.Input
	command := &cobra.Command{Use: "uninstall", Short: "Uninstall bundled agent Guidance.", Annotations: map[string]string{"tadx.capability": "agent.uninstall"}, Args: func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage("agent.uninstall", err)
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.Uninstaller.Execute(command.Context(), input)
		if err != nil {
			return err
		}
		return deps.Renderer.Render(result)
	}}
	command.Flags().StringVar(&input.Target, "target", "", "agent target: "+agenttarget.Summary()+" (required)")
	AnnotateTargetHelp(command.Flags().Lookup("target"), false)
	command.Flags().BoolVar(&input.Preview, "preview", false, "inspect uninstall changes without removing files")
	command.Flags().BoolVar(&input.Force, "force", false, "compatibility flag; edited TADX-owned Guidance is backed up on removal")
	return command
}

// AnnotateTargetHelp shares the target registry with installation and update help.
func AnnotateTargetHelp(flag *pflag.Flag, includeAuto bool) {
	values := agenttarget.SupportedTargets()
	if includeAuto {
		values = append([]string{"auto"}, values...)
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations["tadx.help.choices"] = values
}

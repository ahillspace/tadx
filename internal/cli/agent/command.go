// Package agent contains thin Cobra plumbing for agent skills.
package agent

import (
	"context"
	"github.com/ahillspace/tadx/actions/agent"
	"github.com/ahillspace/tadx/internal/agenttarget"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Service runs the two agent guidance operations.
type Service interface {
	Install(context.Context, agent.InstallInput) (agent.InstallOutput, error)
	Uninstall(context.Context, agent.UninstallInput) (agent.UninstallOutput, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	Service    Service
	Renderer   Renderer
	Use, Short string
}

// New creates the agent command group.
func New(deps Dependencies) *cobra.Command {
	var input agent.InstallInput
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
			result, err := deps.Service.Install(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
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
	var input agent.UninstallInput
	command := &cobra.Command{Use: "uninstall", Short: "Uninstall bundled agent Guidance.", Annotations: map[string]string{"tadx.capability": "agent.uninstall"}, Args: func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage("agent.uninstall", err)
		}
		return nil
	}, RunE: func(command *cobra.Command, _ []string) error {
		result, err := deps.Service.Uninstall(command.Context(), input)
		if err != nil {
			return clierr.WithOutput(result, err)
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

// Package policy contains thin Cobra bindings for managed policy recovery tools.
package policy

import (
	"context"

	policyops "github.com/ahillspace/tadx/actions/policy"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func noArgs(command *cobra.Command, args []string) error {
	if err := cobra.NoArgs(command, args); err != nil {
		return clierr.Usage(command.CommandPath(), err)
	}
	return nil
}

func oneArg(command *cobra.Command, args []string) error {
	if err := cobra.ExactArgs(1)(command, args); err != nil {
		return clierr.Usage(command.CommandPath(), err)
	}
	return nil
}

type Sampler interface {
	WriteSamples(context.Context, string) (policyops.SamplesOutput, error)
}
type Installer interface {
	InstallManagedPolicy(context.Context, policyops.InstallInput) (policyops.InstallOutput, error)
}
type Validator interface {
	ValidateCandidate(context.Context, string) (policyops.ValidationOutput, error)
}
type Statuser interface {
	ReadPolicy(context.Context) (policyops.StatusOutput, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	Installer Installer
	Sampler   Sampler
	Validator Validator
	Statuser  Statuser
	Renderer  Renderer
}

func New(deps Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "policy", Short: "Install, inspect, and prepare administrator-managed policy", Long: "Policy install, samples, validate, and status remain available for recovery when the active managed policy is invalid or denies other commands. Installation uses the native administrator boundary and does not change Tableau site consent."}
	var installDirectory, template string
	install := &cobra.Command{Use: "install", Short: "Install and activate a protected managed policy", Example: "tadx policy install --template read-only\ntadx policy install --template read-write-no-admin\ntadx policy install --template superuser\nsudo \"$(command -v tadx)\" policy install --template read-only", Annotations: map[string]string{"tadx.capability": "policy.install"}, Args: noArgs, RunE: func(c *cobra.Command, _ []string) error {
		out, err := deps.Installer.InstallManagedPolicy(c.Context(), policyops.InstallInput{OutputDirectory: installDirectory, Template: template})
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	install.Flags().StringVar(&installDirectory, "output", "", "managed policy directory (default: platform system TADX location)")
	install.Flags().StringVar(&template, "template", policyops.TemplateSuperuser, "policy template: read-only, read-write-no-admin, or superuser")
	var directory string
	samples := &cobra.Command{Use: "samples", Short: "Write three policy candidates without installing them", Annotations: map[string]string{"tadx.capability": "policy.samples"}, Args: noArgs, RunE: func(c *cobra.Command, _ []string) error {
		out, err := deps.Sampler.WriteSamples(c.Context(), directory)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return deps.Renderer.Render(out)
	}}
	samples.Flags().StringVar(&directory, "output", "", "directory for new candidate files; existing files are never overwritten")
	_ = samples.MarkFlagRequired("output")
	validate := &cobra.Command{Use: "validate <file>", Short: "Validate candidate schema and capability IDs without activation", Annotations: map[string]string{"tadx.capability": "policy.validate"}, Args: oneArg, RunE: func(c *cobra.Command, args []string) error {
		out, err := deps.Validator.ValidateCandidate(c.Context(), args[0])
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	status := &cobra.Command{Use: "status", Short: "Inspect fixed system policy and effective restrictions", Annotations: map[string]string{"tadx.capability": "policy.status"}, Args: noArgs, RunE: func(c *cobra.Command, _ []string) error {
		out, err := deps.Statuser.ReadPolicy(c.Context())
		if err != nil {
			return err
		}
		return deps.Renderer.Render(out)
	}}
	root.AddCommand(install, samples, validate, status)
	return root
}

// Package env contains thin Cobra plumbing for environment profile commands.
package env

import (
	"context"
	"errors"
	"strings"

	profileadd "github.com/ahillspace/tadx/actions/env/profile/add"
	profileget "github.com/ahillspace/tadx/actions/env/profile/get"
	profilelist "github.com/ahillspace/tadx/actions/env/profile/list"
	profileremove "github.com/ahillspace/tadx/actions/env/profile/remove"
	profilesetdefault "github.com/ahillspace/tadx/actions/env/profile/setdefault"
	profileupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type Lister interface {
	List(context.Context, profilelist.Input) (profilelist.Output, error)
}
type Getter interface {
	Get(context.Context, profileget.Input) (profileget.Output, error)
}
type Adder interface {
	Add(context.Context, profileadd.Input) (profileadd.Output, error)
}
type Updater interface {
	Update(context.Context, profileupdate.Input) (profileupdate.Output, error)
}
type Remover interface {
	Remove(context.Context, profileremove.Input) (profileremove.Output, error)
}
type DefaultSetter interface {
	SetDefault(context.Context, profilesetdefault.Input) (profilesetdefault.Output, error)
}
type Renderer interface{ Render(any) error }

type Dependencies struct {
	Lister        Lister
	Getter        Getter
	Adder         Adder
	Updater       Updater
	Remover       Remover
	DefaultSetter DefaultSetter
	Renderer      Renderer
	Uses          map[string]string
	Shorts        map[string]string
}

func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "env", Short: "Manage non-secret Tableau environment profiles"}
	command.AddCommand(newList(deps), newGet(deps), newAdd(deps), newUpdate(deps), newRemove(deps), newDefault(deps))
	return command
}

func newList(deps Dependencies) *cobra.Command {
	var input profilelist.Input
	command := &cobra.Command{
		Use: use(deps, "env.profile.list", "list"), Short: short(deps, "env.profile.list", "List environment profiles."),
		Annotations: map[string]string{"tadx.capability": "env.profile.list"}, Args: noArgs("env.profile.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Lister.List(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum profiles to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newGet(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: use(deps, "env.profile.get", "get <alias>"), Short: short(deps, "env.profile.get", "Inspect one environment profile."),
		Annotations: map[string]string{"tadx.capability": "env.profile.get"}, Args: exactAlias("env.profile.get"),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := deps.Getter.Get(command.Context(), profileget.Input{Alias: args[0]})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
}

func newAdd(deps Dependencies) *cobra.Command {
	var input profileadd.Input
	command := &cobra.Command{
		Use: use(deps, "env.profile.add", "add <alias>"), Short: short(deps, "env.profile.add", "Add an environment profile."),
		Annotations: map[string]string{"tadx.capability": "env.profile.add"},
		Args: func(command *cobra.Command, args []string) error {
			if err := exactAlias("env.profile.add")(command, args); err != nil {
				return err
			}
			if input.ServerURL == "" {
				return clierr.Usage("env.profile.add", errors.New("--url is required"))
			}
			input.Alias = args[0]
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Adder.Add(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	addProfileFlags(command, &input.ServerURL, &input.SiteContentURL, &input.APIVersion, &input.PATNameEnv, &input.PATSecretEnv, &input.DefaultWorkspace)
	return command
}

func newUpdate(deps Dependencies) *cobra.Command {
	var values profileValues
	var clears profileClears
	command := &cobra.Command{
		Use: use(deps, "env.profile.update", "update <alias>"), Short: short(deps, "env.profile.update", "Update an environment profile."),
		Annotations: map[string]string{"tadx.capability": "env.profile.update"},
		Args: func(command *cobra.Command, args []string) error {
			if err := exactAlias("env.profile.update")(command, args); err != nil {
				return err
			}
			for _, conflict := range []struct{ value, clear string }{{"site", "clear-site"}, {"api-version", "clear-api-version"}, {"pat-name-env", "clear-pat-name-env"}, {"pat-secret-env", "clear-pat-secret-env"}, {"default-workspace", "clear-default-workspace"}} {
				if command.Flags().Changed(conflict.value) && command.Flags().Changed(conflict.clear) {
					return clierr.Usage("env.profile.update", errors.New("--"+conflict.value+" and --"+conflict.clear+" cannot be used together"))
				}
			}
			return nil
		},
		RunE: func(command *cobra.Command, args []string) error {
			patch := profileupdate.Patch{
				ServerURL:        field(command, "url", values.serverURL, false),
				SiteContentURL:   field(command, "site", values.site, clears.site),
				APIVersion:       field(command, "api-version", values.apiVersion, clears.apiVersion),
				PATNameEnv:       field(command, "pat-name-env", values.patNameEnv, clears.patNameEnv),
				PATSecretEnv:     field(command, "pat-secret-env", values.patSecretEnv, clears.patSecretEnv),
				DefaultWorkspace: field(command, "default-workspace", values.defaultWorkspace, clears.defaultWorkspace),
			}
			result, err := deps.Updater.Update(command.Context(), profileupdate.Input{Alias: args[0], Patch: patch})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	addProfileFlags(command, &values.serverURL, &values.site, &values.apiVersion, &values.patNameEnv, &values.patSecretEnv, &values.defaultWorkspace)
	command.Flags().BoolVar(&clears.site, "clear-site", false, "clear the site content URL")
	command.Flags().BoolVar(&clears.apiVersion, "clear-api-version", false, "restore the default API version")
	command.Flags().BoolVar(&clears.patNameEnv, "clear-pat-name-env", false, "restore the conventional PAT name variable")
	command.Flags().BoolVar(&clears.patSecretEnv, "clear-pat-secret-env", false, "restore the conventional PAT secret variable")
	command.Flags().BoolVar(&clears.defaultWorkspace, "clear-default-workspace", false, "clear the environment workspace default")
	return command
}

func newRemove(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: use(deps, "env.profile.remove", "remove <alias>"), Short: short(deps, "env.profile.remove", "Remove an environment profile."),
		Annotations: map[string]string{"tadx.capability": "env.profile.remove"}, Args: exactAlias("env.profile.remove"),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := deps.Remover.Remove(command.Context(), profileremove.Input{Alias: args[0]})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
}

func newDefault(deps Dependencies) *cobra.Command {
	return &cobra.Command{
		Use: use(deps, "env.profile.set-default", "default <alias>"), Short: short(deps, "env.profile.set-default", "Set the default environment."),
		Annotations: map[string]string{"tadx.capability": "env.profile.set-default"}, Args: exactAlias("env.profile.set-default"),
		RunE: func(command *cobra.Command, args []string) error {
			result, err := deps.DefaultSetter.SetDefault(command.Context(), profilesetdefault.Input{Alias: args[0]})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
}

type profileValues struct{ serverURL, site, apiVersion, patNameEnv, patSecretEnv, defaultWorkspace string }
type profileClears struct{ site, apiVersion, patNameEnv, patSecretEnv, defaultWorkspace bool }

func addProfileFlags(command *cobra.Command, serverURL, site, apiVersion, patNameEnv, patSecretEnv, defaultWorkspace *string) {
	command.Flags().StringVar(serverURL, "url", "", "Tableau server HTTPS URL")
	command.Flags().StringVar(site, "site", "", "Tableau site content URL")
	command.Flags().StringVar(apiVersion, "api-version", "", "Tableau REST API version")
	command.Flags().StringVar(patNameEnv, "pat-name-env", "", "PAT name environment-variable reference")
	command.Flags().StringVar(patSecretEnv, "pat-secret-env", "", "PAT secret environment-variable reference")
	command.Flags().StringVar(defaultWorkspace, "default-workspace", "", "logical default workspace name")
}

func field(command *cobra.Command, flag, value string, clear bool) profileupdate.StringField {
	return profileupdate.StringField{Set: command.Flags().Changed(flag) || clear, Value: value}
}

func noArgs(operation string) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := cobra.NoArgs(command, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}

func exactAlias(operation string) cobra.PositionalArgs {
	return func(command *cobra.Command, args []string) error {
		if err := cobra.ExactArgs(1)(command, args); err != nil {
			return clierr.Usage(operation, err)
		}
		return nil
	}
}

func use(deps Dependencies, id, fallback string) string {
	registered := deps.Uses[id]
	if registered == "" {
		return fallback
	}
	if _, arguments, found := strings.Cut(fallback, " "); found && !strings.Contains(registered, " ") {
		return registered + " " + arguments
	}
	return registered
}
func short(deps Dependencies, id, fallback string) string {
	if deps.Shorts[id] != "" {
		return deps.Shorts[id]
	}
	return fallback
}

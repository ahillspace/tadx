// Package auth contains thin Cobra plumbing for authentication commands.
package auth

import (
	"context"
	"errors"
	"strings"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authlogin "github.com/ahillspace/tadx/actions/auth/login"
	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
)

// Checker executes auth.check.
type Checker interface {
	Execute(context.Context, authcheck.Input) (authcheck.Output, error)
}

// Statuser inspects local PAT-reference readiness without contacting Tableau.
type Statuser interface {
	Execute(context.Context, authstatus.Input) (authstatus.Output, error)
}

// Login validates and stores one PAT supplied through an interactive terminal.
type Login interface {
	Execute(context.Context, authlogin.Input) (authlogin.Output, error)
}

// Logout removes one TADX-stored PAT without revoking it in Tableau.
type Logout interface {
	Execute(context.Context, authlogout.Input) (authlogout.Output, error)
}

// Prompter provides terminal-only credential input. Implementations must echo
// the PAT name and suppress echo while reading the PAT secret.
type Prompter interface {
	IsTerminal() bool
	ReadPATName(context.Context) (string, error)
	ReadPATSecret(context.Context) (string, error)
}

// Renderer writes one structured result.
type Renderer interface{ Render(any) error }

// Dependencies contains auth command wiring.
type Dependencies struct {
	Checker     Checker
	Statuser    Statuser
	Login       Login
	Logout      Logout
	Prompter    Prompter
	Renderer    Renderer
	Use         string
	Short       string
	StatusUse   string
	StatusShort string
	LoginUse    string
	LoginShort  string
	LogoutUse   string
	LogoutShort string
}

// New creates the auth domain.
func New(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "auth", Short: "Manage and inspect Tableau authentication"}
	use := deps.Use
	if use == "" {
		use = "check"
	}
	short := deps.Short
	if short == "" {
		short = "Verify PAT authentication."
	}
	var environment string
	check := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "auth.check"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("auth.check", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Checker.Execute(command.Context(), authcheck.Input{Environment: environment})
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.Renderer.Render(result)
		},
	}
	check.Flags().StringVar(&environment, "environment", "", "exact environment alias; defaults to configured read environment")
	command.AddCommand(check)
	if deps.Statuser != nil {
		statusUse := deps.StatusUse
		if statusUse == "" {
			statusUse = "status"
		}
		statusShort := deps.StatusShort
		if statusShort == "" {
			statusShort = "Report local authentication readiness."
		}
		var statusEnvironment string
		status := &cobra.Command{
			Use: statusUse, Short: statusShort, Annotations: map[string]string{"tadx.capability": "auth.status"},
			Args: func(command *cobra.Command, args []string) error {
				if err := cobra.NoArgs(command, args); err != nil {
					return clierr.Usage("auth.status", err)
				}
				return nil
			},
			RunE: func(command *cobra.Command, _ []string) error {
				result, err := deps.Statuser.Execute(command.Context(), authstatus.Input{Environment: statusEnvironment})
				if err != nil {
					return err
				}
				return deps.Renderer.Render(result)
			},
		}
		status.Flags().StringVar(&statusEnvironment, "environment", "", "exact environment alias; defaults to configured read environment")
		command.AddCommand(status)
	}
	if deps.Login != nil {
		command.AddCommand(newLogin(deps))
	}
	if deps.Logout != nil {
		command.AddCommand(newLogout(deps))
	}
	return command
}

func newLogin(deps Dependencies) *cobra.Command {
	use := deps.LoginUse
	if use == "" {
		use = "login"
	}
	short := deps.LoginShort
	if short == "" {
		short = "Validate and store a PAT in the OS credential store."
	}
	var environment string
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "auth.login"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("auth.login", err)
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			if strings.TrimSpace(environment) == "" {
				return clierr.Usage("auth.login", errors.New("--environment is required"))
			}
			if preflight, ok := deps.Login.(interface {
				Preflight(context.Context, string) error
			}); ok {
				if err := preflight.Preflight(command.Context(), environment); err != nil {
					return err
				}
			}
			if deps.Prompter == nil {
				return promptError(errors.New("interactive credential input is not configured"))
			}
			if !deps.Prompter.IsTerminal() {
				return clierr.Usage("auth.login", errors.New("auth login requires an interactive terminal; use env add/update --pat-name-env and --pat-secret-env for automation (variable names, not secrets)"))
			}
			name, err := deps.Prompter.ReadPATName(command.Context())
			if err != nil {
				return promptError(err)
			}
			secret, err := deps.Prompter.ReadPATSecret(command.Context())
			if err != nil {
				return promptError(err)
			}
			result, err := deps.Login.Execute(command.Context(), authlogin.Input{Environment: environment, PATName: name, PATSecret: secret})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&environment, "environment", "", "exact environment alias")
	return command
}

func newLogout(deps Dependencies) *cobra.Command {
	var preview bool
	use := deps.LogoutUse
	if use == "" {
		use = "logout"
	}
	short := deps.LogoutShort
	if short == "" {
		short = "Remove a PAT from the OS credential store."
	}
	var environment string
	command := &cobra.Command{
		Use: use, Short: short, Annotations: map[string]string{"tadx.capability": "auth.logout"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("auth.logout", err)
			}
			if strings.TrimSpace(environment) == "" {
				return clierr.Usage("auth.logout", errors.New("--environment is required"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.Logout.Execute(command.Context(), authlogout.Input{Environment: environment, Preview: preview})
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		},
	}
	command.Flags().StringVar(&environment, "environment", "", "exact environment alias")
	command.Flags().BoolVar(&preview, "preview", false, "show configured local credential removal without reading or deleting the stored PAT")
	return command
}

func promptError(cause error) error {
	return &errs.Error{
		ID: "auth.login.input", Kind: errs.KindRuntime, Operation: "auth.login",
		Summary: "PAT input failed.", Cause: cause, Retryable: errs.Bool(false),
		CorrectiveAction: "Run auth login from an interactive terminal. No credential was saved.",
	}
}

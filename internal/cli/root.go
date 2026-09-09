// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"
	"fmt"
	"reflect"
	"strings"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	searchaction "github.com/ahillspace/tadx/actions/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	doctorcli "github.com/ahillspace/tadx/internal/cli/doctor"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// CapabilityAnnotation associates an executable command with its registry ID.
const CapabilityAnnotation = "tadx.capability"

const groupingAnnotation = "tadx.grouping"
const aliasAnnotation = "tadx.alias"

// RegisteredCommand describes a capability discovered from the actual Cobra tree.
type RegisteredCommand struct {
	CapabilityID string
	CommandPath  []string
}

// MutationPolicy identifies registry-defined remote mutation capabilities.
type MutationPolicy interface {
	IsRemoteMutation(string) bool
}

// Lister executes capability list.
type Lister interface {
	Execute(context.Context, capabilitylist.Input) (capabilitylist.Output, error)
}

// Getter executes capability get.
type Getter interface {
	Execute(context.Context, capabilityget.Input) (capabilityget.Output, error)
}

type AuthChecker interface {
	Execute(context.Context, authcheck.Input) (authcheck.Output, error)
}
type AuthStatuser interface {
	Execute(context.Context, authstatus.Input) (authstatus.Output, error)
}
type Searcher interface {
	Execute(context.Context, searchaction.Input) (searchaction.Output, error)
}
type WorkbookPuller interface {
	Execute(context.Context, workbookpull.Input) (workbookpull.Output, error)
}
type WorkbookPublisher interface {
	Execute(context.Context, workbookpublish.Input, bool) (workbookpublish.Output, error)
}

// Renderer writes structured command output.
type Renderer interface {
	Render(any) error
}

// RenderOptions contains presentation-only flags shared by every command.
type RenderOptions struct {
	Full bool
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Lister               Lister
	Getter               Getter
	Renderer             Renderer
	RenderOptions        *RenderOptions
	ConfigPath           *string
	MutationsEnabled     bool
	MutationPolicy       MutationPolicy
	ListUse              string
	ListShort            string
	GetUse               string
	GetShort             string
	AuthChecker          AuthChecker
	Searcher             Searcher
	CatalogRefresher     catalogcli.Refresher
	CatalogStatuser      catalogcli.Statuser
	WorkbookPuller       WorkbookPuller
	WorkbookPublisher    WorkbookPublisher
	Content              *contentcli.Dependencies
	EnvironmentProfiles  *envcli.Dependencies
	Workspaces           *workspacecli.Dependencies
	Admin                *admincli.Dependencies
	Agent                *agentcli.Dependencies
	Pulse                *pulsecli.Dependencies
	Version              *versioncli.Dependencies
	DoctorRunner         doctorcli.Runner
	DoctorUse            string
	DoctorShort          string
	AuthUse              string
	AuthShort            string
	AuthStatuser         AuthStatuser
	AuthLogin            authcli.Login
	AuthLogout           authcli.Logout
	AuthPrompter         authcli.Prompter
	AuthStatusUse        string
	AuthStatusShort      string
	AuthLoginUse         string
	AuthLoginShort       string
	AuthLogoutUse        string
	AuthLogoutShort      string
	CatalogRefreshUse    string
	CatalogRefreshShort  string
	CatalogStatusUse     string
	CatalogStatusShort   string
	WorkbookPullUse      string
	WorkbookPullShort    string
	WorkbookPublishUse   string
	WorkbookPublishShort string
}

// NewRoot creates the root command. It contains no domain behavior.
func NewRoot(deps Dependencies) *cobra.Command {
	renderOptions := deps.RenderOptions
	if renderOptions == nil {
		renderOptions = &RenderOptions{}
	}
	configPath := deps.ConfigPath
	if configPath == nil {
		configPath = new(string)
	}
	root := &cobra.Command{
		Use:   "tadx",
		Short: "Deterministic Tableau lifecycle and development CLI",
		Long: `Deterministic Tableau lifecycle and development CLI.

Run tadx capability list to discover available operations and tadx capability get <id> for bounded details.
TADX returns compact TOON by default. Use --full to show expanded bounded details for the same operation.
Read commands query Tableau by default. Pass --catalog on supported reads to use local catalog data without contacting Tableau.
Use --env as a short alias for --environment on commands that select an environment.

Remote mutation commands remain visible when execution is disabled. Set TADX_ENABLE_MUTATIONS=1 to enable them.
When enabled, mutation commands perform changes by default. Pass --preview to inspect the plan without performing the mutation.

TADX handles lifecycle operations, not datasource value queries, view rendering, or current Pulse values and insights.
Other connected tools remain independent; TADX does not configure, select, proxy, or report their connections.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.SetGlobalNormalizationFunc(func(_ *pflag.FlagSet, name string) pflag.NormalizedName {
		if name == "env" {
			name = "environment"
		}
		return pflag.NormalizedName(name)
	})
	root.PersistentFlags().BoolVar(&renderOptions.Full, "full", false, "show expanded bounded details")
	root.PersistentFlags().StringVar(configPath, "config", *configPath, "path to the non-secret TADX configuration file")
	root.PersistentFlags().Lookup("config").DefValue = ""
	root.AddCommand(capabilitycli.New(capabilitycli.Dependencies{
		Lister:           deps.Lister,
		Getter:           deps.Getter,
		Renderer:         deps.Renderer,
		MutationsEnabled: deps.MutationsEnabled,
		ListUse:          deps.ListUse,
		ListShort:        deps.ListShort,
		GetUse:           deps.GetUse,
		GetShort:         deps.GetShort,
	}))
	if deps.EnvironmentProfiles != nil {
		environmentProfiles := *deps.EnvironmentProfiles
		environmentProfiles.Renderer = deps.Renderer
		root.AddCommand(envcli.New(environmentProfiles))
	}
	if deps.Workspaces != nil {
		workspaces := *deps.Workspaces
		workspaces.Renderer = deps.Renderer
		root.AddCommand(workspacecli.New(workspaces))
	}
	if deps.Admin != nil {
		admin := *deps.Admin
		admin.Renderer = deps.Renderer
		admin.MutationsEnabled = deps.MutationsEnabled
		root.AddCommand(admincli.New(admin))
	}
	if deps.Agent != nil {
		agent := *deps.Agent
		agent.Renderer = deps.Renderer
		root.AddCommand(agentcli.New(agent))
	}
	if deps.Pulse != nil {
		pulse := *deps.Pulse
		pulse.Renderer = deps.Renderer
		root.AddCommand(pulsecli.New(pulse))
	}
	if deps.Version != nil {
		version := *deps.Version
		version.Renderer = deps.Renderer
		root.AddCommand(versioncli.New(version))
	}
	if deps.DoctorRunner != nil {
		root.AddCommand(doctorcli.New(doctorcli.Dependencies{Runner: deps.DoctorRunner, Renderer: deps.Renderer, Use: deps.DoctorUse, Short: deps.DoctorShort}))
	}
	if deps.AuthChecker != nil {
		root.AddCommand(authcli.New(authcli.Dependencies{
			Checker: deps.AuthChecker, Statuser: deps.AuthStatuser, Login: deps.AuthLogin, Logout: deps.AuthLogout,
			Prompter: deps.AuthPrompter, Renderer: deps.Renderer,
			Use: deps.AuthUse, Short: deps.AuthShort, StatusUse: deps.AuthStatusUse, StatusShort: deps.AuthStatusShort,
			LoginUse: deps.AuthLoginUse, LoginShort: deps.AuthLoginShort, LogoutUse: deps.AuthLogoutUse, LogoutShort: deps.AuthLogoutShort,
		}))
	}
	if deps.Searcher != nil {
		root.AddCommand(newSearch(deps.Searcher, deps.Renderer))
	}
	if deps.CatalogRefresher != nil || deps.CatalogStatuser != nil {
		root.AddCommand(catalogcli.New(catalogcli.Dependencies{
			Refresher: deps.CatalogRefresher, Statuser: deps.CatalogStatuser,
			Renderer:   deps.Renderer,
			RefreshUse: deps.CatalogRefreshUse, RefreshShort: deps.CatalogRefreshShort,
			StatusUse: deps.CatalogStatusUse, StatusShort: deps.CatalogStatusShort,
		}))
	}
	if deps.WorkbookPuller != nil && deps.WorkbookPublisher != nil {
		contentDependencies := contentcli.Dependencies{}
		if deps.Content != nil {
			contentDependencies = *deps.Content
		}
		contentDependencies.Puller = deps.WorkbookPuller
		contentDependencies.Publisher = deps.WorkbookPublisher
		contentDependencies.Renderer = deps.Renderer
		contentDependencies.MutationsEnabled = deps.MutationsEnabled
		contentDependencies.PullUse = deps.WorkbookPullUse
		contentDependencies.PullShort = deps.WorkbookPullShort
		contentDependencies.PublishUse = deps.WorkbookPublishUse
		contentDependencies.PublishShort = deps.WorkbookPublishShort
		root.AddCommand(contentcli.New(contentDependencies))
	}
	root.AddCommand(NewCompletion(root))
	rejectGroupingArguments(root)
	applyMutationExecutionPolicy(root, deps.MutationPolicy, deps.MutationsEnabled)
	root.CompletionOptions.DisableDefaultCmd = true
	setFlagErrorHandlers(root)
	return root
}

func rejectGroupingArguments(root *cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if command.Args == nil && command.HasSubCommands() && command.Run == nil && command.RunE == nil {
			operation := strings.TrimPrefix(command.CommandPath(), root.Name()+" ")
			if command.Annotations == nil {
				command.Annotations = map[string]string{}
			}
			command.Annotations[groupingAnnotation] = "true"
			command.Args = func(command *cobra.Command, args []string) error {
				if err := cobra.NoArgs(command, args); err != nil {
					return clierr.Usage(operation, err)
				}
				return nil
			}
			command.RunE = func(command *cobra.Command, _ []string) error {
				return command.Help()
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func applyMutationExecutionPolicy(root *cobra.Command, policy MutationPolicy, enabled bool) {
	if root == nil {
		return
	}
	policyMissing := mutationPolicyMissing(policy)
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		runnable := command.RunE != nil || command.Run != nil
		if runnable && command.Annotations[CapabilityAnnotation] != "" && policyMissing {
			command.Run = nil
			command.RunE = func(*cobra.Command, []string) error {
				return &errs.Error{
					ID:               "mutation.policy.unconfigured",
					Kind:             errs.KindRuntime,
					Operation:        "startup",
					Summary:          "Remote mutation policy is not configured.",
					Retryable:        errs.Bool(false),
					CorrectiveAction: "Configure the registry-backed mutation policy before running TADX.",
				}
			}
		}
		if capabilityID := command.Annotations[CapabilityAnnotation]; runnable && capabilityID != "" && !policyMissing && policy.IsRemoteMutation(capabilityID) {
			command.Hidden = false
			originalRunE := command.RunE
			originalRun := command.Run
			command.Run = nil
			command.RunE = func(command *cobra.Command, args []string) error {
				if !enabled {
					return &errs.Error{
						ID:               "mutation.disabled",
						Kind:             errs.KindOperation,
						Operation:        capabilityID,
						Summary:          "Remote mutation execution is disabled.",
						Retryable:        errs.Bool(false),
						CorrectiveAction: "Set TADX_ENABLE_MUTATIONS=1, then retry.",
					}
				}
				if originalRunE != nil {
					return originalRunE(command, args)
				}
				originalRun(command, args)
				return nil
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
}

func mutationPolicyMissing(policy MutationPolicy) bool {
	if policy == nil {
		return true
	}
	value := reflect.ValueOf(policy)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

// RegisteredCommands discovers capability bindings from the actual command tree.
func RegisteredCommands(root *cobra.Command) ([]RegisteredCommand, error) {
	var registrations []RegisteredCommand
	var walk func(*cobra.Command) error
	walk = func(command *cobra.Command) error {
		if command.Annotations[aliasAnnotation] == "true" {
			return nil
		}
		id := command.Annotations[CapabilityAnnotation]
		runnable := command.Run != nil || command.RunE != nil
		if runnable && id == "" && command.Annotations[groupingAnnotation] != "true" {
			return fmt.Errorf("runnable command %q has no capability annotation", command.CommandPath())
		}
		if !runnable && id != "" {
			return fmt.Errorf("non-runnable command %q has capability annotation %q", command.CommandPath(), id)
		}
		if id != "" {
			path := strings.Fields(command.CommandPath())
			if len(path) > 0 {
				path = path[1:]
			}
			registrations = append(registrations, RegisteredCommand{CapabilityID: id, CommandPath: path})
		}
		for _, child := range command.Commands() {
			if err := walk(child); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(root); err != nil {
		return nil, err
	}
	return registrations, nil
}

func setFlagErrorHandlers(command *cobra.Command) {
	originalArgs := command.Args
	command.Args = func(current *cobra.Command, args []string) error {
		if originalArgs != nil {
			if err := originalArgs(current, args); err != nil {
				return err
			}
		}
		if err := current.ValidateRequiredFlags(); err != nil {
			return clierr.Usage(current.CommandPath(), err)
		}
		if err := current.ValidateFlagGroups(); err != nil {
			return clierr.Usage(current.CommandPath(), err)
		}
		return nil
	}
	if cursor := command.Flags().Lookup("cursor"); cursor != nil {
		cursor.Hidden = true
	}
	command.SetFlagErrorFunc(func(command *cobra.Command, cause error) error {
		advice := "Run " + command.CommandPath() + " --help for supported flags."
		switch cause.Error() {
		case "unknown flag: --site":
			if command.Flags().Lookup("environment") != nil {
				advice = "Use --environment <alias> (or --env <alias>) to select a configured environment."
			}
		case "unknown flag: --terms":
			if command.Annotations[CapabilityAnnotation] == "search.run" {
				advice = `Pass the search term as a positional argument: tadx search "<term>" --env <alias>.`
			}
		}
		return &errs.Error{Kind: errs.KindUsage, Operation: command.CommandPath(), Summary: cause.Error(), Cause: cause, CorrectiveAction: advice}
	})
	for _, child := range command.Commands() {
		setFlagErrorHandlers(child)
	}
}

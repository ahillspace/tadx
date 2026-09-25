// Package cli contains thin Cobra plumbing for TADX.
package cli

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	searchaction "github.com/ahillspace/tadx/actions/search"
	sessionoverview "github.com/ahillspace/tadx/actions/session/overview"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/batchspec"
	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	cachecli "github.com/ahillspace/tadx/internal/cli/cache"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	doctorcli "github.com/ahillspace/tadx/internal/cli/doctor"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	jobcli "github.com/ahillspace/tadx/internal/cli/job"
	lastcli "github.com/ahillspace/tadx/internal/cli/last"
	mutationcli "github.com/ahillspace/tadx/internal/cli/mutation"
	policycli "github.com/ahillspace/tadx/internal/cli/policy"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	updatecli "github.com/ahillspace/tadx/internal/cli/update"
	versioncli "github.com/ahillspace/tadx/internal/cli/version"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
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

type SessionOverview interface {
	Execute(context.Context) (sessionoverview.Output, error)
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
	JSON bool
	// HintConfig returns explicitly selected non-secret configuration context.
	HintConfig func() string
}

// Dependencies contains the explicitly wired Phase 0 command dependencies.
type Dependencies struct {
	Policy                *policycli.Dependencies
	SessionOverview       SessionOverview
	Update                *updatecli.Dependencies
	Catalog               *catalogcli.Dependencies
	ContentLabels         *contentcli.LabelDependencies
	AdminLabels           *admincli.LabelDependencies
	BatchSelectors        map[string]string
	BatchOptions          map[string]batchspec.Options
	Lister                Lister
	Getter                Getter
	Renderer              Renderer
	RenderOptions         *RenderOptions
	ConfigPath            *string
	MutationsEnabled      bool
	MutationPolicy        MutationPolicy
	ResolveWriteTarget    func(string) (string, error)
	ResolveMutationPolicy func(string) (bool, string, error)
	MutationStatus        mutationcli.Status
	MutationSetter        mutationcli.Setter
	LastReader            lastcli.Reader
	Jobs                  *jobcli.Dependencies
	ListUse               string
	ListShort             string
	GetUse                string
	GetShort              string
	AuthChecker           AuthChecker
	Searcher              Searcher
	CacheRefresher        cachecli.Refresher
	CacheStatuser         cachecli.Statuser
	WorkbookPuller        WorkbookPuller
	WorkbookPublisher     WorkbookPublisher
	Content               *contentcli.Dependencies
	EnvironmentProfiles   *envcli.Dependencies
	Workspaces            *workspacecli.Dependencies
	Admin                 *admincli.Dependencies
	Agent                 *agentcli.Dependencies
	Pulse                 *pulsecli.Dependencies
	Version               *versioncli.Dependencies
	DoctorRunner          doctorcli.Runner
	DoctorUse             string
	DoctorShort           string
	AuthUse               string
	AuthShort             string
	AuthStatuser          AuthStatuser
	AuthLogin             authcli.Login
	AuthLogout            authcli.Logout
	AuthPrompter          authcli.Prompter
	AuthStatusUse         string
	AuthStatusShort       string
	AuthLoginUse          string
	AuthLoginShort        string
	AuthLogoutUse         string
	AuthLogoutShort       string
	CacheRefreshUse       string
	CacheRefreshShort     string
	CacheStatusUse        string
	CacheStatusShort      string
	WorkbookPullUse       string
	WorkbookPullShort     string
	WorkbookPublishUse    string
	WorkbookPublishShort  string
}

// NewRoot creates the root command. It contains no domain behavior.
func NewRoot(deps Dependencies) *cobra.Command {
	return newRoot(deps, true)
}

// RootVersionRequested reports whether args select the root-local installed
// version alias, without treating a value of another flag as the alias.
func RootVersionRequested(args []string) bool {
	found := false
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" || !strings.HasPrefix(arg, "-") {
			return false
		}
		if arg == "--version" || arg == "-v" {
			found = true
			continue
		}
		if strings.HasPrefix(arg, "--version=") || strings.HasPrefix(arg, "-v=") {
			found = strings.TrimPrefix(strings.TrimPrefix(arg, "--version="), "-v=") == "true"
			continue
		}
		if arg == "--config" && index+1 < len(args) {
			index++
		}
	}
	return found
}

func newRoot(deps Dependencies, withBatches bool) *cobra.Command {
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

Run tadx without arguments for a local, read-only overview of environments, credential configuration, workspaces, and mutation policy.
Run tadx update to refresh the CLI and bundled agent Guidance together; --check only checks the release.
Use resource help, such as tadx admin group --help, for that resource's commands, flags, and examples; direct actions use shorter paths.
Use tadx capability list and tadx capability get for inventory and availability diagnostics, not command syntax.
TADX returns compact TOON by default. Use --full to show expanded bounded details for the same operation.
Run tadx --version for installed version information only; it does not check releases, update, authenticate, or contact Tableau.
Read commands query Tableau by default. Pass --cache on supported reads to use local cache data without contacting Tableau.
Use --env as a short alias for --environment on commands that select an environment.

Remote mutation commands remain visible when execution is disabled. Use --preview for a read-only plan without enabling mutations.
Run tadx mutation status --environment <alias> to inspect site consent.
With permission, tadx mutation set --environment <alias> --enabled=true saves consent for that site in future sessions.
Each Tableau server and exact site content URL has separate consent. Environment aliases for the same site share consent.
When enabled, mutation commands perform changes by default. Pass --preview to inspect the plan without performing the mutation.

TADX handles lifecycle operations, not datasource value queries, view rendering, or current Pulse values and insights.
Other connected tools remain independent; TADX does not configure, select, proxy, or report their connections.`,
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	var versionCommand *cobra.Command
	var showVersion bool
	if deps.SessionOverview != nil {
		root.Annotations = map[string]string{CapabilityAnnotation: "session.overview"}
		root.Args = cobra.NoArgs
		root.RunE = func(cmd *cobra.Command, _ []string) error {
			result, err := deps.SessionOverview.Execute(cmd.Context())
			if err != nil {
				return err
			}
			return deps.Renderer.Render(result)
		}
	}
	if deps.Update != nil {
		updateDependencies := *deps.Update
		updateDependencies.Renderer = deps.Renderer
		root.AddCommand(updatecli.New(updateDependencies))
	}
	root.PersistentFlags().BoolVar(&renderOptions.Full, "full", false, "show expanded bounded details")
	root.PersistentFlags().BoolVar(&renderOptions.JSON, "json", renderOptions.JSON, "render machine-readable JSON instead of TOON")
	// Early error rendering may seed JSON=true, but the public CLI default is false.
	root.PersistentFlags().Lookup("json").DefValue = "false"
	root.PersistentFlags().StringVar(configPath, "config", *configPath, "path to the non-secret TADX configuration file")
	root.PersistentFlags().Lookup("config").DefValue = ""
	priorHintConfig := renderOptions.HintConfig
	renderOptions.HintConfig = func() string {
		if root.PersistentFlags().Changed("config") {
			return *configPath
		}
		if priorHintConfig != nil {
			return priorHintConfig()
		}
		return ""
	}
	if deps.LastReader != nil {
		root.AddCommand(lastcli.New(deps.LastReader, deps.Renderer))
	}
	if deps.Policy != nil {
		policyDependencies := *deps.Policy
		policyDependencies.Renderer = deps.Renderer
		root.AddCommand(policycli.New(policyDependencies))
	}
	if deps.Jobs != nil {
		jobs := *deps.Jobs
		jobs.Renderer = deps.Renderer
		root.AddCommand(jobcli.New(jobs))
	}
	if deps.MutationStatus != nil && deps.MutationSetter != nil {
		root.AddCommand(mutationcli.New(deps.MutationStatus, deps.MutationSetter, deps.Renderer))
	}
	root.AddCommand(capabilitycli.New(capabilitycli.Dependencies{
		Lister:                deps.Lister,
		Getter:                deps.Getter,
		Renderer:              deps.Renderer,
		MutationsEnabled:      deps.MutationsEnabled,
		ResolveMutationPolicy: deps.ResolveMutationPolicy,
		ListUse:               deps.ListUse,
		ListShort:             deps.ListShort,
		GetUse:                deps.GetUse,
		GetShort:              deps.GetShort,
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
		node := admincli.New(admin)
		if deps.AdminLabels != nil {
			labels := *deps.AdminLabels
			labels.Renderer = deps.Renderer
			node.AddCommand(admincli.NewLabels(labels)...)
		}
		root.AddCommand(node)
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
		versionCommand = versioncli.New(version)
		root.AddCommand(versionCommand)
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
	if deps.CacheRefresher != nil || deps.CacheStatuser != nil {
		root.AddCommand(cachecli.New(cachecli.Dependencies{
			Refresher: deps.CacheRefresher, Statuser: deps.CacheStatuser,
			Renderer:   deps.Renderer,
			RefreshUse: deps.CacheRefreshUse, RefreshShort: deps.CacheRefreshShort,
			StatusUse: deps.CacheStatusUse, StatusShort: deps.CacheStatusShort,
		}))
	}
	if deps.Catalog != nil || deps.ContentLabels != nil || (deps.Content != nil && deps.Content.LineagePuller != nil) {
		node := catalogcli.NewGroup()
		if deps.Catalog != nil {
			catalog := *deps.Catalog
			catalog.Renderer = deps.Renderer
			node = catalogcli.New(catalog)
		}
		if deps.Content != nil && deps.Content.LineagePuller != nil {
			node.AddCommand(contentcli.NewLineage(deps.Content.LineagePuller, deps.Renderer))
		}
		if deps.ContentLabels != nil {
			labels := *deps.ContentLabels
			labels.Renderer = deps.Renderer
			node.AddCommand(contentcli.NewLabels(labels))
		}
		root.AddCommand(node)
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
	if versionCommand != nil {
		root.Flags().BoolVar(&showVersion, "version", false, "show the installed version without release checks")
		originalRunE := root.RunE
		root.RunE = func(command *cobra.Command, args []string) error {
			if showVersion {
				return versionCommand.RunE(command, nil)
			}
			if originalRunE != nil {
				return originalRunE(command, args)
			}
			return command.Help()
		}
	}
	rejectGroupingArguments(root)
	if withBatches {
		enableBatches(root, deps)
	}
	applyMutationExecutionPolicy(root, deps.MutationPolicy, deps.MutationsEnabled, deps.ResolveMutationPolicy)
	root.CompletionOptions.DisableDefaultCmd = true
	setFlagErrorHandlers(root)
	applyWriteTargetResolution(root, deps)
	applyShorthand(root)
	applyHelpExamples(root)
	applyHelpValues(root)
	installCategoryHelp(root)
	return root
}

// Resolve before command argument and required-flag validation, using application policy.
func applyWriteTargetResolution(root *cobra.Command, deps Dependencies) {
	if deps.ResolveWriteTarget == nil || mutationPolicyMissing(deps.MutationPolicy) {
		return
	}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		id := command.Annotations[CapabilityAnnotation]
		if id != "" && (deps.MutationPolicy.IsRemoteMutation(id) || id == "auth.login" || id == "auth.logout") && command.Flags().Lookup("environment") != nil {
			original := command.Args
			command.Args = func(cmd *cobra.Command, args []string) error {
				alias, _ := cmd.Flags().GetString("environment")
				if alias == "" {
					resolved, err := deps.ResolveWriteTarget(alias)
					if err != nil {
						var structured *errs.Error
						if errors.As(err, &structured) {
							return err
						}
						return &errs.Error{ID: "target.environment_required", Kind: errs.KindUsage, Operation: id, Summary: err.Error(), Retryable: errs.Bool(false)}
					}
					if err := cmd.Flags().Set("environment", resolved); err != nil {
						return err
					}
				}
				if original != nil {
					return original(cmd, args)
				}
				return nil
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
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

func applyMutationExecutionPolicy(root *cobra.Command, policy MutationPolicy, enabled bool, resolver ...func(string) (bool, string, error)) {
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
				effective := enabled
				if !explicitMutationPreview(command) && len(resolver) > 0 && resolver[0] != nil {
					var err error
					alias, _ := command.Flags().GetString("environment")
					effective, _, err = resolver[0](alias)
					if err != nil {
						return err
					}
				}
				if !effective && !explicitMutationPreview(command) {
					alias, _ := command.Flags().GetString("environment")
					return &errs.Error{
						ID:               "mutation.disabled",
						Kind:             errs.KindOperation,
						Operation:        capabilityID,
						Environment:      alias,
						Phase:            errs.PhaseSetup,
						Outcome:          errs.OutcomeNotAttempted,
						Summary:          "Remote mutation execution is disabled.",
						Retryable:        errs.Bool(false),
						CorrectiveAction: "Use --preview for a read-only plan where supported. Run " + commandhint.Environment(alias, "mutation", "status") + " to inspect site consent; obtain permission before changing that site's saved setting.",
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

// Only the executable command's own parsed Boolean preview flag authorizes a plan.
func explicitMutationPreview(command *cobra.Command) bool {
	flag := command.LocalNonPersistentFlags().Lookup("preview")
	if flag == nil || !flag.Changed || flag.Value.Type() != "bool" {
		return false
	}
	preview, err := command.Flags().GetBool("preview")
	return err == nil && preview
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
			if command != root && len(path) > 0 {
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
		if limit := current.Flags().Lookup("limit"); limit != nil && limit.Changed && limit.Value.Type() == "int" {
			value, err := current.Flags().GetInt("limit")
			if err == nil && value == 0 {
				return withUsageRecovery(current, clierr.Usage(current.CommandPath(), errors.New("--limit must be greater than zero when supplied")))
			}
		}
		if originalArgs != nil {
			if err := originalArgs(current, args); err != nil {
				return withUsageRecovery(current, err)
			}
		}
		if err := current.ValidateRequiredFlags(); err != nil {
			return withUsageRecovery(current, clierr.Usage(current.CommandPath(), err))
		}
		if err := current.ValidateFlagGroups(); err != nil {
			return withUsageRecovery(current, clierr.Usage(current.CommandPath(), err))
		}
		return nil
	}
	if cursor := command.Flags().Lookup("cursor"); cursor != nil && command.CommandPath() != "tadx capability list" {
		cursor.Hidden = true
	}
	command.SetFlagErrorFunc(func(command *cobra.Command, cause error) error {
		return &errs.Error{Kind: errs.KindUsage, Operation: command.CommandPath(), Summary: cause.Error(), Cause: cause, CorrectiveAction: usageRecovery(command), Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
	})
	for _, child := range command.Commands() {
		setFlagErrorHandlers(child)
	}
}

// CommandUsageError gives command discovery failures the same recovery contract
// as parsed flag and argument errors.
func CommandUsageError(command *cobra.Command, cause error) error {
	if command == nil {
		return clierr.Usage("cli", cause)
	}
	return withUsageRecovery(command, clierr.Usage(command.CommandPath(), cause))
}

// usageRecovery derives bounded recovery from the actual command tree, not
// error-message matching. Positional syntax and sibling actions remain visible.
func usageRecovery(command *cobra.Command) string {
	advice := "Usage: " + strings.TrimPrefix(command.UseLine(), "tadx ") + "."
	if command.Flags().Lookup("environment") != nil {
		advice += " Use --environment <alias> (or --env <alias>) to select a configured environment."
	}
	if command.Example != "" {
		advice += " Example: " + strings.TrimSpace(strings.SplitN(command.Example, "\n", 2)[0]) + "."
		if !command.HasSubCommands() {
			return advice
		}
	}
	if command.HasSubCommands() {
		parent := command
		if command.Parent() != nil && command.Parent().Parent() != nil {
			parent = command.Parent()
		}
		var available []string
		for _, child := range parent.Commands() {
			if !child.Hidden && child.Name() != "help" {
				available = append(available, strings.TrimPrefix(child.CommandPath(), "tadx "))
				if len(available) == 16 {
					break
				}
			}
		}
		if len(available) > 0 {
			advice += " Available commands: " + strings.Join(available, ", ") + "."
		}
	}
	parts := strings.Fields(command.CommandPath())
	return advice + " Run " + commandhint.Command(append(parts[1:], "--help")...) + " for supported flags."
}

func withUsageRecovery(command *cobra.Command, err error) error {
	var structured *errs.Error
	if errors.As(err, &structured) && structured.Kind == errs.KindUsage {
		copy := *structured
		if copy.CorrectiveAction == "" {
			copy.CorrectiveAction = usageRecovery(command)
		}
		copy.Phase, copy.Outcome = errs.PhaseValidation, errs.OutcomeNotAttempted
		return &copy
	}
	return err
}

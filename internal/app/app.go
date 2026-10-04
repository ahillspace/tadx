// Package app is the TADX composition root.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	authops "github.com/ahillspace/tadx/actions/auth"
	cacheops "github.com/ahillspace/tadx/actions/cache"
	capabilityops "github.com/ahillspace/tadx/actions/capability"
	lastaction "github.com/ahillspace/tadx/actions/last"
	lineageops "github.com/ahillspace/tadx/actions/lineage"
	mutationops "github.com/ahillspace/tadx/actions/mutation"
	workbookops "github.com/ahillspace/tadx/actions/workbook"

	"github.com/ahillspace/tadx/internal/artifact"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/output"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	updater "github.com/ahillspace/tadx/internal/update"
	"github.com/ahillspace/tadx/internal/value"
)

// Options contains process-level discovery settings.
type Options struct {
	managedPolicy managedPolicySource
	ConfigPath    string
	HTTPClient    *http.Client
	PATStore      coreauth.PATStore
	AuthPrompter  authcli.Prompter
	Now           func() time.Time
	CorrelationID func() string
	UserHomeDir   func() (string, error)
	Stderr        io.Writer
	JobDirectory  string
	// PublicationWorkers enables detached workers for native publish and pull.
	// The executable enables it; embedders may run actions inline.
	PublicationWorkers   bool
	OperationDirectory   string
	WorkerLauncher       func(context.Context, string, string) error
	publicationExecution *operationrun.PublicationExecution
	publicationResult    func(any, bool, int) error
}

// Run wires and runs the CLI, renders structured output, and returns an AXI exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, options Options) (exitCode int) {
	if len(args) > 0 && args[0] == "__publication-worker" {
		if len(args) != 3 {
			return 2
		}
		return runPublicationWorker(context.Background(), args[1], args[2], options)
	}
	definitions := capability.All()
	renderOptions := &cli.RenderOptions{HintConfig: func() string { return options.ConfigPath }}
	renderOptions.JSON = hasJSONFlag(args)
	runtime, err := newRuntime(options)
	if err != nil {
		return renderError(stdout, err, renderOptions)
	}
	defer runtime.Close()
	mutationService := mutationops.New(func() string { return runtime.configPath }, func() bool {
		return runtime.managedPolicy != nil && errors.Is(runtime.managedPolicy.CheckRemoteMutation(), managedpolicy.ErrRemoteMutationDenied)
	})
	discovery := capabilityops.New(capabilityops.Ports{ManagedPolicy: runtime.managedPolicy, ResolveMutationPolicy: mutationService.Policy})
	capture := newLastCapture(runtime)
	capture.hintConfig = func() string { return hintConfigPath(renderOptions) }
	defer func() {
		if options.publicationResult != nil && capture.value != nil {
			if err := options.publicationResult(capture.value, capture.renderError, exitCode); err != nil {
				exitCode = 1
			}
		}
		var saveErr error
		if capture.enabled {
			capture.saved, saveErr = capture.recorder.Save(capture.operation, capture.value, exitCode)
		}
		if saveErr != nil {
			warningWriter := options.Stderr
			if warningWriter == nil {
				warningWriter = os.Stderr
			}
			_ = output.RenderWithOptions(warningWriter, output.LastResultWarning(), output.Options{JSON: renderOptions.JSON})
		}
		if capture.value != nil {
			renderer := writerRenderer{writer: stdout, options: renderOptions, saved: capture.saved}
			var renderErr error
			if capture.renderError {
				renderErr = output.RenderError(stdout, capture.value.(error), output.Options{Full: renderOptions.Full, JSON: renderOptions.JSON, ConfigPath: hintConfigPath(renderOptions), SavedResult: capture.saved})
			} else {
				renderErr = renderer.Render(capture.value)
			}
			if renderErr != nil {
				exitCode = 1
			}
		}
	}()
	fail := func(err error, opts *cli.RenderOptions) int {
		capture.value = err
		capture.renderError = true
		return errs.ExitCode(err)
	}
	workspaceCommands := newWorkspaceCommands(runtime)
	remoteContent := newRemoteContentCommands(runtime)
	remoteAdmin := newRemoteAdminCommands(runtime)
	pulseActions := newPulseCommands(runtime)
	credentialStore := authops.NewCredentialPersistence(runtime.configPath, runtime.patStore, processEnvironment{})
	cacheService := cacheops.New(cacheProvider{runtime: runtime})
	authService := authops.New(authops.Ports{
		CheckResolver: runtime, CheckAuthenticator: runtime,
		LoginResolver: authCredentialResolver{runtime: runtime}, LoginAuthenticator: loginAuthenticator{runtime: runtime}, LoginStore: credentialStore,
		LogoutResolver: credentialStore, LogoutStore: credentialStore,
		StatusResolver: authStatusResolver{runtime: runtime}, StatusLookup: processEnvironment{},
	})
	root := cli.NewRoot(cli.Dependencies{
		SessionOverview: newSessionService(runtime),
		Update: newUpdateCommand(updater.Runtime{CredentialVariables: func() []string {
			return config.ConfiguredPATVariables(runtime.configPath)
		}}),
		Catalog:               (&catalogCommands{runtime: runtime}).dependencies(),
		CatalogLabels:         contentLabelDependencies(runtime),
		CatalogLineage:        lineageops.New(lineageWorkspace{runtime: runtime}, lineageProvider{commands: remoteContent}),
		AdminLabels:           adminLabelDependencies(runtime),
		Lister:                discovery,
		Getter:                discovery,
		Renderer:              writerRenderer{writer: stdout, options: renderOptions, capture: capture},
		RenderOptions:         renderOptions,
		BatchSelectors:        capability.BatchSelectors(),
		BatchOptions:          capability.BatchOptions(),
		ConfigPath:            &runtime.configPath,
		MutationsEnabled:      false,
		MutationPolicy:        registryMutationPolicy{},
		Policy:                runtime.policyDependencies(),
		ResolveMutationPolicy: mutationService.Policy,
		MutationStatus:        mutationService,
		MutationSetter:        mutationService,
		LastReader: lastaction.New(func(ctx context.Context) (value.SavedExecution, error) {
			return capture.store().Read(ctx)
		}, runtime.checkManagedCapability),
		Jobs: newJobDependencies(runtime),
		ResolveWriteTarget: func(alias string) (string, error) {
			_, environment, err := runtime.environment(alias, true)
			var pathError *os.PathError
			if errors.As(err, &pathError) {
				return "", &errs.Error{ID: "target.configuration", Kind: errs.KindOperation, Operation: "target", Summary: "Environment configuration could not be read.", Cause: pathError.Err, Retryable: errs.Bool(false), CorrectiveAction: "Configure a Tableau environment, then retry."}
			}
			return environment.Alias, err
		},
		ListUse:             registryUse("capability.list"),
		ListShort:           registryShort("capability.list"),
		GetUse:              registryUse("capability.get"),
		GetShort:            registryShort("capability.get"),
		AuthChecker:         authService,
		Searcher:            newSearchCommands(runtime),
		CacheRefresher:      cacheService,
		CacheStatuser:       cacheService,
		WorkbookPuller:      workbookops.New(workbookops.Ports{Pull: workbookPullProvider{runtime: runtime}}),
		WorkbookPublisher:   workbookops.New(workbookops.Ports{Publish: workbookPublishProvider{runtime: runtime}}),
		Content:             remoteContent.dependencies(),
		EnvironmentProfiles: newEnvironmentDependencies(runtime),
		Workspaces:          workspaceCommands,
		Admin:               remoteAdmin.dependencies(),
		Agent:               newAgentCommands(runtime),
		Pulse:               pulseActions.dependencies(),
		Version:             newVersionCommand(runtime),
		DoctorRunner:        newDoctorAction(runtime),
		DoctorUse:           registryLeafUse("doctor.run"),
		DoctorShort:         registryShort("doctor.run"),
		AuthUse:             registryLeafUse("auth.check"), AuthShort: registryShort("auth.check"),
		AuthStatuser: authService, AuthStatusUse: registryLeafUse("auth.status"), AuthStatusShort: registryShort("auth.status"),
		AuthLogin:    authService,
		AuthLogout:   authService,
		AuthPrompter: runtime.authPrompter,
		AuthLoginUse: registryLeafUse("auth.login"), AuthLoginShort: registryShort("auth.login"),
		AuthLogoutUse: registryLeafUse("auth.logout"), AuthLogoutShort: registryShort("auth.logout"),
		CacheRefreshUse: registryLeafUse("cache.refresh"), CacheRefreshShort: registryShort("cache.refresh"),
		CacheStatusUse: registryLeafUse("cache.status"), CacheStatusShort: registryShort("cache.status"),
		WorkbookPullUse: registryLeafUse("workbook.pull"), WorkbookPullShort: registryShort("workbook.pull"),
		WorkbookPublishUse: registryLeafUse("workbook.publish"), WorkbookPublishShort: registryShort("workbook.publish"),
	})
	registrations, err := cli.RegisteredCommands(root)
	if err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "CLI command registration validation failed.", Cause: err}, renderOptions)
	}
	bindings := make([]capability.Binding, len(registrations))
	for index, registration := range registrations {
		bindings[index] = capability.Binding{CapabilityID: registration.CapabilityID, CommandPath: registration.CommandPath}
	}
	if err := capability.ValidateBindings(definitions, bindings); err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "Capability registry validation failed.", Cause: err}, renderOptions)
	}
	root.SetOut(stdout)
	root.SetArgs(args)
	if err := cli.ValidateHelpArgs(root, args); err != nil {
		capture.enabled = false
		return fail(err, renderOptions)
	}
	selected, _, findErr := root.Find(args)
	if selected != nil {
		capture.operation = selected.Annotations[cli.CapabilityAnnotation]
		if options.publicationExecution != nil && options.publicationExecution.Operation != "" && capture.operation != options.publicationExecution.Operation {
			capture.enabled = false
			return fail(operationrun.WorkerError("identity", "The saved publication command no longer matches its recorded operation; no action was started.", nil), renderOptions)
		}
		capture.enabled = capture.operation != "last" && capture.operation != "session.overview"
		if options.publicationExecution != nil {
			capture.enabled = false
		}
	}
	if cli.RootVersionRequested(args) {
		capture.operation = "version.get"
		capture.enabled = true
	}
	if findErr != nil {
		if selected == nil {
			selected = root
		}
		return fail(cli.CommandUsageError(selected, findErr), renderOptions)
	}
	if err := cli.ValidateArguments(selected, args); err != nil {
		return fail(err, renderOptions)
	}
	bindPublicationExecution(root, runtime, capture, args, options)
	cli.BindManagedPolicy(root, runtime.checkManagedCapability, runtime.checkManagedRemoteMutation)
	if err := root.ExecuteContext(ctx); err != nil {
		if clierr.IsRendered(err) {
			return errs.ExitCode(err)
		}
		var structured *errs.Error
		if !errors.As(err, &structured) {
			if strings.HasPrefix(err.Error(), "unknown command ") {
				err = cli.CommandUsageError(selected, err)
			} else {
				err = &errs.Error{Kind: errs.KindRuntime, Operation: "cli", Summary: err.Error(), Cause: err}
			}
		}
		return fail(err, renderOptions)
	}
	return 0
}

func renderError(writer io.Writer, err error, renderOptions *cli.RenderOptions) int {
	if renderErr := output.RenderError(writer, err, output.Options{JSON: renderOptions != nil && renderOptions.JSON}); renderErr != nil {
		return 1
	}
	return errs.ExitCode(err)
}

type writerRenderer struct {
	capture *lastCapture
	writer  io.Writer
	options *cli.RenderOptions
	saved   bool
}

func (r writerRenderer) Render(value any) error {
	if r.capture != nil {
		r.capture.value = value
		return nil
	}
	full := r.options != nil && r.options.Full
	jsonOutput := r.options != nil && r.options.JSON
	configPath := hintConfigPath(r.options)
	if saved, ok := value.(interface{ IsSavedResult() bool }); ok && saved.IsSavedResult() {
		full = true
		configPath = ""
	}
	return output.RenderWithOptions(r.writer, value, output.Options{Full: full, JSON: jsonOutput, ConfigPath: configPath, SavedResult: r.saved})
}

func hintConfigPath(options *cli.RenderOptions) string {
	if options == nil || options.HintConfig == nil {
		return ""
	}
	path := options.HintConfig()
	if path == "" {
		return ""
	}
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return path
}

func renderErrorWithOptions(writer io.Writer, err error, options *cli.RenderOptions) int {
	full := options != nil && options.Full
	if renderErr := output.RenderError(writer, err, output.Options{Full: full, JSON: options != nil && options.JSON, ConfigPath: hintConfigPath(options)}); renderErr != nil {
		return 1
	}
	return errs.ExitCode(err)
}

func hasJSONFlag(args []string) bool {
	enabled := false
	for index, arg := range args {
		if arg == "--" {
			break
		}
		if index > 0 && strings.HasPrefix(args[index-1], "-") && !isBooleanFlag(args[index-1]) && isValueFlag(args[index-1]) {
			continue
		}
		if arg == "--json" || arg == "--jsn" {
			enabled = true
			continue
		}
		for _, name := range []string{"--json=", "--jsn="} {
			if strings.HasPrefix(arg, name) {
				if value, err := strconv.ParseBool(strings.TrimPrefix(arg, name)); err == nil {
					enabled = value
				}
			}
		}
	}
	return enabled
}

func isBooleanFlag(arg string) bool {
	name := strings.SplitN(arg, "=", 2)[0]
	switch name {
	case "--full", "--preview", "--all", "--cache", "--force", "--overwrite", "--raw", "--mutation", "--include-pds", "--include-extract":
		return true
	default:
		return false
	}
}

func isValueFlag(arg string) bool {
	if strings.Contains(arg, "=") {
		return false
	}
	name := strings.SplitN(arg, "=", 2)[0]
	switch name {
	case "--config", "--domain", "--environment", "--env", "--id", "--name", "--limit", "--query", "--resource", "--owner", "--project", "--site", "--workspace", "-e", "-i", "-l", "-n", "-q", "-w":
		return true
	default:
		return false
	}
}

type runtimeDependencies struct {
	managedPolicy        managedPolicySource
	managedChecks        managedCapabilityChecks
	command              commandRuntime
	configPath           string
	httpClient           *http.Client
	now                  func() time.Time
	correlationID        string
	userHomeDir          func() (string, error)
	patStore             coreauth.PATStore
	authPrompter         authcli.Prompter
	jobDirectory         string
	progressWriter       io.Writer
	publicationExecution *operationrun.PublicationExecution
	operationDirectory   string
}

func newRuntime(options Options) (*runtimeDependencies, error) {
	path := options.ConfigPath
	if path == "" {
		var err error
		path, err = config.UserConfigPath()
		if err != nil {
			return nil, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "Configuration path resolution failed.", Cause: err}
		}
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Minute}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	correlation := ""
	if options.CorrelationID != nil {
		correlation = options.CorrelationID()
	}
	if correlation == "" {
		var value [16]byte
		if _, err := rand.Read(value[:]); err == nil {
			correlation = fmt.Sprintf("%x", value[:])
		}
	}
	userHomeDir := options.UserHomeDir
	if userHomeDir == nil {
		userHomeDir = os.UserHomeDir
	}
	patStore := options.PATStore
	if patStore == nil {
		patStore = coreauth.NewOSPATStore()
	}
	prompter := options.AuthPrompter
	if prompter == nil {
		prompter = authcli.NewTerminalPrompter()
	}
	policy := options.managedPolicy
	if policy == nil {
		policy = managedpolicy.Load(capability.All())
	}
	return &runtimeDependencies{managedPolicy: policy, configPath: path, httpClient: client, now: now, correlationID: correlation, userHomeDir: userHomeDir, patStore: patStore, authPrompter: prompter, jobDirectory: options.JobDirectory, progressWriter: options.Stderr, publicationExecution: options.publicationExecution, operationDirectory: options.OperationDirectory}, nil
}

func (r *runtimeDependencies) Resolve(_ context.Context, alias string) (authops.CheckTarget, error) {
	_, environment, err := r.environment(alias, false)
	if err != nil {
		return authops.CheckTarget{}, err
	}
	return authops.CheckTarget{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv, CredentialReference: environment.Auth.CredentialRef}, nil
}

func (r *runtimeDependencies) Authenticate(ctx context.Context, target authops.CheckTarget) (authops.CheckAuthentication, error) {
	session, source, err := r.commandSessions().AuthenticateWithSource(ctx, coreauth.Target{Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, PATNameVariable: target.PATNameVariable, PATSecretVariable: target.PATSecretVariable, CredentialReference: target.CredentialReference}, tableauauth.NewClient(r.transport(target.APIVersion)))
	provenance := string(source)
	if source == coreauth.CredentialSourceOSKeyring {
		provenance = "os_credential_store"
	}
	if err != nil {
		_, missing := errors.AsType[*coreauth.MissingVariablesError](err)
		_, partial := errors.AsType[*coreauth.PartialEnvironmentCredentialsError](err)
		_, store := errors.AsType[*coreauth.CredentialStoreError](err)
		if missing || partial || store {
			err = &errs.Error{ID: "auth.credentials", Kind: errs.KindOperation, Operation: "auth.check", Environment: target.Environment, Site: target.SiteContentURL, Summary: "Authentication credentials are unavailable.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
		}
		return authops.CheckAuthentication{CredentialSource: provenance}, err
	}
	return authops.CheckAuthentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID(), CredentialSource: provenance}, nil
}

func (r *runtimeDependencies) environment(alias string, explicit bool) (config.Config, config.Environment, error) {
	configuration, err := r.configuration()
	if err != nil {
		return config.Config{}, config.Environment{}, err
	}
	var environment config.Environment
	if explicit {
		environment, err = configuration.ResolveWriteEnvironment(alias)
	} else {
		environment, err = configuration.ResolveEnvironment(alias)
	}
	if err != nil {
		return configuration, environment, &errs.Error{ID: "environment.resolve", Kind: errs.KindUsage, Operation: "environment.resolve", Environment: alias, Summary: "A configured environment alias is required, not a Tableau site name or URL.", Cause: err, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Prerequisite: &errs.Prerequisite{Kind: "environment", Resource: alias, Summary: "Select a configured environment alias."}}
	}
	return configuration, environment, err
}

func (r *runtimeDependencies) workbookAdapter(ctx context.Context, alias string, explicit bool) (config.Environment, *resourceworkbook.Adapter, resourceworkbook.ProjectResolution, error) {
	connection, err := r.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return connection.environment, nil, nil, err
	}
	clients := r.clients(connection)
	projects := resourceproject.NewAdapter(clients.projects)
	return connection.environment, resourceworkbook.NewAdapterWithProjectIdentityResolver(clients.workbooks, projects), projects, nil
}

type authenticatedTableau struct {
	configuration config.Config
	environment   config.Environment
	transport     *tableau.Transport
	session       coreauth.Session
}

func (r *runtimeDependencies) tableauConnection(ctx context.Context, alias string, explicit bool) (authenticatedTableau, error) {
	configuration, environment, err := r.environment(alias, explicit)
	if err != nil {
		return authenticatedTableau{configuration: configuration, environment: environment}, err
	}
	transport := r.transport(environment.APIVersion)
	session, err := r.authenticate(ctx, coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv, CredentialReference: environment.Auth.CredentialRef}, environment.APIVersion)
	return authenticatedTableau{configuration: configuration, environment: environment, transport: transport, session: session}, err
}

type workbookPublishProvider struct{ runtime *runtimeDependencies }

func (p workbookPublishProvider) ResolveWorkbookPublishTarget(_ context.Context, alias, site string) (string, string, error) {
	_, environment, err := p.runtime.environment(alias, true)
	if err != nil {
		return "", "", capabilitySetupError("workbook.publish.setup", "workbook.publish", alias, site, "Workbook publish target resolution failed.", "Select the destination with --env.", err)
	}
	return environment.Alias, environment.SiteContentURL, nil
}

func (p workbookPublishProvider) OpenWorkbookPublishSource(ctx context.Context, input workbookops.PublishInput) (workbookops.PublishInput, workbookops.ArtifactReader, string, error) {
	source := resourceworkbook.PublishSource{Manager: artifact.NewWorkbookManager(p.runtime.now), Workspace: func(ctx context.Context, selector, alias string) (string, string, error) {
		workspace, err := (&workspaceRuntime{runtime: p.runtime}).resolveForEnvironment(ctx, selector, alias)
		return workspace.Name, workspace.Root, err
	}}
	return source.Open(ctx, input)
}

func (p workbookPublishProvider) OpenWorkbookPublish(ctx context.Context, alias, site, sourcePath string) (workbookops.PublishSession, error) {
	environment, adapter, projects, err := p.runtime.workbookAdapter(ctx, alias, true)
	if err != nil {
		return workbookops.PublishSession{}, capabilitySetupError("workbook.publish.setup", "workbook.publish", alias, site, "Workbook publish setup failed.", "Review the explicit environment, site, and PAT configuration.", err)
	}
	var lifecycle *resourceworkbook.PublicationLifecycle
	bridge := resourceworkbook.PublishPorts{
		Adapter:  adapter,
		Projects: projects,
		Fresh: func(ctx context.Context) (*resourceworkbook.Adapter, error) {
			_, fresh, _, err := p.runtime.workbookAdapter(ctx, environment.Alias, true)
			return fresh, err
		},
		Begin: func(ctx context.Context, request workbookops.PublishRequest) (resourceworkbook.PublishAcceptance, error) {
			monitor, asJob, err := p.runtime.publication(ctx, environment.Alias, "workbook", sourcePath, request.ProjectLUID, request.Name)
			if err != nil {
				return resourceworkbook.PublishAcceptance{}, err
			}
			readback := resourceworkbook.FreshPublicationDestination{
				Name: request.Name, ProjectID: request.ProjectLUID,
				Open: func(ctx context.Context) (*resourceworkbook.Adapter, error) {
					connection, err := newRemoteContentCommands(p.runtime).connect(ctx, monitor.Base.Environment, true)
					if err != nil {
						return nil, err
					}
					return connection.workbooks, nil
				},
				Progress: func(ctx context.Context) { progress.SetLabel(ctx, "Confirming published content") },
			}
			lifecycle = &resourceworkbook.PublicationLifecycle{Publication: monitor, Readback: readback}
			return resourceworkbook.NewPublishAcceptance(monitor, asJob, environment.Alias, environment.SiteContentURL, readback, contentbatch.Bulk), nil
		},
		Progress: func(ctx context.Context, label string) { progress.SetLabel(ctx, label) },
	}
	return workbookops.PublishSession{Environment: environment.Alias, Site: environment.SiteContentURL, Resolver: bridge, Preparer: bridge,
		Lifecycle: func() workbookops.PublishLifecycle {
			if lifecycle == nil {
				return nil
			}
			return lifecycle
		},
		NoWait: p.runtime.publicationExecution.StopRequested, Defer: func(ctx context.Context, finish func(context.Context) (any, error)) {
			contentbatch.DeferCompletion(ctx, finish)
		},
	}, nil
}

func resolvedTarget(environmentAlias, site string, environment config.Environment) (string, string) {
	if environment.Alias != "" {
		environmentAlias = environment.Alias
	}
	if environment.SiteContentURL != "" || environment.Alias != "" {
		site = environment.SiteContentURL
	}
	return environmentAlias, site
}

func capabilitySetupError(id, operation, environment, site, summary, fallbackAction string, err error) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(err, fallbackAction)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
}

type registryMutationPolicy struct{}

func (registryMutationPolicy) IsRemoteMutation(id string) bool {
	definition, ok := capability.Lookup(id)
	return ok && definition.RemoteMutation
}

func registryUse(id string) string {
	definition, ok := capability.Lookup(id)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(definition.Surface, "tadx capability ")
}

func registryShort(id string) string {
	definition, ok := capability.Lookup(id)
	if !ok {
		return ""
	}
	return definition.Outcome
}

func registryLeafUse(id string) string {
	definition, ok := capability.Lookup(id)
	if !ok || len(definition.CommandPath) == 0 {
		return ""
	}
	return definition.CommandPath[len(definition.CommandPath)-1]
}

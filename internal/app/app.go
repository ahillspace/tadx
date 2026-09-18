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

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authlogin "github.com/ahillspace/tadx/actions/auth/login"
	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	lastaction "github.com/ahillspace/tadx/actions/last"
	mutationset "github.com/ahillspace/tadx/actions/mutation/set"
	mutationstatus "github.com/ahillspace/tadx/actions/mutation/status"
	sessionoverview "github.com/ahillspace/tadx/actions/session/overview"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/artifact"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	"github.com/ahillspace/tadx/internal/value"
)

// Options contains process-level discovery settings.
type Options struct {
	MutationsEnabled    bool
	MutationEnvironment func() (string, bool)
	ConfigPath          string
	HTTPClient          *http.Client
	PATStore            coreauth.PATStore
	AuthPrompter        authcli.Prompter
	Now                 func() time.Time
	CorrelationID       func() string
	UserHomeDir         func() (string, error)
	Stderr              io.Writer
	JobDirectory        string
}

// Run wires and runs the CLI, renders structured output, and returns an AXI exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, options Options) (exitCode int) {
	definitions := capability.All()
	source := registrySource{}
	renderOptions := &cli.RenderOptions{HintConfig: func() string { return options.ConfigPath }}
	renderOptions.JSON = hasJSONFlag(args)
	runtime, err := newRuntime(options)
	if err != nil {
		return renderError(stdout, err, renderOptions)
	}
	defer runtime.Close()
	capture := newLastCapture(runtime)
	capture.hintConfig = func() string { return hintConfigPath(renderOptions) }
	defer func() {
		if err := capture.save(exitCode); err != nil {
			warningWriter := options.Stderr
			if warningWriter == nil {
				warningWriter = os.Stderr
			}
			_ = output.RenderWithOptions(warningWriter, lastResultWarning(), output.Options{JSON: renderOptions.JSON})
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
	environmentCommands := newEnvironmentCommands(runtime)
	workspaceCommands := newWorkspaceCommands(runtime)
	remoteContent := newRemoteContentCommands(runtime)
	remoteAdmin := newRemoteAdminCommands(runtime)
	pulseActions := newPulseCommands(runtime)
	doctorCommands := newDoctorCommands(runtime)
	cacheGroup2 := newCacheGroup2Commands(runtime)
	credentialStore := authCredentialStore{runtime: runtime}
	root := cli.NewRoot(cli.Dependencies{
		SessionOverview:       sessionoverview.New(sessionOverviewReader{runtime: runtime}),
		Update:                newUpdateCommand(runtime),
		Catalog:               (&catalogCommands{runtime: runtime}).dependencies(),
		ContentLabels:         contentLabelDependencies(runtime),
		AdminLabels:           adminLabelDependencies(runtime),
		Lister:                capabilitylist.New(source),
		Getter:                capabilityget.New(source),
		Renderer:              writerRenderer{writer: stdout, options: renderOptions, capture: capture},
		RenderOptions:         renderOptions,
		BatchSelectors:        capability.BatchSelectors(),
		BatchOptions:          capability.BatchOptions(),
		ConfigPath:            &runtime.configPath,
		MutationsEnabled:      options.MutationsEnabled,
		MutationPolicy:        registryMutationPolicy{},
		ResolveMutationPolicy: runtime.mutationPolicy,
		MutationStatus:        mutationstatus.New(runtime),
		MutationSetter:        mutationset.New(runtime),
		LastReader:            lastaction.New(capture.store),
		Jobs:                  newJobCommands(runtime).dependencies(),
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
		AuthChecker:         authcheck.New(runtime, runtime),
		Searcher:            newSearchCommands(runtime),
		CacheRefresher:      cacheGroup2.refresher(),
		CacheStatuser:       cacheGroup2.statuser(),
		WorkbookPuller:      &pullService{runtime: runtime},
		WorkbookPublisher:   &publishService{runtime: runtime},
		Content:             remoteContent.dependencies(),
		EnvironmentProfiles: environmentCommands.dependencies(),
		Workspaces:          workspaceCommands.dependencies(),
		Admin:               remoteAdmin.dependencies(),
		Agent:               newAgentCommands(runtime),
		Pulse:               pulseActions.dependencies(),
		Version:             newVersionCommand(runtime),
		DoctorRunner:        doctorCommands,
		DoctorUse:           registryLeafUse("doctor.run"),
		DoctorShort:         registryShort("doctor.run"),
		AuthUse:             registryLeafUse("auth.check"), AuthShort: registryShort("auth.check"),
		AuthStatuser: newAuthStatus(runtime), AuthStatusUse: registryLeafUse("auth.status"), AuthStatusShort: registryShort("auth.status"),
		AuthLogin:    authlogin.New(authCredentialResolver{runtime: runtime}, loginAuthenticator{runtime: runtime}, credentialStore),
		AuthLogout:   authlogout.New(authLogoutResolver{runtime: runtime}, credentialStore),
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
		capture.enabled = capture.operation != "last" && capture.operation != "session.overview"
	}
	if cli.RootVersionRequested(args) {
		capture.operation = "version.get"
		capture.enabled = true
	}
	if findErr != nil {
		return fail(&errs.Error{Kind: errs.KindUsage, Operation: "cli", Summary: findErr.Error(), Cause: findErr}, renderOptions)
	}
	if err := root.ExecuteContext(ctx); err != nil {
		if clierr.IsRendered(err) {
			return errs.ExitCode(err)
		}
		var structured *errs.Error
		if !errors.As(err, &structured) {
			err = &errs.Error{Kind: errs.KindRuntime, Operation: "cli", Summary: err.Error(), Cause: err}
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
		if arg == "--json" || arg == "--jsn" {
			if index == 0 || !strings.HasPrefix(args[index-1], "-") || isBooleanFlag(args[index-1]) || !isValueFlag(args[index-1]) {
				enabled = true
			}
			continue
		}
		for _, name := range []string{"--json=", "--jsn="} {
			if strings.HasPrefix(arg, name) && (index == 0 || !strings.HasPrefix(args[index-1], "-") || isBooleanFlag(args[index-1]) || !isValueFlag(args[index-1])) {
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
	mutationOverride func() (string, bool)
	command          commandRuntime
	configPath       string
	httpClient       *http.Client
	now              func() time.Time
	correlationID    string
	userHomeDir      func() (string, error)
	patStore         coreauth.PATStore
	authPrompter     authcli.Prompter
	jobDirectory     string
	progressWriter   io.Writer
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
		prompter = newTerminalCredentialPrompter()
	}
	mutationOverride := options.MutationEnvironment
	if mutationOverride == nil && options.MutationsEnabled {
		mutationOverride = func() (string, bool) { return "1", true }
	}
	return &runtimeDependencies{mutationOverride: mutationOverride, configPath: path, httpClient: client, now: now, correlationID: correlation, userHomeDir: userHomeDir, patStore: patStore, authPrompter: prompter, jobDirectory: options.JobDirectory, progressWriter: options.Stderr}, nil
}

func (r *runtimeDependencies) Resolve(_ context.Context, alias string) (authcheck.Target, error) {
	_, environment, err := r.environment(alias, false)
	if err != nil {
		return authcheck.Target{}, err
	}
	return authcheck.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv, CredentialReference: environment.Auth.CredentialRef}, nil
}

func (r *runtimeDependencies) Authenticate(ctx context.Context, target authcheck.Target) (authcheck.Authentication, error) {
	session, source, err := r.commandSessions().AuthenticateWithSource(ctx, coreauth.Target{Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, PATNameVariable: target.PATNameVariable, PATSecretVariable: target.PATSecretVariable, CredentialReference: target.CredentialReference}, tableauauth.NewClient(r.transport(target.APIVersion)))
	provenance := string(source)
	if source == coreauth.CredentialSourceOSKeyring {
		provenance = "os_credential_store"
	}
	if err != nil {
		return authcheck.Authentication{CredentialSource: provenance}, err
	}
	return authcheck.Authentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID(), CredentialSource: provenance}, nil
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

func (r *runtimeDependencies) workbookAdapter(ctx context.Context, alias string, explicit bool) (config.Config, config.Environment, *resourceworkbook.Adapter, string, error) {
	connection, err := r.tableauConnection(ctx, alias, explicit)
	if err != nil {
		return connection.configuration, connection.environment, nil, "", err
	}
	clients := r.clients(connection)
	projects := resourceproject.NewAdapter(clients.projects)
	return connection.configuration, connection.environment, resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, workbookProjectResolver{projects}), connection.session.SiteLUID(), nil
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

type pullService struct{ runtime *runtimeDependencies }

func (s *pullService) Execute(ctx context.Context, input workbookpull.Input) (workbookpull.Output, error) {
	if err := workbookpull.ValidateInput(input); err != nil {
		return workbookpull.Output{}, err
	}
	resolvedWorkspace, err := (&workspaceRuntime{runtime: s.runtime}).resolveForEnvironment(ctx, input.Workspace, input.Environment)
	if err != nil {
		return workbookpull.Output{}, capabilitySetupError("workbook.pull.workspace", "workbook.pull", input.Environment, input.Site, "Workbook workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	input.Workspace = resolvedWorkspace.Root
	input.WorkspaceName = resolvedWorkspace.Name
	connection, err := s.runtime.tableauConnection(ctx, input.Environment, false)
	environment := connection.environment
	if err != nil {
		environmentAlias, site := resolvedTarget(input.Environment, input.Site, environment)
		return workbookpull.Output{}, capabilitySetupError("workbook.pull.setup", "workbook.pull", environmentAlias, site, "Workbook pull setup failed.", "Review the environment, site, and PAT configuration.", err)
	}
	clients := s.runtime.clients(connection)
	workbookClient, metadataClient, datasourceClient := clients.workbooks, clients.metadata, clients.datasources
	workbooks := resourceworkbook.NewAdapterWithProjectResolver(workbookClient, s.runtime.discoveryPaths(connection))
	references := resourceworkbook.NewReferenceAdapter(metadataClient)
	lineage := resourcelineage.NewAdapter(metadataClient)
	datasources := resourcedatasource.NewAdapter(datasourceClient)
	input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	input.ServerOrigin, err = artifact.NormalizeServerOrigin(environment.URL)
	if err != nil {
		return workbookpull.Output{}, capabilitySetupError("workbook.pull.source", "workbook.pull", input.Environment, input.Site, "Workbook source identity resolution failed.", "Review the configured Tableau server URL, then retry.", err)
	}
	input.SiteLUID = connection.session.SiteLUID()

	return workbookpull.New(
		pullReader{workbooks: workbooks, references: references, datasources: datasources, lineage: lineage},
		artifactWriter{workbooks: artifact.NewWorkbookManager(s.runtime.now), bundles: artifact.NewWorkbookBundleManager(s.runtime.now)},
	).Execute(ctx, input)
}

type pullReader struct {
	workbooks   *resourceworkbook.Adapter
	references  *resourceworkbook.ReferenceAdapter
	datasources *resourcedatasource.Adapter
	lineage     *resourcelineage.Adapter
}

func (r pullReader) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookpull.Workbook, error) {
	item, err := r.workbooks.ResolveWorkbook(ctx, selector)
	return workbookpull.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}
func (r pullReader) DownloadWorkbook(ctx context.Context, luid string, include *bool) (workbookpull.Download, error) {
	item, err := r.workbooks.DownloadWorkbook(ctx, luid, include)
	return workbookpull.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

func (r pullReader) PublishedDatasources(ctx context.Context, luid string) ([]workbookpull.PublishedDatasource, error) {
	items, err := r.references.PublishedDatasources(ctx, luid)
	result := make([]workbookpull.PublishedDatasource, len(items))
	for index, item := range items {
		result[index] = workbookpull.PublishedDatasource{LUID: item.LUID, Name: item.Name}
	}
	return result, err
}

func (r pullReader) DownloadPublishedDatasource(ctx context.Context, luid string) (workbookpull.DatasourceDownload, error) {
	item, err := r.datasources.DownloadDatasource(ctx, luid)
	if err != nil {
		return workbookpull.DatasourceDownload{}, err
	}
	project, err := r.workbooks.ResolveProject(ctx, identity.Selector{LUID: identity.LUID(item.ProjectLUID)})
	if err != nil {
		return workbookpull.DatasourceDownload{}, fmt.Errorf("resolve datasource project %q: %w", item.ProjectLUID, err)
	}
	return workbookpull.DatasourceDownload{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: project.Path, Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, nil
}

func (r pullReader) CaptureWorkbookLineage(ctx context.Context, input workbookpull.LineageRequest) (workbookpull.LineageCapture, error) {
	graph, err := r.lineage.Capture(ctx, resourcelineage.Request{Kind: "workbook", RESTLUID: input.RESTLUID, Direction: input.Direction, Depth: input.Depth})
	nodes := make([]workbookpull.LineageNode, len(graph.Nodes))
	for index, node := range graph.Nodes {
		nodes[index] = workbookpull.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]workbookpull.LineageEdge, len(graph.Edges))
	for index, edge := range graph.Edges {
		edges[index] = workbookpull.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return workbookpull.LineageCapture{RootMetadataID: graph.RootMetadataID, Complete: graph.Complete, Direction: graph.Direction, Depth: graph.Depth, Failure: graph.Failure, Nodes: nodes, Edges: edges, Warnings: append([]string(nil), graph.Warnings...)}, err
}

type artifactWriter struct {
	workbooks *artifact.WorkbookManager
	bundles   *artifact.WorkbookBundleManager
}

func (w artifactWriter) WriteWorkbook(ctx context.Context, input workbookpull.Artifact) (workbookpull.ArtifactResult, error) {
	references := make([]artifact.PublishedDatasourceRef, len(input.PublishedDatasources))
	for index, item := range input.PublishedDatasources {
		references[index] = artifact.PublishedDatasourceRef{LUID: item.LUID, Name: item.Name, SourceSite: item.SourceSite, LocalArtifactPath: item.LocalArtifactPath}
	}
	result, err := w.workbooks.Pull(ctx, artifact.WorkbookPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Lineage: workbookLineageDocument(input.Lineage), LineageCountsKnown: input.LineageCountsKnown, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: input.Name, TableauID: input.TableauID, SourceServerOrigin: input.ServerOrigin, SourceSiteLUID: input.SiteLUID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID, Portability: input.Portability, PublishedDatasources: references, DependenciesAcquired: input.DependenciesAcquired}})
	return workbookpull.ArtifactResult{Path: result.ArtifactPath, CanonicalPath: result.CanonicalPath, LineagePath: result.LineagePath, LineageStatus: result.LineageStatus, BaselineFingerprint: result.BaselineFingerprint, Warnings: result.Warnings}, err
}

func (w artifactWriter) WriteBundle(ctx context.Context, workbook workbookpull.Artifact, datasources []workbookpull.DatasourceArtifact) (workbookpull.ArtifactResult, error) {
	references := make([]artifact.PublishedDatasourceRef, len(workbook.PublishedDatasources))
	for index, item := range workbook.PublishedDatasources {
		references[index] = artifact.PublishedDatasourceRef{LUID: item.LUID, Name: item.Name, SourceSite: item.SourceSite}
	}
	bundle := artifact.WorkbookBundlePull{
		Workbook:    artifact.WorkbookPull{Workspace: workbook.Workspace, Filename: workbook.Filename, Content: workbook.Content, Overwrite: workbook.Overwrite, Lineage: workbookLineageDocument(workbook.Lineage), LineageCountsKnown: workbook.LineageCountsKnown, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: workbook.Name, TableauID: workbook.TableauID, SourceServerOrigin: workbook.ServerOrigin, SourceSiteLUID: workbook.SiteLUID, SourceEnvironment: workbook.Environment, SourceSite: workbook.Site, SourceProjectName: workbook.ProjectName, SourceProjectID: workbook.ProjectID, Portability: workbook.Portability, PublishedDatasources: references, DependenciesAcquired: true}},
		Datasources: make([]artifact.DatasourcePull, len(datasources)),
	}
	for index, item := range datasources {
		bundle.Datasources[index] = artifact.DatasourcePull{Workspace: item.Workspace, Filename: item.Filename, Content: item.Content, Overwrite: item.Overwrite, Metadata: artifact.DatasourceMetadata{Kind: "datasource", Name: item.Name, TableauID: item.TableauID, SourceServerOrigin: item.ServerOrigin, SourceSiteLUID: item.SiteLUID, SourceEnvironment: item.Environment, SourceSite: item.Site, SourceProjectName: item.ProjectName, SourceProjectID: item.ProjectID}}
	}
	result, err := w.bundles.Pull(ctx, bundle)
	if err != nil {
		return workbookpull.ArtifactResult{}, err
	}
	dependencies := make([]workbookpull.DependencyArtifactResult, len(result.Datasources))
	pathByLUID := make(map[string]string, len(result.Datasources))
	for index, item := range result.Datasources {
		source := datasources[index]
		dependencies[index] = workbookpull.DependencyArtifactResult{LUID: source.TableauID, Name: source.Name, Path: item.WorkspaceRelativePath, CanonicalPath: item.CanonicalPath, BaselineFingerprint: item.BaselineFingerprint, Warnings: item.Warnings}
		pathByLUID[source.TableauID] = item.WorkspaceRelativePath
	}
	outputReferences := make([]workbookpull.PublishedDatasourceRef, len(workbook.PublishedDatasources))
	for index, item := range workbook.PublishedDatasources {
		item.LocalArtifactPath = pathByLUID[item.LUID]
		outputReferences[index] = item
	}
	return workbookpull.ArtifactResult{Path: result.Workbook.ArtifactPath, CanonicalPath: result.Workbook.CanonicalPath, LineagePath: result.Workbook.LineagePath, LineageStatus: result.Workbook.LineageStatus, BaselineFingerprint: result.Workbook.BaselineFingerprint, Portability: workbook.Portability, PublishedDatasources: outputReferences, DependenciesAcquired: true, Dependencies: dependencies, Warnings: result.Workbook.Warnings}, nil
}

func workbookLineageDocument(input workbookpull.LineageCapture) artifact.LineageDocument {
	nodes := make([]artifact.LineageNode, len(input.Nodes))
	for index, node := range input.Nodes {
		nodes[index] = artifact.LineageNode{MetadataID: node.MetadataID, Kind: node.Kind, RESTLUID: node.RESTLUID, Name: node.Name}
	}
	edges := make([]artifact.LineageEdge, len(input.Edges))
	for index, edge := range input.Edges {
		edges[index] = artifact.LineageEdge{FromMetadataID: edge.FromMetadataID, ToMetadataID: edge.ToMetadataID, Relationship: edge.Relationship}
	}
	return artifact.LineageDocument{Complete: input.Complete, Direction: input.Direction, Depth: input.Depth, Failure: artifactLineageFailure(input.Failure), Nodes: nodes, Edges: edges, Warnings: append([]string(nil), input.Warnings...)}
}

func artifactLineageFailure(failure *value.LineageFailure) *artifact.LineageFailure {
	if failure == nil {
		return nil
	}
	return &artifact.LineageFailure{Provider: failure.Provider, Relation: failure.Relation, RootKind: failure.RootKind, RootRESTLUID: failure.RootRESTLUID, RequestID: failure.RequestID}
}

type publishService struct{ runtime *runtimeDependencies }

func (s *publishService) Execute(ctx context.Context, input workbookpublish.Input, preview bool) (workbookpublish.Output, error) {
	if err := workbookpublish.ValidateInput(input); err != nil {
		return workbookpublish.Output{}, err
	}
	manager := artifact.NewWorkbookManager(s.runtime.now)
	var adapter *resourceworkbook.Adapter
	_, environment, err := s.runtime.environment(input.Environment, true)
	if err != nil {
		return workbookpublish.Output{}, capabilitySetupError("workbook.publish.setup", "workbook.publish", input.Environment, input.Site, "Workbook publish target resolution failed.", "Select the destination with --env.", err)
	}
	input.Environment, input.Site, input.TargetResolved = environment.Alias, environment.SiteContentURL, true
	var reader workbookpublish.ArtifactReader
	if input.File != "" {
		input.ArtifactPath = input.File
		reader = nativeWorkbookArtifactReader{}
		if _, err := reader.ReadWorkbook(ctx, input.File); err != nil {
			return workbookpublish.Output{}, capabilitySetupError("workbook.publish.file", "workbook.publish", input.Environment, input.Site, "Native workbook file is invalid.", "Select an existing .twb or .twbx file with --file.", err)
		}
	} else {
		resolvedWorkspace, err := (&workspaceRuntime{runtime: s.runtime}).resolveForEnvironment(ctx, input.Workspace, environment.Alias)
		if err != nil {
			return workbookpublish.Output{}, capabilitySetupError("workbook.publish.workspace", "workbook.publish", input.Environment, input.Site, "Workbook workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
		}
		managedArtifact, err := artifact.Resolve(ctx, resolvedWorkspace.Root, artifact.Selector{Path: input.ArtifactPath, Kind: "workbook", LUID: input.ArtifactID, Name: input.ArtifactName})
		if err != nil {
			if _, ambiguous := errors.AsType[*artifact.AmbiguousSelectorError](err); ambiguous {
				return workbookpublish.Output{}, mapArtifactResolutionError("workbook.publish", resolvedWorkspace.Name, input.ArtifactID, err)
			}
			return workbookpublish.Output{}, capabilitySetupError("workbook.publish.artifact", "workbook.publish", input.Environment, input.Site, "Workbook artifact resolution failed.", "Select one exact workspace-relative managed workbook artifact, then retry.", err)
		}
		input.ArtifactPath = filepath.Join(resolvedWorkspace.Root, filepath.FromSlash(managedArtifact.Path))
		input.WorkspaceName = resolvedWorkspace.Name
		if _, err := manager.Read(ctx, input.ArtifactPath); err != nil {
			return workbookpublish.Output{}, capabilitySetupError("workbook.publish.artifact", "workbook.publish", input.Environment, input.Site, "Workbook artifact read failed.", "Repair or pull the exact workbook artifact, then retry.", err)
		}
		reader = artifactReader{manager: manager, displayPath: managedArtifact.Path}
	}
	_, environment, adapter, _, err = s.runtime.workbookAdapter(ctx, input.Environment, true)
	if err != nil {
		return workbookpublish.Output{}, capabilitySetupError("workbook.publish.setup", "workbook.publish", input.Environment, input.Site, "Workbook publish setup failed.", "Review the explicit environment, site, and PAT configuration.", err)
	}
	input.Environment, input.Site, input.TargetResolved = environment.Alias, environment.SiteContentURL, true
	var lifecycle *publication
	bridge := publishAdapter{adapter: adapter, runtime: s.runtime, environment: environment.Alias, sourcePath: input.ArtifactPath, lifecycle: &lifecycle}
	if managed, ok := reader.(artifactReader); ok {
		bridge.sourcePath = managed.displayPath
	}
	action := workbookpublish.New(reader, bridge, bridge)
	out, err := action.Execute(ctx, input, preview)
	if out.Result != nil && out.Result.Status != "" && lifecycle != nil {
		var saveErr error
		out.Result.ReceiptPath, saveErr = lifecycle.record(ctx, out.Result.JobID, out.Result.Status, out.Result.WorkbookLUID, out.Result.TableauRequestID, out.Result.Verification)
		err = errors.Join(err, saveErr)
	}
	if err == nil && out.Result != nil && out.Result.Status == "pending" && lifecycle != nil {
		contentbatch.DeferCompletion(ctx, func(ctx context.Context) (any, error) {
			return lifecycle.completeWorkbook(ctx, action, out)
		})
	}
	return out, err
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

type artifactReader struct {
	manager     *artifact.WorkbookManager
	displayPath string
}

func (r artifactReader) ReadWorkbook(ctx context.Context, path string) (workbookpublish.Artifact, error) {
	item, err := r.manager.Read(ctx, path)
	if err == nil && r.displayPath != "" {
		item.Path = r.displayPath
	}
	return workbookpublish.Artifact{Path: item.Path, PayloadPath: item.PayloadPath, Filename: item.Filename, Size: item.Size, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint, SourceEnvironment: item.SourceEnvironment, SourceSite: item.SourceSite, SourceProjectName: item.SourceProjectName, SourceProjectID: item.SourceProjectID, Portability: item.Portability, PublishedDatasourceCount: item.PublishedDatasourceCount}, err
}

type publishAdapter struct {
	adapter                 *resourceworkbook.Adapter
	runtime                 *runtimeDependencies
	environment, sourcePath string
	lifecycle               **publication
}

func (a publishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (workbookpublish.Project, error) {
	item, err := a.adapter.ResolveProject(ctx, selector)
	return workbookpublish.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}
func (a publishAdapter) FindWorkbooks(ctx context.Context, name, project string) ([]workbookpublish.Workbook, error) {
	if a.runtime != nil {
		_, _, fresh, _, err := a.runtime.workbookAdapter(ctx, a.environment, true)
		if err != nil {
			return nil, err
		}
		a.adapter = fresh
	}
	items, err := a.adapter.FindWorkbooks(ctx, name, project)
	result := make([]workbookpublish.Workbook, len(items))
	for index, item := range items {
		result[index] = workbookpublish.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID}
	}
	return result, err
}
func (a publishAdapter) Prepare(ctx context.Context, input workbookpublish.PublishRequest) (workbookpublish.PreparedPublish, error) {
	request := tableauworkbook.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite, AsJob: input.AsJob}
	if a.runtime != nil {
		p, err := a.runtime.publication(ctx, a.environment, "workbook", a.sourcePath, input.ProjectLUID, input.Name)
		if err != nil {
			return nil, err
		}
		if a.lifecycle != nil {
			*a.lifecycle = p
		}
		request.AsJob = p.asJob
		request.Accepted = p.acceptWorkbook
	}
	prepared, err := a.adapter.PrepareWorkbook(ctx, request)
	if err != nil {
		return nil, err
	}
	return preparedPublishAdapter{prepared: prepared}, nil
}

type preparedPublishAdapter struct {
	prepared *tableauworkbook.PreparedPublish
}

func (a preparedPublishAdapter) Commit(ctx context.Context) (workbookpublish.Result, error) {
	result, err := a.prepared.Commit(ctx)
	warnings := make([]workbookpublish.ValidationIssue, len(result.Warnings))
	for index, warning := range result.Warnings {
		warnings[index] = workbookpublish.ValidationIssue{Severity: warning.Severity, Message: warning.Message, Line: warning.Line, Column: warning.Column, ElementName: warning.ElementName}
	}
	return workbookpublish.Result{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID, ReceiptPath: result.ReceiptPath, ValidationWarnings: warnings}, err
}

type registrySource struct{}

func (registrySource) List(_ context.Context) ([]capabilitylist.Capability, error) {
	return capability.AllDiscoveries(), nil
}

type registryMutationPolicy struct{}

func (registryMutationPolicy) IsRemoteMutation(id string) bool {
	definition, ok := capability.Lookup(id)
	return ok && definition.RemoteMutation
}

func (registrySource) Get(_ context.Context, id string) (capabilityget.Capability, bool) {
	return capability.LookupDiscovery(id)
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

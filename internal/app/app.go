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
	"strings"
	"time"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	capabilityget "github.com/ahillspace/tadx/actions/capability/get"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/artifact"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/cli"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
)

// Options contains process-level discovery settings.
type Options struct {
	MutationsEnabled bool
	ConfigPath       string
	HTTPClient       *http.Client
	Now              func() time.Time
	CorrelationID    func() string
}

// Run wires and runs the CLI, renders structured output, and returns an AXI exit code.
func Run(ctx context.Context, args []string, stdout io.Writer, options Options) int {
	definitions := capability.All()
	source := registrySource{}
	runtime, err := newRuntime(options)
	if err != nil {
		return renderError(stdout, err)
	}
	root := cli.NewRoot(cli.Dependencies{
		Lister:            capabilitylist.New(source),
		Getter:            capabilityget.New(source),
		Renderer:          writerRenderer{writer: stdout},
		MutationsEnabled:  options.MutationsEnabled,
		ListUse:           registryUse("capability.list"),
		ListShort:         registryShort("capability.list"),
		GetUse:            registryUse("capability.get"),
		GetShort:          registryShort("capability.get"),
		AuthChecker:       authcheck.New(runtime, runtime),
		CatalogSearcher:   &catalogService{runtime: runtime},
		WorkbookPuller:    &pullService{runtime: runtime},
		WorkbookPublisher: &publishService{runtime: runtime},
		AuthUse:           registryLeafUse("auth.check"), AuthShort: registryShort("auth.check"),
		CatalogSearchUse: registryLeafUse("catalog.search"), CatalogSearchShort: registryShort("catalog.search"),
		WorkbookPullUse: registryLeafUse("workbook.pull"), WorkbookPullShort: registryShort("workbook.pull"),
		WorkbookPublishUse: registryLeafUse("workbook.publish"), WorkbookPublishShort: registryShort("workbook.publish"),
	})
	registrations, err := cli.RegisteredCommands(root)
	if err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "CLI command registration validation failed.", Cause: err})
	}
	bindings := make([]capability.Binding, len(registrations))
	for index, registration := range registrations {
		bindings[index] = capability.Binding{CapabilityID: registration.CapabilityID, CommandPath: registration.CommandPath}
	}
	if err := capability.ValidateBindings(definitions, bindings); err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindRuntime, Operation: "startup", Summary: "Capability registry validation failed.", Cause: err})
	}
	root.SetOut(stdout)
	root.SetArgs(args)
	if _, _, err := root.Find(args); err != nil {
		return renderError(stdout, &errs.Error{Kind: errs.KindUsage, Operation: "cli", Summary: err.Error(), Cause: err})
	}
	if err := root.ExecuteContext(ctx); err != nil {
		var structured *errs.Error
		if !errors.As(err, &structured) {
			err = &errs.Error{Kind: errs.KindRuntime, Operation: "cli", Summary: err.Error(), Cause: err}
		}
		return renderError(stdout, err)
	}
	return 0
}

func renderError(writer io.Writer, err error) int {
	if renderErr := output.RenderError(writer, err, output.Options{}); renderErr != nil {
		return 1
	}
	return errs.ExitCode(err)
}

type writerRenderer struct {
	writer io.Writer
}

func (r writerRenderer) Render(value any) error {
	return output.Render(r.writer, value)
}

type runtimeDependencies struct {
	configPath    string
	httpClient    *http.Client
	now           func() time.Time
	correlationID string
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
	return &runtimeDependencies{configPath: path, httpClient: client, now: now, correlationID: correlation}, nil
}

func (r *runtimeDependencies) Resolve(_ context.Context, alias string) (authcheck.Target, error) {
	_, environment, err := r.environment(alias, false)
	if err != nil {
		return authcheck.Target{}, err
	}
	return authcheck.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, APIVersion: environment.APIVersion, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv}, nil
}

func (r *runtimeDependencies) Authenticate(ctx context.Context, target authcheck.Target) (authcheck.Authentication, error) {
	transport := tableau.NewTransport(r.httpClient, target.APIVersion, func() string { return r.correlationID })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(ctx, coreauth.Target{Environment: target.Environment, ServerURL: target.ServerURL, SiteContentURL: target.SiteContentURL, PATNameVariable: target.PATNameVariable, PATSecretVariable: target.PATSecretVariable})
	if err != nil {
		return authcheck.Authentication{}, err
	}
	return authcheck.Authentication{SiteLUID: session.SiteLUID(), UserLUID: session.UserLUID()}, nil
}

func (r *runtimeDependencies) environment(alias string, explicit bool) (config.Config, config.Environment, error) {
	if explicit && alias == "" {
		return config.Config{}, config.Environment{}, errors.New("an explicit write environment is required")
	}
	configuration, err := config.Load(r.configPath)
	if err != nil {
		return config.Config{}, config.Environment{}, err
	}
	environment, err := configuration.ResolveEnvironment(alias)
	return configuration, environment, err
}

func (r *runtimeDependencies) workbookAdapter(ctx context.Context, alias string, explicit bool) (config.Config, config.Environment, *resourceworkbook.Adapter, error) {
	configuration, environment, err := r.environment(alias, explicit)
	if err != nil {
		return config.Config{}, config.Environment{}, nil, err
	}
	transport := tableau.NewTransport(r.httpClient, environment.APIVersion, func() string { return r.correlationID })
	provider := coreauth.NewPATProvider(coreauth.LookupEnvFunc(os.LookupEnv), tableauauth.NewClient(transport))
	session, err := provider.Authenticate(ctx, coreauth.Target{Environment: environment.Alias, ServerURL: environment.URL, SiteContentURL: environment.SiteContentURL, PATNameVariable: environment.Auth.PATNameEnv, PATSecretVariable: environment.Auth.PATSecretEnv})
	if err != nil {
		return configuration, environment, nil, err
	}
	client := tableauworkbook.NewClient(transport, session, environment.URL)
	return configuration, environment, resourceworkbook.NewAdapter(client), nil
}

type catalogService struct{ runtime *runtimeDependencies }

func (s *catalogService) Execute(ctx context.Context, input catalogsearch.Input) (catalogsearch.Output, error) {
	_, environment, err := s.runtime.environment(input.Environment, false)
	if err != nil {
		return catalogsearch.Output{}, capabilitySetupError("catalog.search.setup", "catalog.search", input.Environment, input.Site, "Catalog search setup failed.", "Review the selected environment and catalog configuration.", err)
	}
	input.Environment = environment.Alias
	if input.Site == "" {
		input.Site = environment.SiteContentURL
	}
	input.SiteResolved = true
	store := catalog.NewFileStore(filepath.Dir(s.runtime.configPath), s.runtime.now)
	return catalogsearch.New(catalogSource{store: store}).Execute(ctx, input)
}

type catalogSource struct{ store *catalog.FileStore }

func (s catalogSource) Search(ctx context.Context, input catalogsearch.Input) (catalogsearch.Result, error) {
	result, err := s.store.Search(ctx, catalog.Query{Text: input.Text, Kind: input.Kind, ProjectPath: input.ProjectPath, Owner: input.Owner, Environment: input.Environment, Site: input.Site, SiteSelected: input.SiteResolved, LUID: input.LUID, Cursor: input.Cursor, Limit: input.Limit})
	if err != nil {
		return catalogsearch.Result{}, err
	}
	items := make([]catalogsearch.Item, len(result.Records))
	for index, item := range result.Records {
		items[index] = catalogsearch.Item{LUID: item.LUID, Kind: item.Kind, Name: item.Name, ProjectPath: item.ProjectPath, Owner: item.Owner}
	}
	return catalogsearch.Result{Page: catalogsearch.Page{Returned: result.Page.Returned, Total: result.Page.Total, Limit: result.Page.Limit, NextCursor: result.Page.NextCursor}, GenerationID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Stale: result.Stale, Items: items, Warnings: result.Warnings}, nil
}

type pullService struct{ runtime *runtimeDependencies }

func (s *pullService) Execute(ctx context.Context, input workbookpull.Input) (workbookpull.Output, error) {
	configuration, environment, adapter, err := s.runtime.workbookAdapter(ctx, input.Environment, false)
	if err != nil {
		environmentAlias, site := resolvedTarget(input.Environment, input.Site, environment)
		return workbookpull.Output{}, capabilitySetupError("workbook.pull.setup", "workbook.pull", environmentAlias, site, "Workbook pull setup failed.", "Review the environment, site, and PAT configuration.", err)
	}
	input.Environment, input.Site = environment.Alias, environment.SiteContentURL
	if input.Workspace == "" {
		input.Workspace, err = resolveWorkspace(configuration, environment)
		if err != nil {
			return workbookpull.Output{}, capabilitySetupError("workbook.pull.workspace", "workbook.pull", input.Environment, input.Site, "Workbook workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
		}
	}
	return workbookpull.New(pullReader{adapter: adapter}, artifactWriter{manager: artifact.NewWorkbookManager(s.runtime.now)}).Execute(ctx, input)
}

type pullReader struct{ adapter *resourceworkbook.Adapter }

func (r pullReader) ResolveWorkbook(ctx context.Context, selector identity.Selector) (workbookpull.Workbook, error) {
	item, err := r.adapter.ResolveWorkbook(ctx, selector)
	return workbookpull.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID, ProjectPath: item.ProjectPath}, err
}
func (r pullReader) DownloadWorkbook(ctx context.Context, luid string, include *bool) (workbookpull.Download, error) {
	item, err := r.adapter.DownloadWorkbook(ctx, luid, include)
	return workbookpull.Download{Filename: item.Filename, Content: item.Content, TableauRequestID: item.TableauRequestID}, err
}

type artifactWriter struct{ manager *artifact.WorkbookManager }

func (w artifactWriter) WriteWorkbook(ctx context.Context, input workbookpull.Artifact) (workbookpull.ArtifactResult, error) {
	result, err := w.manager.Pull(ctx, artifact.WorkbookPull{Workspace: input.Workspace, Filename: input.Filename, Content: input.Content, Overwrite: input.Overwrite, Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: input.Name, TableauID: input.TableauID, SourceEnvironment: input.Environment, SourceSite: input.Site, SourceProjectName: input.ProjectName, SourceProjectID: input.ProjectID}})
	return workbookpull.ArtifactResult{Path: result.ArtifactPath, CanonicalPath: result.CanonicalPath, BaselineFingerprint: result.BaselineFingerprint, Warnings: result.Warnings}, err
}

type publishService struct{ runtime *runtimeDependencies }

func (s *publishService) Execute(ctx context.Context, input workbookpublish.Input, apply bool) (workbookpublish.Output, error) {
	_, environment, adapter, err := s.runtime.workbookAdapter(ctx, input.Environment, true)
	if err != nil {
		environmentAlias, site := resolvedTarget(input.Environment, input.Site, environment)
		return workbookpublish.Output{}, capabilitySetupError("workbook.publish.setup", "workbook.publish", environmentAlias, site, "Workbook publish setup failed.", "Review the explicit environment, site, and PAT configuration.", err)
	}
	input.Environment, input.Site, input.TargetResolved = environment.Alias, environment.SiteContentURL, true
	action := workbookpublish.New(artifactReader{manager: artifact.NewWorkbookManager(s.runtime.now)}, publishAdapter{adapter: adapter}, publishAdapter{adapter: adapter})
	return action.Execute(ctx, input, apply)
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
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: operation, Environment: environment, Site: site, Summary: summary, Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
}

type artifactReader struct{ manager *artifact.WorkbookManager }

func (r artifactReader) ReadWorkbook(ctx context.Context, path string) (workbookpublish.Artifact, error) {
	item, err := r.manager.Read(ctx, path)
	return workbookpublish.Artifact{Path: item.Path, PayloadPath: item.PayloadPath, Filename: item.Filename, Size: item.Size, Name: item.Name, TableauID: item.TableauID, Fingerprint: item.Fingerprint}, err
}

type publishAdapter struct{ adapter *resourceworkbook.Adapter }

func (a publishAdapter) ResolveProject(ctx context.Context, selector identity.Selector) (workbookpublish.Project, error) {
	item, err := a.adapter.ResolveProject(ctx, selector)
	return workbookpublish.Project{LUID: item.LUID, Name: item.Name, Path: item.Path}, err
}
func (a publishAdapter) FindWorkbooks(ctx context.Context, name, project string) ([]workbookpublish.Workbook, error) {
	items, err := a.adapter.FindWorkbooks(ctx, name, project)
	result := make([]workbookpublish.Workbook, len(items))
	for index, item := range items {
		result[index] = workbookpublish.Workbook{LUID: item.LUID, Name: item.Name, ProjectLUID: item.ProjectLUID}
	}
	return result, err
}
func (a publishAdapter) Prepare(ctx context.Context, input workbookpublish.PublishRequest) (workbookpublish.PreparedPublish, error) {
	prepared, err := a.adapter.PrepareWorkbook(ctx, tableauworkbook.PublishRequest{Name: input.Name, ProjectLUID: input.ProjectLUID, Filename: input.Filename, ContentPath: input.ContentPath, ContentSize: input.ContentSize, ExpectedFingerprint: input.ExpectedFingerprint, Overwrite: input.Overwrite, AsJob: input.AsJob})
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
	return workbookpublish.Result{Status: result.Status, WorkbookLUID: result.WorkbookLUID, WorkbookName: result.WorkbookName, ProjectLUID: result.ProjectLUID, JobID: result.JobID, TableauRequestID: result.TableauRequestID}, err
}

func resolveWorkspace(configuration config.Config, environment config.Environment) (string, error) {
	current, err := os.Getwd()
	if err == nil {
		for directory := current; ; directory = filepath.Dir(directory) {
			if _, statErr := os.Stat(filepath.Join(directory, config.WorkspaceConfigName)); statErr == nil {
				return directory, nil
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
		}
	}
	if environment.DefaultWorkspace != "" {
		return environment.DefaultWorkspace, nil
	}
	if configuration.DefaultWorkspace != "" {
		return configuration.DefaultWorkspace, nil
	}
	return "", errors.New("no workspace selected or configured")
}

type registrySource struct{}

func (registrySource) List(_ context.Context, includeMutations bool) ([]capabilitylist.Capability, error) {
	definitions := capability.All()
	items := make([]capabilitylist.Capability, 0, len(definitions))
	for _, definition := range definitions {
		if definition.RemoteMutation && !includeMutations {
			continue
		}
		items = append(items, capabilitylist.Capability{
			ID:             definition.ID,
			Owner:          string(definition.Owner),
			Disposition:    string(definition.Disposition),
			State:          string(definition.Implementation),
			Command:        strings.Join(definition.CommandPath, " "),
			Blocked:        definition.Verification == capability.VerificationBlocked,
			Domain:         filterDomain(definition),
			Resource:       filterResource(definition),
			Product:        definition.Availability,
			RemoteMutation: definition.RemoteMutation,
		})
	}
	return items, nil
}

func (registrySource) Get(_ context.Context, id string) (capabilityget.Capability, bool) {
	definition, ok := capability.Lookup(id)
	if !ok {
		return capabilityget.Capability{}, false
	}
	parts := strings.Split(definition.ID, ".")
	domain, resource := classify(definition)
	return capabilityget.Capability{
		ID:                    definition.ID,
		Domain:                domain,
		Resource:              resource,
		Verb:                  parts[len(parts)-1],
		Owner:                 string(definition.Owner),
		Surface:               definition.Surface,
		Outcome:               definition.Outcome,
		OperationType:         string(definition.Type),
		Disposition:           string(definition.Disposition),
		MCPOverlap:            definition.MCPOverlap,
		EvidenceLevel:         string(definition.EvidenceLevel),
		VerificationReadiness: string(definition.Verification),
		ImplementationState:   string(definition.Implementation),
		Command:               strings.Join(definition.CommandPath, " "),
		Selectors:             []string{definition.Selectors},
		Availability:          definition.Availability,
		SafetyGuard:           definition.SafetyGuard,
		ArtifactEffect:        definition.ArtifactEffect,
		UpstreamOperation:     definition.Upstream,
		Evidence:              definition.Evidence,
		Validation:            definition.Validation,
		Blocker:               string(definition.Blocker),
		RemoteMutation:        definition.RemoteMutation,
		RequiresApply:         definition.RequiresApply,
		LocalWrite:            definition.LocalWrite,
		RawCapable:            definition.RawCapable,
	}, true
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

func filterDomain(definition capability.Definition) string {
	domain, _ := classify(definition)
	return domain
}

func filterResource(definition capability.Definition) string {
	_, resource := classify(definition)
	return resource
}

func classify(definition capability.Definition) (string, string) {
	parts := strings.Split(definition.ID, ".")
	if definition.Owner == capability.OwnerCLI && (parts[0] == "workbook" || parts[0] == "datasource" || parts[0] == "flow" || parts[0] == "project") {
		return "content", parts[0]
	}
	if len(parts) == 3 {
		return parts[0], parts[1]
	}
	return parts[0], ""
}

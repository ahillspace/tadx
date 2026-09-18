package app

import (
	"context"
	"os"
	"sync"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	resourceproject "github.com/ahillspace/tadx/internal/resources/project"
	"github.com/ahillspace/tadx/internal/tableau"
	tableauauth "github.com/ahillspace/tadx/internal/tableau/auth"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
	tableauflow "github.com/ahillspace/tadx/internal/tableau/flow"
	tableaumetadata "github.com/ahillspace/tadx/internal/tableau/metadata"
	"github.com/ahillspace/tadx/internal/tableau/metadataassets"
	tableauproject "github.com/ahillspace/tadx/internal/tableau/project"
	tableauworkbook "github.com/ahillspace/tadx/internal/tableau/workbook"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// commandRuntime lives inside one Run. It contains setup snapshots, never
// mutable remote inventory or mutation validation results.
type commandRuntime struct {
	mu                sync.Mutex
	configurationOnce sync.Once
	configuration     config.Config
	configurationErr  error
	sessions          *coreauth.CommandSessions
	transports        map[string]*tableau.Transport
	clients           map[clientKey]tableauClients
	workspaces        map[workspaceKey]workspaceResult
	discovery         map[clientKey]*commandDiscoveryPaths
	jobMonitoring     map[string]bool
}

type clientKey struct {
	session   coreauth.Session
	transport *tableau.Transport
}
type tableauClients struct {
	metadataAssets *metadataassets.Client
	workbooks      *tableauworkbook.Client
	datasources    *tableaudatasource.Client
	projects       *tableauproject.Client
	flows          *tableauflow.RESTClient
	metadata       *tableaumetadata.Client
}
type workspaceKey struct{ selector, environmentDefault string }
type workspaceResult struct {
	record workspacecore.Record
	err    error
}

type commandDiscoveryPaths struct {
	paths *resourceproject.DiscoveryPaths
	fresh *resourceproject.Adapter
}

func (p *commandDiscoveryPaths) ResolveProjectPath(ctx context.Context, luid string) (string, error) {
	paths, err := p.paths.ResolveProjectPaths(ctx, []string{luid})
	return paths[luid], err
}
func (p *commandDiscoveryPaths) ResolveProjectPaths(ctx context.Context, luids []string) (map[string]string, error) {
	return p.paths.ResolveProjectPaths(ctx, luids)
}
func (p *commandDiscoveryPaths) ValidateProjectPath(ctx context.Context, path string) error {
	return p.fresh.ValidateProjectPath(ctx, path)
}
func (p *commandDiscoveryPaths) ResolveProjectSelectorPath(ctx context.Context, path string) (string, error) {
	return p.fresh.ResolveProjectSelectorPath(ctx, path)
}

func (r *runtimeDependencies) discoveryPaths(connection authenticatedTableau) *commandDiscoveryPaths {
	clients := r.clients(connection)
	r.command.mu.Lock()
	defer r.command.mu.Unlock()
	if r.command.discovery == nil {
		r.command.discovery = make(map[clientKey]*commandDiscoveryPaths)
	}
	key := clientKey{connection.session, connection.transport}
	if r.command.discovery[key] == nil {
		fresh := resourceproject.NewAdapter(clients.projects)
		r.command.discovery[key] = &commandDiscoveryPaths{paths: resourceproject.NewDiscoveryPaths(fresh), fresh: fresh}
	}
	return r.command.discovery[key]
}

func (r *runtimeDependencies) configuration() (config.Config, error) {
	r.command.configurationOnce.Do(func() {
		r.command.configuration, r.command.configurationErr = config.Load(r.configPath)
		if r.command.configurationErr != nil {
			r.command.configurationErr = &errs.Error{ID: "configuration.load", Kind: errs.KindOperation, Operation: "configuration", Summary: "CLI settings could not be loaded.", Cause: r.command.configurationErr, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted, Retryable: errs.Bool(false)}
		}
	})
	return r.command.configuration, r.command.configurationErr
}

func (r *runtimeDependencies) commandSessions() *coreauth.CommandSessions {
	r.command.mu.Lock()
	defer r.command.mu.Unlock()
	if r.command.sessions == nil {
		r.command.sessions = coreauth.NewCommandSessions(coreauth.LookupEnvFunc(os.LookupEnv), r.patStore, "")
	}
	return r.command.sessions
}

func (r *runtimeDependencies) transport(version string) *tableau.Transport {
	r.command.mu.Lock()
	defer r.command.mu.Unlock()
	if r.command.transports == nil {
		r.command.transports = make(map[string]*tableau.Transport)
	}
	if r.command.transports[version] == nil {
		r.command.transports[version] = tableau.NewTransport(r.httpClient, version, func() string { return r.correlationID })
	}
	return r.command.transports[version]
}

func (r *runtimeDependencies) authenticate(ctx context.Context, target coreauth.Target, version string) (coreauth.Session, error) {
	return r.commandSessions().Authenticate(ctx, target, tableauauth.NewClient(r.transport(version)))
}

func (r *runtimeDependencies) clients(connection authenticatedTableau) tableauClients {
	r.command.mu.Lock()
	defer r.command.mu.Unlock()
	if r.command.clients == nil {
		r.command.clients = make(map[clientKey]tableauClients)
	}
	key := clientKey{connection.session, connection.transport}
	if existing, ok := r.command.clients[key]; ok {
		return existing
	}
	transport, session, server := connection.transport, connection.session, connection.environment.URL
	result := tableauClients{
		metadataAssets: metadataassets.NewClient(transport, session, server),
		workbooks:      tableauworkbook.NewClient(transport, session, server),
		datasources:    tableaudatasource.NewClient(transport, session, server),
		projects:       tableauproject.NewClient(transport, session, server),
		flows:          tableauflow.NewClient(transport, session, server),
		metadata:       tableaumetadata.NewClient(transport, session, server),
	}
	r.command.clients[key] = result
	return result
}

func (r *runtimeDependencies) resolveWorkspace(ctx context.Context, configuration config.Config, selector, environmentDefault string) (workspacecore.Record, error) {
	if err := ctx.Err(); err != nil {
		return workspacecore.Record{}, err
	}
	r.command.mu.Lock()
	defer r.command.mu.Unlock()
	if r.command.workspaces == nil {
		r.command.workspaces = make(map[workspaceKey]workspaceResult)
	}
	key := workspaceKey{selector, environmentDefault}
	if previous, ok := r.command.workspaces[key]; ok {
		return previous.record, previous.err
	}
	record, err := workspacecore.NewManager(r.configPath, nil).ResolveReadOnlyWithConfig(ctx, configuration, selector, environmentDefault)
	r.command.workspaces[key] = workspaceResult{record, err}
	return record, err
}

// Close ends the command session lifetime. Direct composition tests must call it
// after all requests, just as Run does on every exit path.
func (r *runtimeDependencies) Close() error {
	r.command.mu.Lock()
	sessions := r.command.sessions
	r.command.clients, r.command.transports, r.command.workspaces = nil, nil, nil
	r.command.discovery = nil
	r.command.mu.Unlock()
	return sessions.Close()
}

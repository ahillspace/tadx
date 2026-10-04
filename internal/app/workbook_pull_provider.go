package app

import (
	"context"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
	resourcelineage "github.com/ahillspace/tadx/internal/resources/lineage"
	resourceworkbook "github.com/ahillspace/tadx/internal/resources/workbook"
)

type workbookPullProvider struct{ runtime *runtimeDependencies }

func (p workbookPullProvider) ResolveWorkbookWorkspace(ctx context.Context, selector, environment, site string) (workbook.PullWorkspace, error) {
	workspace, err := (&workspaceRuntime{runtime: p.runtime}).resolveForEnvironment(ctx, selector, environment)
	if err != nil {
		return workbook.PullWorkspace{}, capabilitySetupError("workbook.pull.workspace", "workbook.pull", environment, site, "Workbook workspace resolution failed.", "Select or configure an exact workspace, then retry.", err)
	}
	return workbook.PullWorkspace{Root: workspace.Root, Name: workspace.Name}, nil
}

func (p workbookPullProvider) OpenWorkbookPull(ctx context.Context, alias, site string) (workbook.PullSession, error) {
	connection, err := p.runtime.tableauConnection(ctx, alias, false)
	environment := connection.environment
	if err != nil {
		environmentAlias, resolvedSite := resolvedTarget(alias, site, environment)
		return workbook.PullSession{}, capabilitySetupError("workbook.pull.setup", "workbook.pull", environmentAlias, resolvedSite, "Workbook pull setup failed.", "Review the environment, site, and PAT configuration.", err)
	}
	clients := p.runtime.clients(connection)
	workbooks := resourceworkbook.NewAdapterWithProjectResolver(clients.workbooks, p.runtime.discoveryPaths(connection))
	origin, err := artifact.NormalizeServerOrigin(environment.URL)
	if err != nil {
		return workbook.PullSession{}, capabilitySetupError("workbook.pull.source", "workbook.pull", environment.Alias, environment.SiteContentURL, "Workbook source identity resolution failed.", "Review the configured Tableau server URL, then retry.", err)
	}
	mark := func(ctx context.Context, label string) { progress.SetLabel(ctx, label) }
	reader := resourceworkbook.PullReaderPort{Workbooks: workbooks, References: resourceworkbook.NewReferenceAdapter(clients.metadata), Datasources: resourcedatasource.NewAdapter(clients.datasources), Lineage: resourcelineage.NewAdapter(clients.metadata), Progress: mark}
	writer := resourceworkbook.PullWriterPort{Workbooks: artifact.NewWorkbookManager(p.runtime.now), Bundles: artifact.NewWorkbookBundleManager(p.runtime.now), Progress: mark}
	return workbook.PullSession{Environment: environment.Alias, Site: environment.SiteContentURL, SiteLUID: connection.session.SiteLUID(), ServerOrigin: origin, Reader: reader, Writer: writer, Previewer: writer}, nil
}

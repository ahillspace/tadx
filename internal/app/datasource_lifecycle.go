package app

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/cli/progress"
	"github.com/ahillspace/tadx/internal/contentbatch"
	resourcedatasource "github.com/ahillspace/tadx/internal/resources/datasource"
)

type datasourcePublishProvider struct{ commands *remoteContentCommands }

func (p datasourcePublishProvider) OpenDatasourcePublishSource(ctx context.Context, input datasourceops.PublishInput) (datasourceops.PublishInput, datasourceops.ArtifactReader, string, error) {
	source := resourcedatasource.PublishSource{Manager: artifact.NewDatasourceManager(p.commands.runtime.now), Workspace: func(ctx context.Context, selector, alias string) (string, string, error) {
		workspace, err := (&workspaceRuntime{runtime: p.commands.runtime}).resolveForEnvironment(ctx, selector, alias)
		return workspace.Name, workspace.Root, err
	}}
	return source.Open(ctx, input)
}

func (p datasourcePublishProvider) OpenDatasourcePublish(ctx context.Context, input datasourceops.PublishInput, sourcePath string) (datasourceops.PublishSession, error) {
	connection, err := p.commands.connect(ctx, input.Environment, true)
	if err != nil {
		return datasourceops.PublishSession{}, remoteSetupError("datasource.publish", input.Environment, input.Site, connection.environment, err)
	}
	var lifecycle *resourcedatasource.PublicationLifecycle
	ports := resourcedatasource.PublishPorts{Datasources: connection.datasources, Projects: connection.projects, Changes: connection.datasourceChanges,
		Fresh: func(ctx context.Context) (*resourcedatasource.Adapter, error) {
			fresh, err := newRemoteContentCommands(p.commands.runtime).connect(ctx, connection.environment.Alias, true)
			return fresh.datasources, err
		},
		Begin: func(ctx context.Context, request datasourceops.PublishRequest) (resourcedatasource.PublishAcceptance, error) {
			monitor, asJob, err := p.commands.runtime.publication(ctx, connection.environment.Alias, "datasource", sourcePath, request.ProjectLUID, request.Name)
			if err != nil {
				return resourcedatasource.PublishAcceptance{}, err
			}
			readback := resourcedatasource.FreshPublicationDestination{
				Name: request.Name, ProjectID: request.ProjectLUID,
				Open: func(ctx context.Context) (*resourcedatasource.Adapter, error) {
					fresh, err := newRemoteContentCommands(p.commands.runtime).connect(ctx, monitor.Base.Environment, true)
					if err != nil {
						return nil, err
					}
					return fresh.datasources, nil
				},
				Progress: func(ctx context.Context) { progress.SetLabel(ctx, "Confirming published content") },
			}
			lifecycle = &resourcedatasource.PublicationLifecycle{Publication: monitor, Readback: readback}
			return resourcedatasource.NewPublishAcceptance(monitor, asJob, connection.environment.Alias, connection.environment.SiteContentURL, readback, contentbatch.Bulk), nil
		},
		Progress: func(ctx context.Context, label string) { progress.SetLabel(ctx, label) },
	}
	return datasourceops.PublishSession{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Resolver: ports, Preparer: ports,
		Lifecycle: func() datasourceops.PublishLifecycle {
			if lifecycle == nil {
				return nil
			}
			return lifecycle
		},
		NoWait: p.commands.runtime.publicationExecution.StopRequested, Defer: func(ctx context.Context, finish func(context.Context) (any, error)) {
			contentbatch.DeferCompletion(ctx, finish)
		},
	}, nil
}

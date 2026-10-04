package app

import (
	"context"
	"time"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/resources/datasource"
	"github.com/ahillspace/tadx/internal/tableau/fieldcatalog"
)

type datasourceSchemaProvider struct{ commands *remoteContentCommands }

func (p datasourceSchemaProvider) CursorTarget(alias string) (datasourceops.SchemaTarget, error) {
	_, environment, err := p.commands.runtime.environment(alias, false)
	return datasourceops.SchemaTarget{Environment: environment.Alias, Site: environment.SiteContentURL}, err
}

func (p datasourceSchemaProvider) CacheTarget(alias string) (datasourceops.SchemaTarget, error) {
	environment, site, err := p.commands.resolveCacheTarget(alias)
	if err != nil {
		err = capabilitySetupError("datasource.schema.cache.setup", "datasource.schema", alias, "", "Cache datasource schema setup failed.", "Verify the selected environment and cache configuration.", err)
	}
	return datasourceops.SchemaTarget{Environment: environment, Site: site}, err
}

func (p datasourceSchemaProvider) CachedSchema(target datasourceops.SchemaTarget, metadata bool) datasourceops.CachedSchemaReader {
	return &datasource.CachedSchemaPort{Store: p.commands.cacheStore(target.Environment), Environment: target.Environment, Site: target.Site, Metadata: metadata, ReadError: cacheReadError}
}

func (p datasourceSchemaProvider) OpenDatasourceSchema(ctx context.Context, alias string, metadata bool) (datasourceops.SchemaSession, error) {
	connection, err := p.commands.runtime.tableauConnection(ctx, alias, false)
	if err != nil {
		return datasourceops.SchemaSession{}, capabilitySetupError("datasource.schema.setup", "datasource.schema", alias, connection.environment.SiteContentURL, "Datasource schema setup failed.", "Verify the selected environment, PAT variables, and Tableau connectivity.", err)
	}
	clients := p.commands.runtime.clients(connection)
	reader := datasource.SchemaReadPort{Adapter: datasource.NewSchemaAdapter(clients.datasources, fieldcatalog.NewClient(connection.transport, connection.session, connection.environment.URL)), Now: p.commands.runtime.now}
	if metadata {
		reader.Metadata = clients.metadataAssets
	}
	publisher := datasource.SchemaPublisherPort{Store: func() datasource.SchemaCacheWriter { return p.commands.runtime.cacheStore(connection.environment) }, Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL, Now: p.commands.runtime.now}
	return datasourceops.SchemaSession{SchemaTarget: datasourceops.SchemaTarget{Environment: connection.environment.Alias, Site: connection.environment.SiteContentURL}, Reader: reader, Publisher: publisher}, nil
}

func (p datasourceSchemaProvider) Now() time.Time { return p.commands.runtime.now() }

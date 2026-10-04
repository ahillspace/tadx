package app

import (
	"context"

	"github.com/ahillspace/tadx/actions/catalog"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
)

// catalogCommands wires the target provider; the catalog service owns each operation.
type catalogCommands struct{ runtime *runtimeDependencies }

func (c *catalogCommands) dependencies() *catalogcli.Dependencies {
	service := catalog.New(catalogProvider{runtime: c.runtime})
	return &catalogcli.Dependencies{
		DatabaseLister: service, DatabaseInspector: service, DatabaseUpdater: service,
		TableLister: service, TableInspector: service, TableUpdater: service,
		ColumnLister: service, ColumnInspector: service, ColumnUpdater: service,
		Searcher: service, Auditor: service,
	}
}

type catalogProvider struct{ runtime *runtimeDependencies }

func (p catalogProvider) Open(ctx context.Context, operation, environment string, explicit bool) (catalog.Target, error) {
	connection, err := p.runtime.tableauConnection(ctx, environment, explicit)
	if err != nil {
		return catalog.Target{}, capabilitySetupError(operation+".setup", operation, environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	return catalog.Target{
		Environment: connection.environment.Alias,
		Site:        connection.environment.SiteContentURL,
		Assets:      p.runtime.clients(connection).metadataAssets,
	}, nil
}

package app

import (
	"context"

	catalogaudit "github.com/ahillspace/tadx/actions/catalog/audit"
	catalogread "github.com/ahillspace/tadx/actions/catalog/read"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	catalogupdate "github.com/ahillspace/tadx/actions/catalog/update"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	resourcecatalog "github.com/ahillspace/tadx/internal/resources/catalog"
)

type catalogCommands struct{ runtime *runtimeDependencies }

func (c *catalogCommands) dependencies() *catalogcli.Dependencies {
	return &catalogcli.Dependencies{DatabaseLister: c, DatabaseInspector: c, DatabaseUpdater: c, TableLister: c, TableInspector: c, TableUpdater: c, ColumnLister: c, ColumnInspector: c, ColumnUpdater: c, Searcher: c, Auditor: c}
}

func (c *catalogCommands) ListCatalogDatabases(ctx context.Context, input catalogread.DatabaseListInput) (catalogread.DatabaseListOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.database.list", input.Environment, false)
	if err != nil {
		return catalogread.DatabaseListOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.ListDatabases(ctx, adapter, input)
}

func (c *catalogCommands) InspectCatalogDatabase(ctx context.Context, input catalogread.DatabaseInspectInput) (catalogread.DatabaseInspectOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.database.inspect", input.Environment, false)
	if err != nil {
		return catalogread.DatabaseInspectOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.InspectDatabase(ctx, adapter, input)
}

func (c *catalogCommands) UpdateCatalogDatabase(ctx context.Context, input catalogupdate.DatabaseInput, preview bool) (catalogupdate.DatabaseOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.database.update", input.Environment, true)
	if err != nil {
		return catalogupdate.DatabaseOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	return catalogupdate.NewDatabase(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) ListCatalogTables(ctx context.Context, input catalogread.TableListInput) (catalogread.TableListOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.table.list", input.Environment, false)
	if err != nil {
		return catalogread.TableListOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.ListTables(ctx, adapter, input)
}

func (c *catalogCommands) InspectCatalogTable(ctx context.Context, input catalogread.TableInspectInput) (catalogread.TableInspectOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.table.inspect", input.Environment, false)
	if err != nil {
		return catalogread.TableInspectOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.InspectTable(ctx, adapter, input)
}

func (c *catalogCommands) UpdateCatalogTable(ctx context.Context, input catalogupdate.TableInput, preview bool) (catalogupdate.TableOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.table.update", input.Environment, true)
	if err != nil {
		return catalogupdate.TableOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	return catalogupdate.NewTable(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) ListCatalogColumns(ctx context.Context, input catalogread.ColumnListInput) (catalogread.ColumnListOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.column.list", input.Environment, false)
	if err != nil {
		return catalogread.ColumnListOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.ListColumns(ctx, adapter, input)
}

func (c *catalogCommands) InspectCatalogColumn(ctx context.Context, input catalogread.ColumnInspectInput) (catalogread.ColumnInspectOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.column.inspect", input.Environment, false)
	if err != nil {
		return catalogread.ColumnInspectOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogread.InspectColumn(ctx, adapter, input)
}

func (c *catalogCommands) UpdateCatalogColumn(ctx context.Context, input catalogupdate.ColumnInput, preview bool) (catalogupdate.ColumnOutput, error) {
	connection, adapter, err := c.setup(ctx, "catalog.column.update", input.Environment, true)
	if err != nil {
		return catalogupdate.ColumnOutput{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	return catalogupdate.NewColumn(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) SearchCatalog(ctx context.Context, input catalogsearch.Input) (catalogsearch.Output, error) {
	connection, adapter, err := c.setup(ctx, "catalog.search", input.Environment, false)
	if err != nil {
		return catalogsearch.Output{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogsearch.Execute(ctx, adapter, input)
}

func (c *catalogCommands) AuditCatalog(ctx context.Context, input catalogaudit.Input) (catalogaudit.Output, error) {
	connection, adapter, err := c.setup(ctx, "catalog.audit", input.Environment, false)
	if err != nil {
		return catalogaudit.Output{}, err
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	return catalogaudit.Execute(ctx, adapter, input)
}

func (c *catalogCommands) setup(ctx context.Context, operation, environment string, explicit bool) (authenticatedTableau, *resourcecatalog.Adapter, error) {
	connection, err := c.runtime.tableauConnection(ctx, environment, explicit)
	if err != nil {
		return connection, nil, capabilitySetupError(operation+".setup", operation, environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	return connection, resourcecatalog.New(c.runtime.clients(connection).metadataAssets), nil
}

package app

import (
	"context"
	catalogaudit "github.com/ahillspace/tadx/actions/catalog/audit"
	columninspect "github.com/ahillspace/tadx/actions/catalog/column/inspect"
	columnlist "github.com/ahillspace/tadx/actions/catalog/column/list"
	columnupdate "github.com/ahillspace/tadx/actions/catalog/column/update"
	databaseinspect "github.com/ahillspace/tadx/actions/catalog/database/inspect"
	databaselist "github.com/ahillspace/tadx/actions/catalog/database/list"
	databaseupdate "github.com/ahillspace/tadx/actions/catalog/database/update"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	tableinspect "github.com/ahillspace/tadx/actions/catalog/table/inspect"
	tablelist "github.com/ahillspace/tadx/actions/catalog/table/list"
	tableupdate "github.com/ahillspace/tadx/actions/catalog/table/update"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	resourcecatalog "github.com/ahillspace/tadx/internal/resources/catalog"
)

type catalogCommands struct{ runtime *runtimeDependencies }

func (c *catalogCommands) dependencies() *catalogcli.Dependencies {
	return &catalogcli.Dependencies{DatabaseLister: c, DatabaseInspector: c, DatabaseUpdater: c, TableLister: c, TableInspector: c, TableUpdater: c, ColumnLister: c, ColumnInspector: c, ColumnUpdater: c, Searcher: c, Auditor: c}
}

func (c *catalogCommands) ListCatalogDatabases(ctx context.Context, input databaselist.Input) (databaselist.Output, error) {
	if err := databaselist.ValidateInput(input); err != nil {
		return databaselist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return databaselist.Output{}, capabilitySetupError("catalog.database.list.setup", "catalog.database.list", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return databaselist.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) InspectCatalogDatabase(ctx context.Context, input databaseinspect.Input) (databaseinspect.Output, error) {
	if err := databaseinspect.ValidateInput(input); err != nil {
		return databaseinspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return databaseinspect.Output{}, capabilitySetupError("catalog.database.inspect.setup", "catalog.database.inspect", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return databaseinspect.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) UpdateCatalogDatabase(ctx context.Context, input databaseupdate.Input, preview bool) (databaseupdate.Output, error) {
	if err := databaseupdate.ValidateInput(input); err != nil {
		return databaseupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return databaseupdate.Output{}, capabilitySetupError("catalog.database.update.setup", "catalog.database.update", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return databaseupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) ListCatalogTables(ctx context.Context, input tablelist.Input) (tablelist.Output, error) {
	if err := tablelist.ValidateInput(input); err != nil {
		return tablelist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return tablelist.Output{}, capabilitySetupError("catalog.table.list.setup", "catalog.table.list", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return tablelist.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) InspectCatalogTable(ctx context.Context, input tableinspect.Input) (tableinspect.Output, error) {
	if err := tableinspect.ValidateInput(input); err != nil {
		return tableinspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return tableinspect.Output{}, capabilitySetupError("catalog.table.inspect.setup", "catalog.table.inspect", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return tableinspect.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) UpdateCatalogTable(ctx context.Context, input tableupdate.Input, preview bool) (tableupdate.Output, error) {
	if err := tableupdate.ValidateInput(input); err != nil {
		return tableupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return tableupdate.Output{}, capabilitySetupError("catalog.table.update.setup", "catalog.table.update", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return tableupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) ListCatalogColumns(ctx context.Context, input columnlist.Input) (columnlist.Output, error) {
	if err := columnlist.ValidateInput(input); err != nil {
		return columnlist.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return columnlist.Output{}, capabilitySetupError("catalog.column.list.setup", "catalog.column.list", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return columnlist.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) InspectCatalogColumn(ctx context.Context, input columninspect.Input) (columninspect.Output, error) {
	if err := columninspect.ValidateInput(input); err != nil {
		return columninspect.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return columninspect.Output{}, capabilitySetupError("catalog.column.inspect.setup", "catalog.column.inspect", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return columninspect.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) UpdateCatalogColumn(ctx context.Context, input columnupdate.Input, preview bool) (columnupdate.Output, error) {
	if err := columnupdate.ValidateInput(input); err != nil {
		return columnupdate.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, true)
	if err != nil {
		return columnupdate.Output{}, capabilitySetupError("catalog.column.update.setup", "catalog.column.update", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL
	input.TargetResolved = true
	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return columnupdate.New(adapter, adapter).Execute(ctx, input, preview)
}

func (c *catalogCommands) SearchCatalog(ctx context.Context, input catalogsearch.Input) (catalogsearch.Output, error) {
	if err := catalogsearch.ValidateInput(input); err != nil {
		return catalogsearch.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return catalogsearch.Output{}, capabilitySetupError("catalog.search.setup", "catalog.search", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return catalogsearch.New(adapter).Execute(ctx, input)
}

func (c *catalogCommands) AuditCatalog(ctx context.Context, input catalogaudit.Input) (catalogaudit.Output, error) {
	if err := catalogaudit.ValidateInput(input); err != nil {
		return catalogaudit.Output{}, err
	}
	connection, err := c.runtime.tableauConnection(ctx, input.Environment, false)
	if err != nil {
		return catalogaudit.Output{}, capabilitySetupError("catalog.audit.setup", "catalog.audit", input.Environment, connection.environment.SiteContentURL, "Catalog operation setup failed.", "Verify the environment and Tableau metadata access.", err)
	}
	input.Environment, input.Site = connection.environment.Alias, connection.environment.SiteContentURL

	adapter := resourcecatalog.New(c.runtime.clients(connection).metadataAssets)
	return catalogaudit.New(adapter).Execute(ctx, input)
}

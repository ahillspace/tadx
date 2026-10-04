package catalog

import (
	"context"

	"github.com/ahillspace/tadx/internal/value"
)

type catalogTestProvider struct{ assets Assets }

func (p catalogTestProvider) Open(_ context.Context, _ string, environment string, _ bool) (Target, error) {
	return Target{Environment: environment, Assets: p.assets}, nil
}

type catalogTestAssets struct {
	Assets
	databaseList    DatabaseListReader
	databaseInspect DatabaseInspectReader
	tableList       TableListReader
	tableInspect    TableInspectReader
	columnList      ColumnListReader
	columnInspect   ColumnInspectReader
	search          SearchReader
	audit           AuditReader
}

func (a catalogTestAssets) DiscoverDatabases(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	if a.search != nil {
		return a.search.DiscoverDatabases(ctx, q)
	}
	if a.databaseInspect != nil {
		return a.databaseInspect.DiscoverDatabases(ctx, q)
	}
	return a.databaseList.DiscoverDatabases(ctx, q)
}
func (a catalogTestAssets) GetDatabase(ctx context.Context, id string) (value.MetadataDatabase, error) {
	if a.audit != nil {
		return a.audit.GetDatabase(ctx, id)
	}
	return a.databaseInspect.GetDatabase(ctx, id)
}
func (a catalogTestAssets) DiscoverTables(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataTable], error) {
	if a.search != nil {
		return a.search.DiscoverTables(ctx, q)
	}
	if a.audit != nil {
		return a.audit.DiscoverTables(ctx, q)
	}
	if a.tableInspect != nil {
		return a.tableInspect.DiscoverTables(ctx, q)
	}
	return a.tableList.DiscoverTables(ctx, q)
}
func (a catalogTestAssets) GetTable(ctx context.Context, id string) (value.MetadataTable, error) {
	if a.audit != nil {
		return a.audit.GetTable(ctx, id)
	}
	return a.tableInspect.GetTable(ctx, id)
}
func (a catalogTestAssets) DiscoverColumns(ctx context.Context, q value.MetadataQuery) (value.MetadataPage[value.MetadataColumn], error) {
	if a.search != nil {
		return a.search.DiscoverColumns(ctx, q)
	}
	if a.audit != nil {
		return a.audit.DiscoverColumns(ctx, q)
	}
	if a.columnInspect != nil {
		return a.columnInspect.DiscoverColumns(ctx, q)
	}
	return a.columnList.DiscoverColumns(ctx, q)
}
func (a catalogTestAssets) GetColumn(ctx context.Context, table, id string) (value.MetadataColumn, error) {
	return a.columnInspect.GetColumn(ctx, table, id)
}
func (a catalogTestAssets) DatasourceFieldDescriptions(ctx context.Context, id string) (value.MetadataDatasourceDescriptions, error) {
	return a.audit.DatasourceFieldDescriptions(ctx, id)
}

func catalogTestService(assets Assets) *Service {
	return New(catalogTestProvider{assets: assets})
}
func databaseListService(r DatabaseListReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{databaseList: r})
}
func databaseInspectService(r DatabaseInspectReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{databaseInspect: r})
}
func tableListService(r TableListReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{tableList: r})
}
func tableInspectService(r TableInspectReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{tableInspect: r})
}
func columnListService(r ColumnListReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{columnList: r})
}
func columnInspectService(r ColumnInspectReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{columnInspect: r})
}
func searchService(r SearchReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{search: r})
}
func auditService(r AuditReader) *Service {
	if r == nil {
		return catalogTestService(nil)
	}
	return catalogTestService(catalogTestAssets{audit: r})
}

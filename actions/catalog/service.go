package catalog

import "context"

// Assets is the metadata capability set returned for one resolved Tableau target.
type Assets interface {
	DatabaseInspectReader
	TableInspectReader
	ColumnInspectReader
	SearchReader
	AuditReader
	DatabaseWriter
	TableWriter
	ColumnWriter
}

// Target binds metadata capabilities to the canonical selected environment and site.
type Target struct {
	Environment string
	Site        string
	Assets      Assets
}

// Provider opens a target only after local operation validation succeeds.
type Provider interface {
	Open(context.Context, string, string, bool) (Target, error)
}

type Service struct{ provider Provider }

func New(provider Provider) *Service { return &Service{provider: provider} }

func (s *Service) ListCatalogDatabases(ctx context.Context, in DatabaseListInput) (DatabaseListOutput, error) {
	if err := ValidateDatabaseListInput(in); err != nil {
		return DatabaseListOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.database.list", in.Environment, false)
	if err != nil {
		return DatabaseListOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return listDatabasesValidated(ctx, target.Assets, in)
}

func (s *Service) InspectCatalogDatabase(ctx context.Context, in DatabaseInspectInput) (DatabaseInspectOutput, error) {
	if err := ValidateDatabaseInspectInput(in); err != nil {
		return DatabaseInspectOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.database.inspect", in.Environment, false)
	if err != nil {
		return DatabaseInspectOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return inspectDatabaseValidated(ctx, target.Assets, in)
}

func (s *Service) UpdateCatalogDatabase(ctx context.Context, in DatabaseInput, preview bool) (DatabaseOutput, error) {
	if err := ValidateDatabaseInput(in); err != nil {
		return DatabaseOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.database.update", in.Environment, true)
	if err != nil {
		return DatabaseOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	in.TargetResolved = true
	return (&databaseUpdate{reader: target.Assets, writer: target.Assets}).executeValidated(ctx, in, preview)
}

func (s *Service) ListCatalogTables(ctx context.Context, in TableListInput) (TableListOutput, error) {
	if err := ValidateTableListInput(in); err != nil {
		return TableListOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.table.list", in.Environment, false)
	if err != nil {
		return TableListOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return listTablesValidated(ctx, target.Assets, in)
}

func (s *Service) InspectCatalogTable(ctx context.Context, in TableInspectInput) (TableInspectOutput, error) {
	if err := ValidateTableInspectInput(in); err != nil {
		return TableInspectOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.table.inspect", in.Environment, false)
	if err != nil {
		return TableInspectOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return inspectTableValidated(ctx, target.Assets, in)
}

func (s *Service) UpdateCatalogTable(ctx context.Context, in TableInput, preview bool) (TableOutput, error) {
	if err := ValidateTableInput(in); err != nil {
		return TableOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.table.update", in.Environment, true)
	if err != nil {
		return TableOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	in.TargetResolved = true
	return (&tableUpdate{reader: target.Assets, writer: target.Assets}).executeValidated(ctx, in, preview)
}

func (s *Service) ListCatalogColumns(ctx context.Context, in ColumnListInput) (ColumnListOutput, error) {
	if err := ValidateColumnListInput(in); err != nil {
		return ColumnListOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.column.list", in.Environment, false)
	if err != nil {
		return ColumnListOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return listColumnsValidated(ctx, target.Assets, in)
}

func (s *Service) InspectCatalogColumn(ctx context.Context, in ColumnInspectInput) (ColumnInspectOutput, error) {
	if err := ValidateColumnInspectInput(in); err != nil {
		return ColumnInspectOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.column.inspect", in.Environment, false)
	if err != nil {
		return ColumnInspectOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return inspectColumnValidated(ctx, target.Assets, in)
}

func (s *Service) UpdateCatalogColumn(ctx context.Context, in ColumnInput, preview bool) (ColumnOutput, error) {
	if err := ValidateColumnInput(in); err != nil {
		return ColumnOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.column.update", in.Environment, true)
	if err != nil {
		return ColumnOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	in.TargetResolved = true
	return (&columnUpdate{reader: target.Assets, writer: target.Assets}).executeValidated(ctx, in, preview)
}

func (s *Service) SearchCatalog(ctx context.Context, in SearchInput) (SearchOutput, error) {
	if err := ValidateSearchInput(in); err != nil {
		return SearchOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.search", in.Environment, false)
	if err != nil {
		return SearchOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return searchCatalogValidated(ctx, target.Assets, in)
}

func (s *Service) AuditCatalog(ctx context.Context, in AuditInput) (AuditOutput, error) {
	if err := ValidateAuditInput(in); err != nil {
		return AuditOutput{}, err
	}
	target, err := s.provider.Open(ctx, "catalog.audit", in.Environment, false)
	if err != nil {
		return AuditOutput{}, err
	}
	in.Environment, in.Site = target.Environment, target.Site
	return auditCatalogValidated(ctx, target.Assets, in)
}

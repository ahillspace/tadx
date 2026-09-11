// Package catalog contains thin command plumbing for upstream metadata.
package catalog

import (
	"context"
	"errors"
	columninspect "github.com/ahillspace/tadx/actions/catalog/column/inspect"
	columnlist "github.com/ahillspace/tadx/actions/catalog/column/list"
	columnupdate "github.com/ahillspace/tadx/actions/catalog/column/update"
	databaseinspect "github.com/ahillspace/tadx/actions/catalog/database/inspect"
	databaselist "github.com/ahillspace/tadx/actions/catalog/database/list"
	databaseupdate "github.com/ahillspace/tadx/actions/catalog/database/update"
	tableinspect "github.com/ahillspace/tadx/actions/catalog/table/inspect"
	tablelist "github.com/ahillspace/tadx/actions/catalog/table/list"
	tableupdate "github.com/ahillspace/tadx/actions/catalog/table/update"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"

	catalogaudit "github.com/ahillspace/tadx/actions/catalog/audit"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
)

type DatabaseLister interface {
	ListCatalogDatabases(context.Context, databaselist.Input) (databaselist.Output, error)
}
type DatabaseInspector interface {
	InspectCatalogDatabase(context.Context, databaseinspect.Input) (databaseinspect.Output, error)
}
type DatabaseUpdater interface {
	UpdateCatalogDatabase(context.Context, databaseupdate.Input, bool) (databaseupdate.Output, error)
}
type TableLister interface {
	ListCatalogTables(context.Context, tablelist.Input) (tablelist.Output, error)
}
type TableInspector interface {
	InspectCatalogTable(context.Context, tableinspect.Input) (tableinspect.Output, error)
}
type TableUpdater interface {
	UpdateCatalogTable(context.Context, tableupdate.Input, bool) (tableupdate.Output, error)
}
type ColumnLister interface {
	ListCatalogColumns(context.Context, columnlist.Input) (columnlist.Output, error)
}
type ColumnInspector interface {
	InspectCatalogColumn(context.Context, columninspect.Input) (columninspect.Output, error)
}
type ColumnUpdater interface {
	UpdateCatalogColumn(context.Context, columnupdate.Input, bool) (columnupdate.Output, error)
}

type Searcher interface {
	SearchCatalog(context.Context, catalogsearch.Input) (catalogsearch.Output, error)
}
type Auditor interface {
	AuditCatalog(context.Context, catalogaudit.Input) (catalogaudit.Output, error)
}
type Renderer interface{ Render(any) error }
type Dependencies struct {
	DatabaseLister    DatabaseLister
	DatabaseInspector DatabaseInspector
	DatabaseUpdater   DatabaseUpdater
	TableLister       TableLister
	TableInspector    TableInspector
	TableUpdater      TableUpdater
	ColumnLister      ColumnLister
	ColumnInspector   ColumnInspector
	ColumnUpdater     ColumnUpdater
	Searcher          Searcher
	Auditor           Auditor
	Renderer          Renderer
}

func New(d Dependencies) *cobra.Command {
	root := &cobra.Command{Use: "catalog", Short: "Inspect and enrich upstream Tableau Catalog metadata."}
	root.AddCommand(newDatabase(d), newTable(d), newColumn(d), newSearch(d), newAudit(d))
	return root
}
func noArgs(op string, validate func() error) func(*cobra.Command, []string) error {
	return func(c *cobra.Command, args []string) error {
		if err := cobra.NoArgs(c, args); err != nil {
			return clierr.Usage(op, err)
		}
		return validate()
	}
}
func missing(op string) error {
	return clierr.Usage(op, errors.New("catalog capability is not configured"))
}
func render(r Renderer, v any, err error) error {
	if err != nil {
		return clierr.WithOutput(v, err)
	}
	if r == nil {
		return errors.New("catalog renderer is not configured")
	}
	return r.Render(v)
}

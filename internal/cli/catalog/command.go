// Package catalog contains thin command plumbing for upstream metadata.
package catalog

import (
	"context"
	"errors"

	catalogaction "github.com/ahillspace/tadx/actions/catalog"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

type DatabaseLister interface {
	ListCatalogDatabases(context.Context, catalogaction.DatabaseListInput) (catalogaction.DatabaseListOutput, error)
}
type DatabaseInspector interface {
	InspectCatalogDatabase(context.Context, catalogaction.DatabaseInspectInput) (catalogaction.DatabaseInspectOutput, error)
}
type DatabaseUpdater interface {
	UpdateCatalogDatabase(context.Context, catalogaction.DatabaseInput, bool) (catalogaction.DatabaseOutput, error)
}
type TableLister interface {
	ListCatalogTables(context.Context, catalogaction.TableListInput) (catalogaction.TableListOutput, error)
}
type TableInspector interface {
	InspectCatalogTable(context.Context, catalogaction.TableInspectInput) (catalogaction.TableInspectOutput, error)
}
type TableUpdater interface {
	UpdateCatalogTable(context.Context, catalogaction.TableInput, bool) (catalogaction.TableOutput, error)
}
type ColumnLister interface {
	ListCatalogColumns(context.Context, catalogaction.ColumnListInput) (catalogaction.ColumnListOutput, error)
}
type ColumnInspector interface {
	InspectCatalogColumn(context.Context, catalogaction.ColumnInspectInput) (catalogaction.ColumnInspectOutput, error)
}
type ColumnUpdater interface {
	UpdateCatalogColumn(context.Context, catalogaction.ColumnInput, bool) (catalogaction.ColumnOutput, error)
}

type Searcher interface {
	SearchCatalog(context.Context, catalogaction.SearchInput) (catalogaction.SearchOutput, error)
}
type Auditor interface {
	AuditCatalog(context.Context, catalogaction.AuditInput) (catalogaction.AuditOutput, error)
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
	root := NewGroup()
	root.AddCommand(newDatabase(d), newTable(d), newColumn(d), newSearch(d), newAudit(d))
	return root
}

// NewGroup creates the catalog boundary without registering unconfigured actions.
func NewGroup() *cobra.Command {
	return &cobra.Command{Use: "catalog", Short: "Inspect and enrich Tableau metadata, lineage, and labels."}
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

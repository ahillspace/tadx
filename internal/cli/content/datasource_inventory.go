package content

import (
	"context"

	datasourceinspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/spf13/cobra"
)

// DatasourceLister lists one bounded published datasource page.
type DatasourceLister interface {
	ListDatasources(context.Context, datasourcelist.Input) (datasourcelist.Output, error)
}

// DatasourceInspector inspects one exact published datasource.
type DatasourceInspector interface {
	InspectDatasource(context.Context, datasourceinspect.Input) (datasourceinspect.Output, error)
}

type datasourceInventoryDependencies struct {
	lister    DatasourceLister
	inspector DatasourceInspector
	renderer  Renderer
}

func newDatasourceInventory(lister DatasourceLister, inspector DatasourceInspector, renderer Renderer) *cobra.Command {
	deps := datasourceInventoryDependencies{lister: lister, inspector: inspector, renderer: renderer}
	command := &cobra.Command{Use: "datasource", Short: "Inspect published Tableau datasources"}
	command.AddCommand(newDatasourceList(deps), newDatasourceInspect(deps))
	return command
}

func newDatasourceList(deps datasourceInventoryDependencies) *cobra.Command {
	var input datasourcelist.Input
	command := &cobra.Command{
		Use: "list", Short: "List one bounded published datasource page.",
		Annotations: map[string]string{"tadx.capability": "datasource.list"},
		Args:        noContentArgs("datasource.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.lister.ListDatasources(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact datasource-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact leaf project name filter; not a project path")
	command.Flags().StringVar(&input.Type, "type", "", "exact datasource-type filter")
	command.Flags().StringVar(&input.Tag, "tag", "", "exact tag filter")
	command.Flags().StringVar(&input.UpdatedAfter, "updated-after", "", "include datasources updated at or after this UTC timestamp")
	command.Flags().StringVar(&input.UpdatedBefore, "updated-before", "", "include datasources updated at or before this UTC timestamp")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum datasources to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}

func newDatasourceInspect(deps datasourceInventoryDependencies) *cobra.Command {
	var input datasourceinspect.Input
	var luid, name, projectPath string
	command := &cobra.Command{
		Use: "inspect", Short: "Inspect one exact published datasource.",
		Annotations: map[string]string{"tadx.capability": "datasource.inspect"},
		Args:        selectorArgs("datasource.inspect", &luid, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.inspector.InspectDatasource(command.Context(), input)
			if err != nil {
				return err
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&name, "name", "", "exact datasource name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}

package content

import (
	"context"
	"github.com/ahillspace/tadx/internal/cli/clierr"

	datasourceinspect "github.com/ahillspace/tadx/actions/datasource/inspect"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/spf13/cobra"
)

// DatasourceLister lists a complete published datasource inventory.
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
	command := &cobra.Command{
		Use:   "datasource",
		Short: "Operate published Tableau datasources",
		Long: "Operate published datasource lifecycle and inspect schema with TADX.\n\n" +
			"Schema inspection returns field and table metadata; TADX does not query datasource values.",
	}
	command.AddCommand(newDatasourceList(deps), newDatasourceInspect(deps))
	return command
}

func newDatasourceList(deps datasourceInventoryDependencies) *cobra.Command {
	var input datasourcelist.Input
	command := &cobra.Command{
		Use: "list", Short: "List datasources with bounded live reads or explicit --all.",
		Annotations: map[string]string{"tadx.capability": "datasource.list"},
		Args:        noContentArgs("datasource.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.lister.ListDatasources(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact datasource-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectLUID, "project-id", "", "authoritative project LUID filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact leaf project name filter; not a project path")
	command.Flags().StringVar(&input.Type, "type", "", "exact datasource-type filter")
	command.Flags().StringVar(&input.Tag, "tag", "", "exact tag filter")
	command.Flags().StringVar(&input.UpdatedAfter, "updated-after", "", "include datasources updated at or after this UTC timestamp")
	command.Flags().StringVar(&input.UpdatedBefore, "updated-before", "", "include datasources updated at or before this UTC timestamp")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum datasources to render, from 1 to 10000 (default 25)")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

func newDatasourceInspect(deps datasourceInventoryDependencies) *cobra.Command {
	var input datasourceinspect.Input
	var luid, name, projectPath, projectID string
	command := &cobra.Command{
		Use: "inspect", Short: "Inspect one exact published datasource.",
		Annotations: map[string]string{"tadx.capability": "datasource.inspect"},
		Args:        selectorArgsWithProjectID("datasource.inspect", &luid, &name, &projectPath, &projectID, input.SetSelectorWithProjectLUID),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deps.inspector.InspectDatasource(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return deps.renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&name, "name", "", "exact datasource name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&projectID, "project-id", "", "authoritative project LUID")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

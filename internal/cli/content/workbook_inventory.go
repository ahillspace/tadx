package content

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/cli/clierr"

	"github.com/spf13/cobra"
)

// WorkbookLister runs complete remote workbook inventory.
type WorkbookLister interface {
	ListWorkbooks(context.Context, workbookops.ListInput) (workbookops.ListOutput, error)
}

// WorkbookInspector inspects one exact remote workbook.
type WorkbookInspector interface {
	InspectWorkbook(context.Context, workbookops.InspectInput) (workbookops.InspectOutput, error)
}

type WorkbookSchemaGetter interface {
	GetWorkbookSchema(context.Context, workbookops.SchemaInput) (workbookops.SchemaOutput, error)
}

func newWorkbookSchema(getter WorkbookSchemaGetter, renderer Renderer) *cobra.Command {
	var in workbookops.SchemaInput
	c := &cobra.Command{Use: "schema", Short: "Inspect fields inside a workbook's embedded and published datasources.", Annotations: map[string]string{"tadx.capability": "workbook.schema"}, Args: noContentArgs("workbook.schema"), RunE: func(c *cobra.Command, _ []string) error {
		if err := workbookops.ValidateSchemaInput(in); err != nil {
			return err
		}
		out, err := getter.GetWorkbookSchema(c.Context(), in)
		if err != nil {
			return clierr.WithOutput(out, err)
		}
		return renderer.Render(out)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "exact environment alias; defaults to configured read environment")
	c.Flags().StringVar(&in.WorkbookLUID, "id", "", "authoritative workbook LUID")
	c.Flags().StringVar(&in.Datasource, "datasource", "", "exact datasource name within the workbook; ambiguous names fail")
	c.Flags().StringVar(&in.Query, "query", "", "case-insensitive field ID, name, caption, label, or formula text")
	c.Flags().StringVar(&in.Role, "role", "", "field role: measure, dimension, date, or excluded")
	c.Flags().StringArrayVar(&in.FieldIDs, "field-id", nil, "exact field identifier; repeatable; ambiguous IDs require --datasource")
	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum fields across all datasource groups, 1 to 10000 (default 20)")
	c.Flags().BoolVar(&in.All, "all", false, "return all matching fields, up to 10000")
	c.Flags().StringVar(&in.Cursor, "cursor", "", "opaque schema continuation cursor")
	_ = c.Flags().MarkHidden("cursor")
	c.Flags().BoolVar(&in.Descriptions, "descriptions", false, "include direct and inherited descriptions with provenance")
	c.Flags().BoolVar(&in.Tags, "tags", false, "include upstream column tags; fields do not own tags")
	c.MarkFlagsMutuallyExclusive("all", "limit")
	c.MarkFlagsMutuallyExclusive("all", "cursor")
	return c
}

func newWorkbookList(lister WorkbookLister, renderer Renderer) *cobra.Command {
	var input workbookops.ListInput
	command := &cobra.Command{
		Use: "list", Short: "List workbooks with bounded live reads or explicit --all.", Annotations: map[string]string{"tadx.capability": "workbook.list"}, Args: noContentArgs("workbook.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := lister.ListWorkbooks(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact workbook-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectLUID, "project-id", "", "authoritative project LUID filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact leaf project name filter; not a project path")
	command.Flags().StringVar(&input.Tag, "tag", "", "exact workbook-tag filter")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum workbooks to render, from 1 to 10000 (default 25)")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

func newWorkbookInspect(inspector WorkbookInspector, renderer Renderer) *cobra.Command {
	var input workbookops.InspectInput
	var luid, name, projectPath, projectID string
	command := &cobra.Command{
		Use: "inspect", Short: "Inspect one exact workbook.", Annotations: map[string]string{"tadx.capability": "workbook.inspect"}, Args: selectorArgsWithProjectID("workbook.inspect", &luid, &name, &projectPath, &projectID, input.SetSelectorWithProjectLUID),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := inspector.InspectWorkbook(command.Context(), input)
			if err != nil {
				return clierr.WithOutput(result, err)
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative workbook LUID")
	command.Flags().StringVar(&name, "name", "", "exact workbook name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().StringVar(&projectID, "project-id", "", "authoritative project LUID")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read indexed local cache data without contacting Tableau")
	return command
}

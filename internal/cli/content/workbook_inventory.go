package content

import (
	"context"

	workbookinspect "github.com/ahillspace/tadx/actions/workbook/inspect"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/spf13/cobra"
)

// WorkbookLister runs complete remote workbook inventory.
type WorkbookLister interface {
	ListWorkbooks(context.Context, workbooklist.Input) (workbooklist.Output, error)
}

// WorkbookInspector inspects one exact remote workbook.
type WorkbookInspector interface {
	InspectWorkbook(context.Context, workbookinspect.Input) (workbookinspect.Output, error)
}

func newWorkbookList(lister WorkbookLister, renderer Renderer) *cobra.Command {
	var input workbooklist.Input
	command := &cobra.Command{
		Use: "list", Short: "List workbooks with bounded live reads or explicit --all.", Annotations: map[string]string{"tadx.capability": "workbook.list"}, Args: noContentArgs("workbook.list"),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := lister.ListWorkbooks(command.Context(), input)
			if err != nil {
				return err
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.Name, "name", "", "exact workbook-name filter")
	command.Flags().StringVar(&input.OwnerName, "owner", "", "exact owner-name filter")
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact leaf project name filter; not a project path")
	command.Flags().StringVar(&input.Tag, "tag", "", "exact workbook-tag filter")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching records, up to 10000; cannot combine with --limit")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum workbooks to render")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.MarkFlagsMutuallyExclusive("all", "cursor")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}

func newWorkbookInspect(inspector WorkbookInspector, renderer Renderer) *cobra.Command {
	var input workbookinspect.Input
	var luid, name, projectPath string
	command := &cobra.Command{
		Use: "inspect", Short: "Inspect one exact workbook.", Annotations: map[string]string{"tadx.capability": "workbook.inspect"}, Args: selectorArgs("workbook.inspect", &luid, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := inspector.InspectWorkbook(command.Context(), input)
			if err != nil {
				return err
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&luid, "id", "", "authoritative workbook LUID")
	command.Flags().StringVar(&name, "name", "", "exact workbook name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read indexed local catalog data without contacting Tableau")
	return command
}

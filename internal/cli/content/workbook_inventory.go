package content

import (
	"context"

	workbookget "github.com/ahillspace/tadx/actions/workbook/get"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	"github.com/spf13/cobra"
)

// WorkbookLister runs bounded remote workbook inventory.
type WorkbookLister interface {
	ListWorkbooks(context.Context, workbooklist.Input) (workbooklist.Output, error)
}

// WorkbookGetter inspects one exact remote workbook.
type WorkbookGetter interface {
	GetWorkbook(context.Context, workbookget.Input) (workbookget.Output, error)
}

func newWorkbookList(lister WorkbookLister, renderer Renderer) *cobra.Command {
	var input workbooklist.Input
	command := &cobra.Command{
		Use: "list", Short: "List one bounded workbook page.", Annotations: map[string]string{"tadx.capability": "workbook.list"}, Args: noContentArgs("workbook.list"),
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
	command.Flags().StringVar(&input.ProjectName, "project-name", "", "exact project-name filter")
	command.Flags().StringVar(&input.Tag, "tag", "", "exact workbook-tag filter")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum workbooks to return")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	return command
}

func newWorkbookGet(getter WorkbookGetter, renderer Renderer) *cobra.Command {
	var input workbookget.Input
	var luid, name, projectPath string
	command := &cobra.Command{
		Use: "get", Short: "Inspect one exact workbook.", Annotations: map[string]string{"tadx.capability": "workbook.get"}, Args: selectorArgs("workbook.get", &luid, &name, &projectPath, input.SetSelector),
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := getter.GetWorkbook(command.Context(), input)
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
	return command
}

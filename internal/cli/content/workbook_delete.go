package content

import (
	"context"
	"errors"

	workbookdelete "github.com/ahillspace/tadx/actions/workbook/delete"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// WorkbookDeleter previews or applies one exact remote workbook deletion.
type WorkbookDeleter interface {
	DeleteWorkbook(context.Context, workbookdelete.Input, bool) (workbookdelete.Output, error)
}

func newWorkbookDelete(deleter WorkbookDeleter, renderer Renderer, _ bool) *cobra.Command {
	var input workbookdelete.Input
	var luid, name, projectPath string
	var preview bool
	command := &cobra.Command{
		Use:         "delete",
		Short:       "Delete one exact remote workbook.",
		Annotations: map[string]string{"tadx.capability": "workbook.delete"},
		Args: func(command *cobra.Command, args []string) error {
			if err := selectorArgs("workbook.delete", &luid, &name, &projectPath, input.SetSelector)(command, args); err != nil {
				return err
			}
			if input.Environment == "" {
				return clierr.Usage("workbook.delete", errors.New("--environment is required for remote deletion"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := deleter.DeleteWorkbook(command.Context(), input, preview)
			if err != nil {
				return err
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "explicit write environment alias")
	command.Flags().StringVar(&luid, "id", "", "authoritative workbook LUID")
	command.Flags().StringVar(&name, "name", "", "exact workbook name")
	command.Flags().StringVar(&projectPath, "project", "", "exact slash-delimited project path")
	command.Flags().BoolVar(&preview, "preview", false, "preview the remote deletion without performing it")
	return command
}

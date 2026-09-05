package content

import (
	"context"
	"errors"

	datasourceschema "github.com/ahillspace/tadx/actions/datasource/schema"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

// DatasourceSchemaGetter lists one datasource's tables and bounded fields.
type DatasourceSchemaGetter interface {
	GetDatasourceSchema(context.Context, datasourceschema.Input) (datasourceschema.Output, error)
}

func newDatasourceSchema(getter DatasourceSchemaGetter, renderer Renderer) *cobra.Command {
	var input datasourceschema.Input
	command := &cobra.Command{
		Use: "schema", Short: "Inspect one datasource's tables and fields.",
		Annotations: map[string]string{"tadx.capability": "datasource.schema"},
		Args: func(command *cobra.Command, args []string) error {
			if err := cobra.NoArgs(command, args); err != nil {
				return clierr.Usage("datasource.schema", err)
			}
			if input.DatasourceLUID == "" {
				return clierr.Usage("datasource.schema", errors.New("--id is required"))
			}
			return nil
		},
		RunE: func(command *cobra.Command, _ []string) error {
			result, err := getter.GetDatasourceSchema(command.Context(), input)
			if err != nil {
				return err
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.DatasourceLUID, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&input.Query, "query", "", "case-insensitive text contained in a field ID, name, caption, label, or formula; use --table to select a table")
	command.Flags().StringVar(&input.Role, "role", "", "exact field role: measure, dimension, date, or excluded")
	command.Flags().StringVar(&input.Table, "table", "", "exact logical table caption")
	command.Flags().StringVar(&input.FieldID, "field-id", "", "exact raw Tableau field identifier")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum fields to return; defaults to 20")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	command.Flags().BoolVar(&input.Catalog, "catalog", false, "read only from the local catalog without contacting Tableau")
	return command
}

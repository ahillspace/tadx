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
				return clierr.WithOutput(result, err)
			}
			return renderer.Render(result)
		},
	}
	command.Flags().StringVar(&input.Environment, "environment", "", "exact environment alias; defaults to the configured read environment")
	command.Flags().StringVar(&input.DatasourceLUID, "id", "", "authoritative datasource LUID")
	command.Flags().StringVar(&input.Query, "query", "", "case-insensitive text contained in a field ID, name, caption, label, or formula; use --table to select a table")
	command.Flags().StringVar(&input.Role, "role", "", "exact field role: measure, dimension, date, or excluded")
	command.Flags().StringVar(&input.Table, "table", "", "exact logical table caption")
	command.Flags().StringArrayVar(&input.FieldIDs, "field-id", nil, "exact raw Tableau field identifier; repeat to select multiple fields")
	command.Flags().IntVar(&input.Limit, "limit", 0, "maximum fields to return, up to 10000; defaults to 20")
	command.Flags().StringVar(&input.Cursor, "cursor", "", "opaque continuation cursor")
	_ = command.Flags().MarkHidden("cursor")
	command.Flags().BoolVar(&input.All, "all", false, "return all matching fields, up to 10,000; cannot combine with --limit")
	command.MarkFlagsMutuallyExclusive("all", "limit")
	command.Flags().BoolVar(&input.Cache, "cache", false, "read only from the local cache without contacting Tableau")
	command.Flags().BoolVar(&input.Descriptions, "descriptions", false, "include direct and inherited descriptions with source identities; an explicit metadata read")
	command.Flags().BoolVar(&input.Tags, "tags", false, "include upstream column tags with source identities; published fields have no tag API")
	return command
}

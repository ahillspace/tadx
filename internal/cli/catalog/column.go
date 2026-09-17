package catalog

import (
	columninspect "github.com/ahillspace/tadx/actions/catalog/column/inspect"
	columnlist "github.com/ahillspace/tadx/actions/catalog/column/list"
	columnupdate "github.com/ahillspace/tadx/actions/catalog/column/update"
	"github.com/spf13/cobra"
)

func newColumn(d Dependencies) *cobra.Command {
	c := &cobra.Command{Use: "column", Short: "Inspect and enrich upstream column metadata."}
	c.AddCommand(newColumnList(d), newColumnInspect(d), newColumnUpdate(d))
	return c
}
func newColumnList(d Dependencies) *cobra.Command {
	var in columnlist.Input
	c := &cobra.Command{Use: "list", Short: "List bounded upstream column identities.", Annotations: map[string]string{"tadx.capability": "catalog.column.list"}, Args: noArgs("catalog.column.list", func() error { return columnlist.ValidateInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.ColumnLister == nil {
			return missing("catalog.column.list")
		}
		out, err := d.ColumnLister.ListCatalogColumns(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.Name, "name", "", "exact column name filter")

	c.Flags().StringVar(&in.TableID, "table-id", "", "required parent table REST LUID")
	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum matching records, 1 to 10000 (default 25)")
	c.Flags().BoolVar(&in.All, "all", false, "collect all matching records within the 10000-record bound")
	c.MarkFlagsMutuallyExclusive("all", "limit")
	return c
}
func newColumnInspect(d Dependencies) *cobra.Command {
	var in columninspect.Input
	c := &cobra.Command{Use: "inspect", Short: "Inspect one exact upstream column; --full expands fetched metadata.", Annotations: map[string]string{"tadx.capability": "catalog.column.inspect"}, Args: noArgs("catalog.column.inspect", func() error { return columninspect.ValidateInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.ColumnInspector == nil {
			return missing("catalog.column.inspect")
		}
		out, err := d.ColumnInspector.InspectCatalogColumn(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.ID, "id", "", "exact column REST LUID")
	c.Flags().StringVar(&in.MetadataID, "metadata-id", "", "exact GraphQL Metadata ID; read-only alternative to --id")
	c.Flags().StringVar(&in.TableID, "table-id", "", "parent table REST LUID required with --id")
	c.MarkFlagsMutuallyExclusive("id", "metadata-id")
	return c
}
func newColumnUpdate(d Dependencies) *cobra.Command {
	var in columnupdate.Input
	var description, contact string
	var preview bool
	_ = contact
	capture := func(c *cobra.Command) {
		in.Description = nil
		if c.Flags().Changed("description") {
			s := description
			in.Description = &s
		}
	}
	c := &cobra.Command{Use: "update", Short: "Update supported column description, or tags; --preview makes no changes.", Annotations: map[string]string{"tadx.capability": "catalog.column.update"}}
	c.Args = func(c *cobra.Command, args []string) error {
		capture(c)
		return noArgs("catalog.column.update", func() error { return columnupdate.ValidateInput(in) })(c, args)
	}
	c.RunE = func(c *cobra.Command, _ []string) error {
		capture(c)
		if d.ColumnUpdater == nil {
			return missing("catalog.column.update")
		}
		out, err := d.ColumnUpdater.UpdateCatalogColumn(c.Context(), in, preview)
		return render(d.Renderer, out, err)
	}
	c.Flags().StringVar(&in.Environment, "environment", "", "explicit destination environment; inferred only when one is configured")
	c.Flags().StringVar(&in.ID, "id", "", "exact column REST LUID")
	c.Flags().StringVar(&in.TableID, "table-id", "", "required parent table REST LUID")
	c.Flags().StringVar(&description, "description", "", "set a description; an empty value clears it and omission preserves the current value")

	c.Flags().StringArrayVar(&in.AddTags, "add-tag", nil, "add a tag without replacing unrelated tags; repeatable")
	c.Flags().StringArrayVar(&in.RemoveTags, "remove-tag", nil, "remove this exact tag only; repeatable")
	c.Flags().BoolVar(&preview, "preview", false, "inspect the intended changes without executing remote mutations")
	return c
}

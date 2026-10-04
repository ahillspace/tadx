package catalog

import (
	catalogaction "github.com/ahillspace/tadx/actions/catalog"
	"github.com/spf13/cobra"
)

func newTable(d Dependencies) *cobra.Command {
	c := &cobra.Command{Use: "table", Short: "Inspect and enrich upstream table metadata."}
	c.AddCommand(newTableList(d), newTableInspect(d), newTableUpdate(d))
	return c
}
func newTableList(d Dependencies) *cobra.Command {
	var in catalogaction.TableListInput
	c := &cobra.Command{Use: "list", Short: "List bounded upstream table identities.", Annotations: map[string]string{"tadx.capability": "catalog.table.list"}, Args: noArgs("catalog.table.list", func() error { return catalogaction.ValidateTableListInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.TableLister == nil {
			return missing("catalog.table.list")
		}
		out, err := d.TableLister.ListCatalogTables(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.Name, "name", "", "exact table name filter")
	c.Flags().StringVar(&in.DatabaseID, "database-id", "", "exact parent database REST LUID")

	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum matching records, 1 to 10000 (default 25)")
	c.Flags().BoolVar(&in.All, "all", false, "collect all matching records within the 10000-record bound")
	c.MarkFlagsMutuallyExclusive("all", "limit")
	return c
}
func newTableInspect(d Dependencies) *cobra.Command {
	var in catalogaction.TableInspectInput
	c := &cobra.Command{Use: "inspect", Short: "Inspect one exact upstream table; --full expands fetched metadata.", Annotations: map[string]string{"tadx.capability": "catalog.table.inspect"}, Args: noArgs("catalog.table.inspect", func() error { return catalogaction.ValidateTableInspectInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.TableInspector == nil {
			return missing("catalog.table.inspect")
		}
		out, err := d.TableInspector.InspectCatalogTable(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.ID, "id", "", "exact table REST LUID")
	c.Flags().StringVar(&in.MetadataID, "metadata-id", "", "exact GraphQL Metadata ID; read-only alternative to --id")

	c.MarkFlagsMutuallyExclusive("id", "metadata-id")
	return c
}
func newTableUpdate(d Dependencies) *cobra.Command {
	var in catalogaction.TableInput
	var description, contact string
	var preview bool
	capture := func(c *cobra.Command) {
		in.Description = nil
		if c.Flags().Changed("description") {
			in.Description = new(description)
		}
		in.ContactLUID = nil
		if c.Flags().Changed("contact-id") {
			in.ContactLUID = new(contact)
		}
	}
	c := &cobra.Command{Use: "update", Short: "Update supported table description, contact, or tags; --preview makes no changes.", Annotations: map[string]string{"tadx.capability": "catalog.table.update"}}
	c.Args = func(c *cobra.Command, args []string) error {
		capture(c)
		return noArgs("catalog.table.update", func() error { return catalogaction.ValidateTableInput(in) })(c, args)
	}
	c.RunE = func(c *cobra.Command, _ []string) error {
		if d.TableUpdater == nil {
			return missing("catalog.table.update")
		}
		out, err := d.TableUpdater.UpdateCatalogTable(c.Context(), in, preview)
		return render(d.Renderer, out, err)
	}
	c.Flags().StringVar(&in.Environment, "environment", "", "explicit destination environment; inferred only when one is configured")
	c.Flags().StringVar(&in.ID, "id", "", "exact table REST LUID")

	c.Flags().StringVar(&description, "description", "", "set a nonempty description; omission preserves the current value")
	c.Flags().StringVar(&contact, "contact-id", "", "set an exact contact user LUID; omission preserves the current contact")
	c.Flags().StringArrayVar(&in.AddTags, "add-tag", nil, "add a tag without replacing unrelated tags; repeatable")
	c.Flags().StringArrayVar(&in.RemoveTags, "remove-tag", nil, "remove this exact tag only; repeatable")
	c.Flags().BoolVar(&preview, "preview", false, "inspect the intended changes without executing remote mutations")
	return c
}

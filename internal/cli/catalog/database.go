package catalog

import (
	databaseinspect "github.com/ahillspace/tadx/actions/catalog/database/inspect"
	databaselist "github.com/ahillspace/tadx/actions/catalog/database/list"
	databaseupdate "github.com/ahillspace/tadx/actions/catalog/database/update"
	"github.com/spf13/cobra"
)

func newDatabase(d Dependencies) *cobra.Command {
	c := &cobra.Command{Use: "database", Short: "Inspect and enrich upstream database metadata."}
	c.AddCommand(newDatabaseList(d), newDatabaseInspect(d), newDatabaseUpdate(d))
	return c
}
func newDatabaseList(d Dependencies) *cobra.Command {
	var in databaselist.Input
	c := &cobra.Command{Use: "list", Short: "List bounded upstream database identities.", Annotations: map[string]string{"tadx.capability": "catalog.database.list"}, Args: noArgs("catalog.database.list", func() error { return databaselist.ValidateInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.DatabaseLister == nil {
			return missing("catalog.database.list")
		}
		out, err := d.DatabaseLister.ListCatalogDatabases(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.Name, "name", "", "exact database name filter")

	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum matching records, 1 to 10000 (default 25)")
	c.Flags().BoolVar(&in.All, "all", false, "collect all matching records within the 10000-record bound")
	c.MarkFlagsMutuallyExclusive("all", "limit")
	return c
}
func newDatabaseInspect(d Dependencies) *cobra.Command {
	var in databaseinspect.Input
	c := &cobra.Command{Use: "inspect", Short: "Inspect one exact upstream database; --full expands fetched metadata.", Annotations: map[string]string{"tadx.capability": "catalog.database.inspect"}, Args: noArgs("catalog.database.inspect", func() error { return databaseinspect.ValidateInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.DatabaseInspector == nil {
			return missing("catalog.database.inspect")
		}
		out, err := d.DatabaseInspector.InspectCatalogDatabase(c.Context(), in)
		return render(d.Renderer, out, err)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.ID, "id", "", "exact database REST LUID")
	c.Flags().StringVar(&in.MetadataID, "metadata-id", "", "exact GraphQL Metadata ID; read-only alternative to --id")

	c.MarkFlagsMutuallyExclusive("id", "metadata-id")
	return c
}
func newDatabaseUpdate(d Dependencies) *cobra.Command {
	var in databaseupdate.Input
	var description, contact string
	var preview bool
	_ = contact
	capture := func(c *cobra.Command) {
		in.Description = nil
		if c.Flags().Changed("description") {
			s := description
			in.Description = &s
		}
		in.ContactLUID = nil
		if c.Flags().Changed("contact-id") {
			s := contact
			in.ContactLUID = &s
		}
	}
	c := &cobra.Command{Use: "update", Short: "Update supported database description, contact, or tags; --preview makes no changes.", Annotations: map[string]string{"tadx.capability": "catalog.database.update"}}
	c.Args = func(c *cobra.Command, args []string) error {
		capture(c)
		return noArgs("catalog.database.update", func() error { return databaseupdate.ValidateInput(in) })(c, args)
	}
	c.RunE = func(c *cobra.Command, _ []string) error {
		capture(c)
		if d.DatabaseUpdater == nil {
			return missing("catalog.database.update")
		}
		out, err := d.DatabaseUpdater.UpdateCatalogDatabase(c.Context(), in, preview)
		return render(d.Renderer, out, err)
	}
	c.Flags().StringVar(&in.Environment, "environment", "", "explicit destination environment; inferred only when one is configured")
	c.Flags().StringVar(&in.ID, "id", "", "exact database REST LUID")

	c.Flags().StringVar(&description, "description", "", "set a nonempty description; omission preserves the current value")
	c.Flags().StringVar(&contact, "contact-id", "", "set an exact contact user LUID; omission preserves the current contact")
	c.Flags().StringArrayVar(&in.AddTags, "add-tag", nil, "add a tag without replacing unrelated tags; repeatable")
	c.Flags().StringArrayVar(&in.RemoveTags, "remove-tag", nil, "remove this exact tag only; repeatable")
	c.Flags().BoolVar(&preview, "preview", false, "inspect the intended changes without executing remote mutations")
	return c
}

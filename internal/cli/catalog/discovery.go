package catalog

import (
	catalogaudit "github.com/ahillspace/tadx/actions/catalog/audit"
	catalogsearch "github.com/ahillspace/tadx/actions/catalog/search"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"github.com/spf13/cobra"
)

func newSearch(d Dependencies) *cobra.Command {
	var in catalogsearch.Input
	c := &cobra.Command{Use: "search <query>", Short: "Search upstream database/table metadata; column search uses a bounded table scan.", Annotations: map[string]string{"tadx.capability": "catalog.search"}}
	c.Args = func(c *cobra.Command, args []string) error {
		if e := cobra.ExactArgs(1)(c, args); e != nil {
			return clierr.Usage("catalog.search", e)
		}
		in.Query = args[0]
		return catalogsearch.ValidateInput(in)
	}
	c.RunE = func(c *cobra.Command, _ []string) error {
		if d.Searcher == nil {
			return missing("catalog.search")
		}
		out, e := d.Searcher.SearchCatalog(c.Context(), in)
		return render(d.Renderer, out, e)
	}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringArrayVar(&in.Types, "type", nil, "database, table, or column; repeatable (default database and table)")
	c.Flags().StringVar(&in.TableID, "table-id", "", "required exact parent table REST LUID for column search")
	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum matches, 1 to 10000 (default 25); column scans remain bounded")
	c.Flags().BoolVar(&in.All, "all", false, "collect matches within the 10000-record traversal bound")
	c.MarkFlagsMutuallyExclusive("all", "limit")
	return c
}
func newAudit(d Dependencies) *cobra.Command {
	var in catalogaudit.Input
	c := &cobra.Command{Use: "audit", Short: "Audit descriptions and tags inside one exact database, table, or datasource scope.", Annotations: map[string]string{"tadx.capability": "catalog.audit"}, Args: noArgs("catalog.audit", func() error { return catalogaudit.ValidateInput(in) }), RunE: func(c *cobra.Command, _ []string) error {
		if d.Auditor == nil {
			return missing("catalog.audit")
		}
		out, e := d.Auditor.AuditCatalog(c.Context(), in)
		return render(d.Renderer, out, e)
	}}
	c.Flags().StringVar(&in.Environment, "environment", "", "environment alias; defaults to the configured read environment")
	c.Flags().StringVar(&in.Type, "type", "", "required scope type: database, table, or datasource")
	c.Flags().StringVar(&in.ID, "id", "", "required exact scope REST LUID")
	c.Flags().StringArrayVar(&in.Checks, "check", nil, "descriptions or tags; repeatable; omitted runs all supported checks in scope")
	c.Flags().BoolVar(&in.DirectOnly, "direct-only", false, "require field-owned descriptions instead of counting inherited descriptions")
	c.Flags().IntVar(&in.Limit, "limit", 0, "maximum assessed assets, 1 to 10000 (default 1000)")
	return c
}

package catalog

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	"github.com/spf13/cobra"
)

// ApplyHelpFacts attaches this command family's presentation-only contracts.
func ApplyHelpFacts(command *cobra.Command, path string) {
	if path == "catalog" {
		helpmeta.Summary(command, "Upstream metadata, lineage, and attached labels")
	}
	switch path {
	case "catalog search":
		helpmeta.Choices(command, "type", "database", "table", "column")
		helpmeta.FlagNote(command, "type", "values must be unique; column requires --table-id")
	case "catalog audit":
		helpmeta.Choices(command, "type", "database", "table", "datasource")
		helpmeta.Choices(command, "check", "descriptions", "tags")
		helpmeta.FlagNote(command, "check", "values must be unique")
	case "catalog lineage pull":
		helpmeta.Choices(command, "kind", "workbook", "datasource", "published_datasource", "flow")
		helpmeta.Choices(command, "direction", "upstream", "downstream", "both")
		helpmeta.Value(command, "depth", "1..3")
	case "catalog label list", "catalog label inspect", "catalog label update", "catalog label delete":
		helpmeta.Choices(command, "type", "database", "table", "column", "datasource", "flow")
	}
	switch path {
	case "catalog label list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"20"})
	case "catalog database list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "catalog table list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "catalog column list":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "catalog search":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"25"})
	case "catalog audit":
		helpmeta.Value(command, "limit", "1..10000")
		helpmeta.FlagAnnotation(command, "limit", "tadx.help.default", []string{"1000"})
	}
	switch path {
	case "catalog audit":
		helpmeta.Required(command, "type", "id")
	case "catalog database inspect", "catalog table inspect":
		helpmeta.Group(command, "exactly-one", "id", "metadata-id")
	case "catalog column inspect":
		helpmeta.Group(command, "exactly-one", "id", "metadata-id")
		helpmeta.Constraint(command, "--id requires --table-id; --metadata-id selects the column directly.")
	case "catalog column list":
		helpmeta.Required(command, "table-id")
	case "catalog database update", "catalog table update":
		helpmeta.Required(command, "id")
		helpmeta.Group(command, "one-required", "description", "contact-id", "add-tag", "remove-tag")
	case "catalog column update":
		helpmeta.Required(command, "id", "table-id")
		helpmeta.Group(command, "one-required", "description", "add-tag", "remove-tag")
	case "catalog lineage pull":
		helpmeta.Required(command, "kind")
		helpmeta.Group(command, "exactly-one", "id", "name")
	case "catalog label list":
		helpmeta.Required(command, "type", "target-id")
	case "catalog label inspect", "catalog label delete":
		helpmeta.Required(command, "id")
	case "catalog label update":
		helpmeta.Constraint(command, "Select --id, or both --type and --target-id with --value. Supply at least one of --value, --message, --active, or --elevated.")
	}
}

package catalog

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "catalog", Note: "Catalog inspects upstream asset metadata, captures lineage, and manages attached labels. REST LUIDs and Metadata API IDs are distinct selectors.", Common: true, Examples: []string{"tadx catalog search <query> --env dev --type database --type table"}},
		{Path: "catalog database", Note: "Inspect accepts exactly one of --id or --metadata-id. Updates require the REST LUID in --id.", Common: false, Examples: []string{}},
		{Path: "catalog table", Note: "Inspect accepts exactly one of --id or --metadata-id. Updates require the REST LUID in --id.", Common: false, Examples: []string{}},
		{Path: "catalog column", Note: "List requires --table-id. Inspect requires --id with --table-id, or --metadata-id alone.", Common: false, Examples: []string{}},
		{Path: "catalog lineage", Note: "Pull requires --kind and an exact --id or --name selector. Depth ranges from 1 to 3.", Common: false, Examples: []string{}},
		{Path: "catalog label", Note: "List requires --type and --target-id. Inspect and delete require the attachment --id.\nUpdate requires --id, or --type and --target-id with --value. Supply at least one label change.", Common: false, Examples: []string{}},
	}
}

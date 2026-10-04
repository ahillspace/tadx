package pulse

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "pulse", Note: "", Common: false, Examples: []string{"tadx pulse subscription list --env dev"}},
		{Path: "pulse subscription", Note: "Lists the current authenticated user's subscriptions. Group-derived coverage depends on the Tableau response.", Common: true, Examples: []string{"tadx pulse subscription list --env dev"}},
		{Path: "pulse definition", Note: "Create requires --name, --datasource-id, --measure-field, and --date-field. Repeat --dimension for allowed slicers.\nPublish requires one of --artifact, --id, or --artifact-name, plus explicit --datasource-map source=destination mappings.", Common: false, Examples: []string{"tadx pulse definition publish --env dev --workspace dev --id <definition-luid> --datasource-map <source-luid>=<destination-luid> --preview"}},
		{Path: "pulse metric", Note: "Fork requires --id and at least one of --period, --filter, or --exclude-filter. CUSTOM_N_DAYS also requires --days.\nFollow requires --id and exactly one of --user-id or --group-id.", Common: false, Examples: []string{"tadx pulse metric fork --env dev --id <metric-luid> --filter <field>=<value> --filter <field>=<second-value> --preview"}},
		{Path: "pulse metric followers", Note: "", Common: false, Examples: []string{}},
	}
}

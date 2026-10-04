package mutation

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "mutation", Note: "Mutation status lists all configured environments; --environment selects one alias, and --full adds server, site, and source.\nMutation set persists consent for the selected server and exact site. Agents need explicit permission for that site and persistent setting.\nProcess overrides do not authorize writes. Supported read-only previews remain available when site consent is disabled.", Common: true, Examples: []string{}},
	}
}

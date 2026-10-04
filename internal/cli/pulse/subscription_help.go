package pulse

import "github.com/ahillspace/tadx/internal/cli/helpmeta"

func subscriptionHelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "pulse subscription", Note: "Lists the current authenticated user's subscriptions. Group-derived coverage depends on the Tableau response.", Common: true, Examples: []string{"tadx pulse subscription list --env dev"}},
	}
}

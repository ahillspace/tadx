package pulse

import "github.com/ahillspace/tadx/internal/cli/helpmeta"

// HelpExamples supplies Pulse examples in the existing command order.
func HelpExamples() []helpmeta.ExampleSet {
	sets := []helpmeta.ExampleSet{
		{Path: "pulse", Note: "", Common: false, Examples: []string{"tadx pulse subscription list --env dev"}},
	}
	sets = append(sets, subscriptionHelpExamples()...)
	sets = append(sets, definitionHelpExamples()...)
	return append(sets, metricHelpExamples()...)
}

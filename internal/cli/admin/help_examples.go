package admin

import "github.com/ahillspace/tadx/internal/cli/helpmeta"

// HelpExamples supplies admin examples in the existing command order.
func HelpExamples() []helpmeta.ExampleSet {
	sets := []helpmeta.ExampleSet{
		{Path: "admin", Note: "Use --preview to inspect supported remote changes. Users, groups, and permission principals use exact selectors.", Common: true, Examples: []string{}},
	}
	sets = append(sets, userHelpExamples()...)
	sets = append(sets, groupHelpExamples()...)
	sets = append(sets, groupMemberHelpExamples()...)
	sets = append(sets, permissionHelpExamples()...)
	return append(sets, labelHelpExamples()...)
}

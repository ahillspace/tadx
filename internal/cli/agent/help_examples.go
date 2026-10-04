package agent

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "agent", Note: "Use --target auto to detect configured agents. Preview reports local file changes without writing them.", Common: false, Examples: []string{}},
	}
}

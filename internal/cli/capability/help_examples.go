package capability

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "capability", Note: "Use capability IDs to inspect ownership, availability, and bounded operation details.", Common: true, Examples: []string{}},
	}
}

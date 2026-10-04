package env

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "env", Note: "Profiles contain non-secret settings. The site value is the Tableau site content URL.", Common: true, Examples: []string{}},
	}
}

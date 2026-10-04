package cache

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "cache", Note: "Reads contact Tableau by default. Pass --cache on supported read commands to use the local inventory.", Common: true, Examples: []string{"tadx cache refresh --env dev --scope workbooks --scope datasources"}},
	}
}

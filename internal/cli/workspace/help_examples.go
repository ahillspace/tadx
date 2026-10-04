package workspace

import (
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
)

// HelpExamples supplies examples and common notes owned by this command family.
func HelpExamples() []helpmeta.ExampleSet {
	return []helpmeta.ExampleSet{
		{Path: "workspace", Note: "Workspace names select registered local roots. Managed artifact paths are relative to those roots and use forward slashes.", Common: true, Examples: []string{}},
		{Path: "workspace artifact", Note: "Select exactly one --artifact path, or both --kind and --id.", Common: false, Examples: []string{"tadx workspace artifact delete --workspace dev --kind workbook --id <workbook-luid> --preview"}},
	}
}

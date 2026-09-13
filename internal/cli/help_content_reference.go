package cli

import (
	"embed"
	"io"

	"github.com/spf13/cobra"
)

//go:embed help_datasource.txt help_workbook.txt help_flow.txt help_project.txt
var contentReferences embed.FS

// Keep the formatted pages verbatim. Partial dependency trees use generated help
// instead of advertising actions that were not registered.
func writeReviewedContentReference(out io.Writer, resource *cobra.Command) bool {
	_, actions := helpNodes(resource)
	expected := map[string]bool{"delete": true, "inspect": true, "list": true, "move": true, "publish": true, "pull": true, "update": true}
	switch resource.Name() {
	case "datasource":
		expected["schema"] = true
	case "workbook", "flow":
	case "project":
		delete(expected, "publish")
		delete(expected, "pull")
		expected["create"] = true
	default:
		return false
	}
	if len(actions) != len(expected) {
		return false
	}
	for _, action := range actions {
		if !expected[action.Name()] {
			return false
		}
	}
	page, err := contentReferences.ReadFile("help_" + resource.Name() + ".txt")
	if err != nil {
		return false
	}
	_, _ = io.WriteString(out, string(page))
	return true
}

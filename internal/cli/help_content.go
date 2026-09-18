package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// Content resources have explicit reference boundaries. Resolve the owner and optional
// focused action without looking at parsed values, so all help spellings render
// from the same definitions.
func writeContentPilotHelp(out io.Writer, command *cobra.Command) bool {
	for node := command; node != nil; node = node.Parent() {
		parent := node.Parent()
		if parent == nil || parent.Name() != "content" || parent.Parent() == nil {
			continue
		}
		switch node.Name() {
		case "workbook", "datasource", "flow", "project":
			if node == command {
				writeContentReference(out, node)
			} else {
				writeContentReference(out, node, command)
			}
			return true
		}
	}
	for node := command; node != nil; node = node.Parent() {
		parent := node.Parent()
		if parent == nil {
			continue
		}
		if node.Name() == "content" && parent.Parent() == nil && node == command {
			writeContentNavigation(out, node)
			return true
		}
	}
	return false
}

func writeContentNavigation(out io.Writer, category *cobra.Command) {
	writeCategoryNavigation(out, category, contentResourceSummary)
}

func contentResourceSummary(resource *cobra.Command) string {
	switch resource.Name() {
	case "workbook":
		return "Tableau workbooks and local workbook files"
	case "datasource":
		return "Published datasources, local files, and field metadata"
	case "flow":
		return "Tableau Prep flows and local flow files"
	case "project":
		return "Tableau content containers and project hierarchy"
	default:
		return helpPlainShort(resource)
	}
}

func writeContentReference(out io.Writer, resource *cobra.Command, focused ...*cobra.Command) {
	var focus *cobra.Command
	if len(focused) > 0 {
		focus = focused[0]
	}
	writeOperationalReference(out, resource, focus)
}

func contentActionSummary(action *cobra.Command) string {
	summaries := map[string]string{
		"list":    "List remote or cached matches",
		"inspect": "Read details, not files",
		"pull":    "Download local files",
		"publish": "Publish local content to Tableau",
		"move":    "Change remote project",
		"update":  "Change remote metadata",
		"delete":  "Delete remote content",
		"create":  "Create a remote project",
		"schema":  "Read tables/fields, not data values",
	}
	if action.Name() == "move" && action.Parent().Name() == "project" {
		return "Change the project's parent or move it to the top level"
	}
	if action.Name() == "update" && action.Parent().Name() == "flow" {
		return "Change the remote flow owner"
	}
	if summary := summaries[action.Name()]; summary != "" {
		return summary
	}
	return helpPlainShort(action)
}

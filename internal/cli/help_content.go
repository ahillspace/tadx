package cli

import (
	"io"

	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/spf13/cobra"
)

// Content resources have explicit reference boundaries. Resolve the owner and optional
// focused action without looking at parsed values, so all help spellings render
// from the same definitions.
func writeContentHelp(out io.Writer, command *cobra.Command) bool {
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
	if text := contentcli.ResourceSummary(resource); text != "" {
		return text
	}
	return helpPlainShort(resource)
}

func writeContentReference(out io.Writer, resource *cobra.Command, focused ...*cobra.Command) {
	var focus *cobra.Command
	if len(focused) > 0 {
		focus = focused[0]
	}
	writeOperationalReference(out, resource, focus)
}

func contentActionSummary(action *cobra.Command) string {
	if text := contentcli.ActionSummary(action); text != "" {
		return text
	}
	return helpPlainShort(action)
}

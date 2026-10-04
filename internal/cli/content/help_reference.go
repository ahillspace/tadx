package content

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// ResourceSummary describes a content resource for category navigation.
func ResourceSummary(resource *cobra.Command) string {
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
		return ""
	}
}

// ActionSummary describes the user-visible purpose of a content action.
func ActionSummary(action *cobra.Command) string {
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
	return ""
}

// ReferenceFlagValue supplies content-specific reference syntax.
func ReferenceFlagValue(resourceName, actionName, flagName string) string {
	value := ""
	switch flagName {
	case "id", "project-id", "owner-id", "parent-id", "destination-project-id":
		value = "luid"
	case "name", "new-name", "artifact-name", "workspace", "environment":
		value = "name"
	case "project-name":
		value = "leaf"
	case "project", "parent", "destination-project", "artifact":
		value = "path"
	case "file":
		if actionName == "publish" {
			value = map[string]string{"workbook": "file.twb|file.twbx", "datasource": "file.tds|file.tdsx|file.hyper", "flow": "file.tfl|file.tflx"}[resourceName]
		}
	}
	return value
}

// ReferenceActionNotes supplies resource-specific operation constraints.
func ReferenceActionNotes(action *cobra.Command) []string {
	resource := action.Parent().Name()
	switch action.Name() {
	case "list":
		if action.Flags().Lookup("top-level") != nil {
			return []string{"--top-level=true: roots; false: nested; omitted: both.", "--owner matches an exact owner name first; a listed owner LUID is accepted when no name matches. LUID fallback requires live inventory, not --cache."}
		}
	case "inspect":
		if resource == "project" {
			return []string{"Exact --id or slash-delimited --project path."}
		}
	case "pull":
		notes := []string{"--overwrite replaces dirty local files."}
		if action.Flags().Lookup("include-pds") != nil {
			notes = append(notes, "--include-pds pulls direct datasource siblings, without recursion.")
		}
		return notes
	case "publish":
		notes := []string{"Source: workspace ID/name/relative artifact, or native file (no workspace). Project: destination; name: source default."}
		if resource == "datasource" {
			return append(notes, "create: collision fails; overwrite: replace; append/replace: data in exact match, prepared .hyper only.")
		}
		if resource == "workbook" {
			notes = append(notes, "Publish missing referenced datasources first, then the workbook. Workbook publish does not publish dependencies or rebind their references.")
		}
		return append(notes, "Creates by default; --overwrite replaces an exact collision.")
	case "update":
		if action.Flags().Lookup("description") != nil {
			return []string{"Empty --description clears it."}
		}
	case "schema":
		return []string{"Limit: fields. --field-id: <=10000, match filters; --table resolves duplicate IDs.", "Descriptions/tags: sourced metadata; cache needs observations."}
	case "create":
		return []string{"Omit parent for top level; omit content-permissions for the server default."}
	case "move":
		if resource == "project" {
			return []string{"Self/descendant parent rejected; same parent is a no-op."}
		}
	}
	return nil
}

// WriteCompactReferenceNotes renders content-specific reference notes.
func WriteCompactReferenceNotes(out io.Writer, resource *cobra.Command, actions []*cobra.Command) {
	has := func(name string) bool {
		for _, action := range actions {
			if action.Name() == name {
				return true
			}
		}
		return false
	}
	fmt.Fprintln(out, "  Exact selectors; ambiguity fails. Project paths use /.")
	if has("list") {
		fmt.Fprintln(out, "  list --all: <=10000; incomplete/overflow fails. Live --all caches best-effort: unfiltered replaces scope; filtered merges.")
		fmt.Fprintln(out, "  Bounded reads never cache; cache failures retain live results.")
	}
	if has("publish") || has("pull") {
		fmt.Fprintln(out, "  --no-wait: one single/batch status command, no polling. Wait stops at 20m; work continues.")
		if has("publish") {
			fmt.Fprintln(out, "  Save accepted IDs before waiting; use the returned status command to recover. Do not republish.")
			fmt.Fprintln(out, "  Repeated --artifact excludes --name.")
			if resource.Name() == "flow" {
				fmt.Fprintln(out, "  Flow publication can complete synchronously.")
			}
		}
		if has("pull") {
			fmt.Fprintln(out, "  Downloads have no remote jobs to poll.")
		}
	}
	if has("publish") || has("update") || has("delete") || has("move") || has("create") {
		fmt.Fprintln(out, "  Remote writes need enabled mutations; preview never authorizes enabling.")
	}
	if resource.Name() == "project" {
		var aliases []string
		for _, action := range actions {
			if action.Flags().Lookup("project-id") != nil && action.Name() != "list" {
				aliases = append(aliases, action.Name())
			}
		}
		if len(aliases) > 0 {
			fmt.Fprintf(out, "  Legacy --project-id = --id (%s); cannot combine.\n", strings.Join(aliases, ","))
		}
		if has("update") {
			fmt.Fprintln(out, "  Legacy --name = --new-name (update); cannot combine.")
		}
	}
}

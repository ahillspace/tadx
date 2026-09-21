package cli

import (
	"fmt"
	"io"
	"maps"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func isContentReference(resource *cobra.Command) bool {
	return resource != nil && resource.Parent() != nil && resource.Parent().Name() == "content"
}

// Both complete and focused help use the registered flags and the same notes.
// Presentation copies never change parsing, validation, or caller-supplied values.
func contentReferenceFlagSyntax(resource, action *cobra.Command, flag *pflag.Flag) string {
	copy := *flag
	copy.Annotations = maps.Clone(flag.Annotations)
	if copy.Annotations == nil {
		copy.Annotations = map[string][]string{}
	}
	value := ""
	switch flag.Name {
	case "id", "project-id", "owner-id", "parent-id", "destination-project-id":
		value = "luid"
	case "name", "new-name", "artifact-name", "workspace", "environment":
		value = "name"
	case "project-name":
		value = "leaf"
	case "project", "parent", "destination-project", "artifact":
		value = "path"
	case "file":
		if action.Name() == "publish" {
			value = map[string]string{"workbook": "file.twb|file.twbx", "datasource": "file.tds|file.tdsx|file.hyper", "flow": "file.tfl|file.tflx"}[resource.Name()]
		}
	}
	if value != "" {
		copy.Annotations["tadx.help.value"] = []string{value}
	}
	if helpRequired(flag) {
		delete(copy.Annotations, "tadx.help.omission")
	}
	return referenceFlagSyntax(&copy)
}

func contentReferenceTerms(resource, action *cobra.Command) []string {
	var terms []string
	used := map[string]bool{}
	for _, flag := range helpFlags(action) {
		if used[flag.Name] || flag.Name == "batch-file" || referenceSharedFlag(flag.Name) && !helpRequired(flag) {
			continue
		}
		var alternatives []string
		for _, group := range flag.Annotations["tadx.help.exactly-one"] {
			for _, name := range strings.Fields(group) {
				if candidate := action.Flags().Lookup(name); candidate != nil && !candidate.Hidden {
					alternatives = append(alternatives, contentReferenceFlagSyntax(resource, action, candidate))
					used[name] = true
				}
			}
			break
		}
		if len(alternatives) > 1 {
			terms = append(terms, "("+strings.Join(alternatives, " | ")+")")
			continue
		}
		term := contentReferenceFlagSyntax(resource, action, flag)
		if !helpRequired(flag) {
			term = "[" + term + "]"
		}
		terms = append(terms, term)
	}
	return terms
}

func contentReferenceActionNotes(action *cobra.Command) []string {
	resource := action.Parent().Name()
	switch action.Name() {
	case "list":
		if action.Flags().Lookup("top-level") != nil {
			return []string{"--top-level=true: roots; false: nested; omitted: both."}
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

func writeCompactContentNotes(out io.Writer, resource *cobra.Command, actions []*cobra.Command) {
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
			fmt.Fprintln(out, "  Publish polls: 2s/30s, 5s/10m, then 15s. Save accepted IDs before waiting; never republish on recovery.")
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

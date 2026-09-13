package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// The pilot has explicit reference boundaries. Resolve the owner without looking
// at parsed values, so all help spellings and descendants render the same text.
func writeContentPilotHelp(out io.Writer, command *cobra.Command) bool {
	for node := command; node != nil; node = node.Parent() {
		parent := node.Parent()
		if parent == nil {
			continue
		}
		if node.Name() == "content" && parent.Parent() == nil && node == command {
			writeContentNavigation(out, node)
			return true
		}
		if parent.Name() == "content" && parent.Parent() != nil && parent.Parent().Parent() == nil {
			switch node.Name() {
			case "workbook", "datasource", "flow", "project":
				writeContentReference(out, node)
				return true
			}
		}
	}
	return false
}

func writeContentNavigation(out io.Writer, category *cobra.Command) {
	fmt.Fprintf(out, "usage: %s <resource> <verb> [flags]\n\nresources:\n", category.CommandPath())
	for _, resource := range visibleHelpChildren(category) {
		_, actions := helpNodes(resource)
		var verbs []string
		for _, action := range actions {
			verbs = append(verbs, action.Name())
		}
		fmt.Fprintf(out, "  %s: %s\n    %s\n", resource.Name(), contentResourceSummary(resource), strings.Join(verbs, ", "))
	}
	fmt.Fprintf(out, "\nUse %s <resource> -h for its reference; --help and %s help content <resource> also work.\n", category.CommandPath(), category.Root().Name())
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

type contentHelpEntry struct {
	text    string
	actions []string
}

func writeContentReference(out io.Writer, resource *cobra.Command) {
	if writeReviewedContentReference(out, resource) {
		return
	}
	_, actions := helpNodes(resource)
	fmt.Fprintf(out, "usage: %s <verb> [flags]\n%s: %s\n\nactions:\n", resource.CommandPath(), helpCommandName(resource), contentResourceSummary(resource))
	entries := map[string]*contentHelpEntry{}
	for _, action := range actions {
		fmt.Fprintf(out, "  %s: %s\n", helpDisplayPath(resource, action), contentActionSummary(action))
		for _, flag := range helpFlags(action) {
			text := contentFlagText(resource, action, flag)
			if entries[text] == nil {
				entries[text] = &contentHelpEntry{text: text}
			}
			entries[text].actions = append(entries[text].actions, action.Name())
		}
	}
	groups := map[string][]string{}
	for _, entry := range entries {
		scope := strings.Join(entry.actions, ",")
		if len(entry.actions) == len(actions) {
			scope = "shared"
		}
		groups[scope] = append(groups[scope], entry.text)
	}
	keys := sortedHelpKeys(groups)
	sort.SliceStable(keys, func(i, j int) bool {
		if keys[i] == "shared" || keys[j] == "shared" {
			return keys[i] == "shared"
		}
		return strings.Count(keys[i], ",") > strings.Count(keys[j], ",")
	})
	for _, scope := range keys {
		sort.Strings(groups[scope])
		fmt.Fprintf(out, "\nflags{%s}:\n", scope)
		writeContentOptions(out, groups[scope])
	}
	fmt.Fprintln(out, "\nnotes:")
	fmt.Fprintln(out, "  --help (-h): help only. Booleans: =true|false; bare=true, omitted=false unless stated.")
	fmt.Fprintln(out, "  Environment omitted: reads use read default; remote writes require exactly one configured environment.")
	fmt.Fprintln(out, "  Remote changes require enabled mutations; previews work when disabled and do not authorize enabling them.")
	writeContentConstraints(out, actions)
	writeContentNotes(out, resource, actions)
	writeContentBatches(out, resource, actions)
	var examples []string
	for _, action := range actions {
		if example, _, _ := strings.Cut(strings.TrimSpace(action.Example), "\n"); example != "" {
			examples = append(examples, example)
		}
	}
	writeExamples(out, strings.Join(examples, "\n"))
}

func writeContentOptions(out io.Writer, options []string) {
	line := "  "
	for _, option := range options {
		if len(line) > 2 && len(line)+len(option)+2 > 130 {
			fmt.Fprintln(out, line+",")
			line = "  "
		}
		if len(line) > 2 {
			line += ", "
		}
		line += option
	}
	if len(line) > 2 {
		fmt.Fprintln(out, line)
	}
}

func contentActionSummary(action *cobra.Command) string {
	summaries := map[string]string{
		"list":    "List matching remote content or cached records",
		"inspect": "Read one item's details without downloading files",
		"pull":    "Download content into a local workspace",
		"publish": "Publish local content to Tableau",
		"move":    "Move remote content to another project",
		"update":  "Change remote metadata",
		"delete":  "Delete remote content",
		"create":  "Create a remote project",
		"schema":  "Read published datasource tables and fields, not data values",
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

func contentFlagText(resource, action *cobra.Command, flag *pflag.Flag) string {
	// Copy presentation only; never mutate the live flag or consult its value.
	copy := *flag
	if helpRequired(flag) && len(flag.Annotations["tadx.help.omission"]) > 0 {
		copy.Annotations = make(map[string][]string, len(flag.Annotations))
		for key, values := range flag.Annotations {
			if key != "tadx.help.omission" {
				copy.Annotations[key] = append([]string(nil), values...)
			}
		}
	}
	summaries := map[string]string{
		"config": "non-secret config file", "json": "JSON instead of compact TOON", "full": "expanded bounded details, not more records",
		"environment": "environment selection (see notes)", "cache": "local cache only; no Tableau calls",
		"workspace": "registered workspace; deterministic default if omitted", "preview": "plan only; no local or remote writes",
		"batch-file": "per-item JSON (see batch)", "id": "authoritative LUID", "name": "exact name", "project": "exact project path",
		"project-name": "exact leaf project name, not path", "owner": "exact owner name", "tag": "exact tag",
		"all": "all matches, at most 10000", "limit": "maximum records", "cursor": "continuation cursor",
		"destination-project": "destination path", "destination-project-id": "destination LUID",
		"new-name": "replacement name", "owner-id": "replacement owner LUID", "description": "description",
		"content-permissions": "content permission mode", "parent": "parent path", "parent-id": "parent LUID",
		"artifact": "managed directory relative to workspace", "artifact-name": "unique exact managed item name",
		"as-job": "server job; still waits for completion", "include-extract": "include workbook extracts",
		"include-pds": "pull direct published datasources as siblings; no recursion",
		"type":        "exact provider datasource type", "updated-after": "inclusive UTC lower bound", "updated-before": "inclusive UTC upper bound",
		"query": "case-insensitive field ID/name/caption/label/formula text", "role": "exact field role", "table": "exact logical table caption",
		"field-id": "raw field ID; at most 10000 distinct", "descriptions": "direct/inherited descriptions and sources",
		"tags":   "upstream column tags and sources; published fields have no tag API",
		"create": "new datasource; fail on collision", "append": "append to exact collision", "replace": "replace data in exact collision",
	}
	if summary, ok := summaries[flag.Name]; ok {
		copy.Usage = summary
	}
	if action.Name() == "update" && flag.Name == "description" {
		copy.Usage = "description; empty clears"
	}
	if action.Name() == "list" {
		switch flag.Name {
		case "name", "project-id", "parent-id":
			copy.Usage = "exact filter"
		case "top-level":
			copy.Usage = "true: top-level only; false: nested only; omitted: both"
		}
	}
	if action.Name() == "publish" {
		switch flag.Name {
		case "file":
			extensions := map[string]string{"workbook": ".twb|.twbx", "datasource": ".tds|.tdsx", "flow": ".tfl|.tflx"}
			copy.Usage = "native " + extensions[resource.Name()] + " file"
		case "id":
			copy.Usage = "source LUID in workspace, not a remote target"
		case "name":
			copy.Usage = "published name; defaults to source name"
		case "project", "project-id":
			copy.Usage = "destination project"
		case "overwrite":
			copy.Usage = "replace exact remote collision"
		}
	}
	if action.Name() == "pull" && flag.Name == "overwrite" {
		copy.Usage = "replace dirty local artifact"
	}
	if resource.Name() == "project" {
		if flag.Name == "project-id" && action.Name() != "list" {
			copy.Usage = "legacy --id alias; cannot combine"
		}
		if flag.Name == "name" && action.Name() == "update" {
			copy.Usage = "legacy --new-name alias; cannot combine"
		}
	}
	text := strings.ReplaceAll(helpFlagText(&copy), "  ", " ")
	return strings.NewReplacer(
		"<"+resource.Name()+"-luid>", "<luid>",
		"<"+resource.Name()+"-name>", "<name>",
		"<workspace-relative-path>", "<path>",
		"<environment-name>", "<name>",
		"<workspace-name>", "<name>",
		"<artifact-name>", "<name>",
		"<destination-project-luid>", "<luid>",
		"<owner-luid>", "<luid>",
		"<parent-luid>", "<luid>",
		"<project-luid>", "<luid>",
		"<project-path>", "<path>",
	).Replace(text)
}

func writeContentConstraints(out io.Writer, actions []*cobra.Command) {
	entries := map[string][]string{}
	var order []string
	for _, action := range actions {
		var text strings.Builder
		writeRelationships(&text, "constraints", action)
		for _, line := range strings.Split(text.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "constraints:" {
				continue
			}
			if entries[line] == nil {
				order = append(order, line)
			}
			entries[line] = append(entries[line], action.Name())
		}
	}
	for _, line := range order {
		fmt.Fprintf(out, "  %s: %s\n", strings.Join(entries[line], ","), line)
	}
}

func writeContentNotes(out io.Writer, resource *cobra.Command, actions []*cobra.Command) {
	has := func(name string) bool {
		for _, action := range actions {
			if action.Name() == name {
				return true
			}
		}
		return false
	}
	fmt.Fprintln(out, "  Selectors are exact; ambiguous matches fail. Project paths use forward slashes.")
	if has("list") {
		fmt.Fprintln(out, "  list: bounded live reads do not touch cache. Live --all collects <=10000 and saves cache best-effort:")
		fmt.Fprintln(out, "    unfiltered replaces the resource scope; filtered saves observations. Cache failures warn without losing the live result.")
		fmt.Fprintln(out, "    --all rejects incomplete coverage/overflow; --cache remains local-only.")
	}
	if resource.Name() == "project" {
		if has("create") {
			fmt.Fprintln(out, "  create: omit parent selectors for a top-level project; omit content-permissions for the server default.")
		}
		return
	}
	fmt.Fprintln(out, "  Remote --id excludes --name/--project; list filters and publish selectors have their own meanings.")
	if has("pull") || has("publish") {
		fmt.Fprintln(out, "  Workspace: containing registered directory > environment default > global default; otherwise --workspace required.")
	}
	if has("publish") {
		fmt.Fprintf(out, "  publish: --artifact is artifacts/%s/<item>; --file selects a native file.\n", resource.Name())
		if resource.Name() != "datasource" {
			fmt.Fprintln(out, "  publish: creates by default; exact collisions require --overwrite. Destination project is always explicit.")
		}
	}
	if has("schema") {
		fmt.Fprintln(out, "  schema: --all excludes --limit; limit bounds fields, not tables. Descriptions/tags request metadata reads; --cache needs prior observations.")
		fmt.Fprintln(out, "  schema: --field-id must match after filters; --table resolves duplicate IDs. Embedded fields belong to workbook metadata.")
	}
}

func writeContentBatches(out io.Writer, resource *cobra.Command, actions []*cobra.Command) {
	var eligible []string
	selectors := map[string][]string{}
	for _, action := range actions {
		if action.Annotations["tadx.batch.file"] != "true" {
			continue
		}
		name := action.Name()
		eligible = append(eligible, name)
		var flags []string
		for _, selector := range strings.Split(action.Annotations["tadx.batch.selectors"], ",") {
			if selector != "" {
				flags = append(flags, "--"+selector)
			}
		}
		if len(flags) > 0 {
			key := strings.Join(flags, "|")
			selectors[key] = append(selectors[key], name)
		}
	}
	if len(eligible) == 0 {
		return
	}
	fmt.Fprintf(out, "\nbatch{%s}:\n", strings.Join(eligible, ","))
	for _, flags := range sortedHelpKeys(selectors) {
		fmt.Fprintf(out, "  %s: repeat %s (one dimension).\n", strings.Join(selectors[flags], ","), flags)
	}
	fmt.Fprintln(out, `  Or --batch-file <path>: {"items":[{"<flag-name>":"<value>"}]}; canonical names, arrays for list flags.`)
	fmt.Fprintln(out, "  Required inputs apply per row; rows override item flags. Different scalar selector pairs need separate rows.")
	fmt.Fprintln(out, "  Environment and invocation controls stay on the command; forbidden row keys: config, preview, json, full, raw, force, version, help, batch-file.")
	fmt.Fprintln(out, "  File or repeated selectors, not both; 1-100 items, <=1 MiB, <=100 expanded selections; duplicate items/JSON keys rejected.")
	fmt.Fprintln(out, "  Sequential items, ordered outcomes; other selector dimensions stay fixed.")
	if resource.Name() != "project" {
		fmt.Fprintln(out, "  Repeated --artifact cannot share --name; use per-item file rows.")
	}
}

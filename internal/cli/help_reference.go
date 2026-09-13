package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Operational references stop at the nearest resource. A verb never expands a
// different manual, and navigation never recursively prints action flags.
func writeStructuredHelp(out io.Writer, command *cobra.Command) {
	root := command.Root()
	if command == root {
		writeRootHelp(out, root)
		return
	}
	owner := command
	if helpAction(command) && command.Parent() != root && (!isHelpNavigation(command.Parent()) || command.Parent().Name() == "workspace") {
		owner = command.Parent()
	}
	if isHelpNavigation(owner) && owner.Name() != "workspace" {
		writeHelpNavigation(out, owner)
		return
	}
	var reference strings.Builder
	writeOperationalReference(&reference, owner)
	fmt.Fprintln(out, strings.TrimRight(reference.String(), "\n"))
}

func isHelpNavigation(command *cobra.Command) bool {
	for _, child := range visibleHelpChildren(command) {
		if !helpAction(child) && hasVisibleHelpChildren(child) {
			return true
		}
	}
	return false
}

func writeHelpNavigation(out io.Writer, category *cobra.Command) {
	fmt.Fprintf(out, "Usage: %s <resource> <verb> [flags]\n\nResources:\n", category.CommandPath())
	for _, child := range visibleHelpChildren(category) {
		fmt.Fprintf(out, "  %s: %s\n", child.Name(), helpPlainShort(child))
		var names []string
		for _, action := range visibleHelpChildren(child) {
			names = append(names, action.Name())
		}
		if len(names) > 0 {
			fmt.Fprintf(out, "    %s\n", strings.Join(names, ", "))
		}
	}
	fmt.Fprintf(out, "\nUse %s <resource> -h for complete syntax.\n", category.CommandPath())
	if category.Name() == "pulse" {
		fmt.Fprintln(out, "Saved configuration only; TADX does not retrieve current metric values or generated insights.")
	}
}

func directHelpActions(owner *cobra.Command) []*cobra.Command {
	if referenceLeaf(owner) {
		return []*cobra.Command{owner}
	}
	var actions []*cobra.Command
	for _, child := range visibleHelpChildren(owner) {
		if helpAction(child) {
			actions = append(actions, child)
		}
	}
	return actions
}

func referenceLeaf(command *cobra.Command) bool {
	return helpAction(command) || (command.Name() == "completion" && command.Parent() == command.Root() && command.Runnable())
}

func writeOperationalReference(out io.Writer, owner *cobra.Command) {
	actions := directHelpActions(owner)
	use := owner.CommandPath()
	if !referenceLeaf(owner) {
		use += " <verb>"
	} else {
		use += strings.TrimPrefix(owner.Use, owner.Name())
	}
	if owner.Name() == "completion" {
		use = strings.ReplaceAll(use, "<shell>", "<bash|zsh|fish|powershell>")
	}
	fmt.Fprintf(out, "Usage: %s [flags]\n\n", use)
	if isHelpNavigation(owner) {
		for _, child := range visibleHelpChildren(owner) {
			if !helpAction(child) {
				fmt.Fprintf(out, "%s: %s (%s %s -h)\n\n", child.Name(), helpPlainShort(child), owner.CommandPath(), child.Name())
			}
		}
	}
	shared := map[string]*pflag.Flag{}
	for _, action := range actions {
		for _, flag := range helpFlags(action) {
			if referenceSharedFlag(flag.Name) {
				shared[flag.Name] = flag
			}
		}
	}
	if len(shared) > 0 {
		fmt.Fprintln(out, "Shared:")
		for _, name := range sortedHelpKeys(shared) {
			fmt.Fprintf(out, "  %s\n", referenceSharedText(shared[name]))
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "Commands:")
	for _, action := range actions {
		name := action.Name() + strings.TrimPrefix(action.Use, action.Name())
		if action.Name() == "completion" {
			name = strings.ReplaceAll(name, "<shell>", "<bash|zsh|fish|powershell>")
		}
		fmt.Fprintf(out, "  %s: (%s)\n", name, referenceActionSummary(action))
		var terms []string
		for _, flag := range helpFlags(action) {
			if (shared[flag.Name] != nil && flag.Name != "cache" && flag.Name != "preview" && flag.Name != "workspace") || flag.Name == "batch-file" {
				continue
			}
			term := referenceFlagSyntax(flag)
			if !helpRequired(flag) {
				term = "[" + term + "]"
			}
			terms = append(terms, term)
		}
		writeReferenceTerms(out, terms)
		for _, note := range referenceActionNotes(action) {
			fmt.Fprintln(out, "    "+note)
		}
		fmt.Fprintln(out)
	}
	var rules strings.Builder
	writeReferenceConstraints(&rules, actions)
	for _, line := range referenceNotes(owner, actions) {
		fmt.Fprintln(&rules, "  "+line)
	}
	if rules.Len() > 0 {
		fmt.Fprintln(out, "Rules:")
		fmt.Fprint(out, rules.String())
		fmt.Fprintln(out)
	}
	writeReferenceBatch(out, actions)
	var examples []string
	for _, action := range actions {
		for _, line := range strings.Split(action.Example, "\n") {
			if strings.TrimSpace(line) != "" {
				examples = appendUnique(examples, strings.TrimSpace(line))
			}
		}
	}
	if len(examples) > 0 {
		fmt.Fprintln(out, "Examples:")
		for _, example := range examples {
			fmt.Fprintln(out, "  "+example)
		}
	}
}

func referenceSharedFlag(name string) bool {
	switch name {
	case "config", "environment", "full", "json", "cache", "preview", "workspace":
		return true
	}
	return false
}

func referenceSharedText(flag *pflag.Flag) string {
	switch flag.Name {
	case "config":
		return "--config <path>"
	case "environment":
		return "--environment (--env,-e) <name> (reads: default; writes: sole configured env)"
	case "full":
		return "--full (details, not rows)"
	case "json":
		return "--json"
	case "cache":
		return "--cache (where listed: local only; default: live)"
	case "preview":
		return "--preview (where listed: no writes; mutation gate may be off)"
	case "workspace":
		return "--workspace (--ws,-w) <name> (where listed: registered; directory > env > global)"
	}
	return referenceFlagSyntax(flag)
}

func referenceFlagSyntax(flag *pflag.Flag) string {
	text := helpFlagSyntax(flag, false)
	if _, repeatable := flag.Value.(pflag.SliceValue); repeatable || helpAnnotationTrue(flag, "tadx.help.repeatable") {
		text += "..."
	}
	if def := helpDeclaredDefault(flag); def != "" && def != "0" {
		text += " (default: " + def + ")"
	}
	if omitted := flag.Annotations["tadx.help.omission"]; len(omitted) > 0 && omitted[0] != "unchanged" {
		text += " (omitted: " + omitted[0] + ")"
	}
	return text
}

func writeReferenceTerms(out io.Writer, terms []string) {
	line := "    "
	for _, term := range terms {
		if len(line) > 4 && len(line)+len(term)+1 > 100 {
			fmt.Fprintln(out, line)
			line = "    "
		}
		if len(line) > 4 {
			line += " "
		}
		line += term
	}
	if len(line) > 4 {
		fmt.Fprintln(out, line)
	}
}

func writeReferenceBatch(out io.Writer, actions []*cobra.Command) {
	var names []string
	for _, action := range actions {
		if action.Annotations["tadx.batch.file"] == "true" {
			names = append(names, action.Name())
		}
	}
	if len(names) == 0 {
		return
	}
	fmt.Fprintf(out, "Batch (%s):\n", strings.Join(names, ", "))
	fmt.Fprintln(out, `  --batch-file <path>: {"items":[{"<flag-name>":"<value>"}]}`)
	for _, action := range actions {
		if action.Annotations["tadx.batch.file"] != "true" {
			continue
		}
		if selectors := action.Annotations["tadx.batch.selectors"]; selectors != "" {
			fmt.Fprintf(out, "  %s: repeat one of --%s instead of a file.\n", action.Name(), strings.ReplaceAll(selectors, ",", " | --"))
		}
		if action.Annotations["tadx.batch.positional"] == "true" {
			fmt.Fprintf(out, "  %s: repeat positional targets; file rows use args:[\"<value>\"].\n", action.Name())
		}
	}
	fmt.Fprintln(out, "  Rows: canonical flag keys; arrays for lists; required per row; override item flags.")
	fmt.Fprintln(out, "  Env/control flags outside rows. File OR repeated selectors; 1-100 sequential items, <=1MiB/100 selections.")
	fmt.Fprintln(out)
}

func referenceNotes(owner *cobra.Command, actions []*cobra.Command) []string {
	var notes []string
	for _, action := range actions {
		for _, flag := range helpFlags(action) {
			if flag.Name == "preview" && action.Name() != "pull" && (strings.HasPrefix(owner.CommandPath(), owner.Root().Name()+" admin ") || strings.HasPrefix(owner.CommandPath(), owner.Root().Name()+" catalog ") || strings.HasPrefix(owner.CommandPath(), owner.Root().Name()+" pulse ")) {
				notes = appendUnique(notes, "Remote writes require enabled mutations; tadx shows the current setting.")
			}
			if omitted := flag.Annotations["tadx.help.omission"]; len(omitted) > 0 && omitted[0] == "unchanged" {
				notes = appendUnique(notes, "Update: omitted settings stay unchanged.")
			}
		}
	}
	if owner.CommandPath() == owner.Root().Name()+" admin permission" {
		for _, action := range actions {
			if action.Name() == "create" {
				if _, tail, ok := strings.Cut(action.Long, "Supported capability names by resource kind:"); ok {
					notes = appendUnique(notes, "Capabilities by kind:")
					for _, line := range strings.Split(tail, "\n") {
						if line = strings.TrimSpace(line); line != "" {
							notes = appendUnique(notes, line)
						}
					}
				}
			}
		}
	}
	if owner.Name() == "user" || owner.Name() == "group" {
		notes = appendUnique(notes, "Site roles (site/version dependent): Viewer, Explorer, ExplorerCanPublish, Creator, SiteAdministratorExplorer, SiteAdministratorCreator, Unlicensed.")
	}
	notes = appendUnique(notes, "Booleans: =true|false (bare=true); ... means repeatable.")
	return notes
}

func referenceActionNotes(action *cobra.Command) []string {
	path := strings.TrimPrefix(action.CommandPath(), action.Root().Name()+" ")
	notes := map[string][]string{
		"pulse definition create":   {"Fields: raw ID or unique caption. Find them with tadx content datasource schema.", "Omitted: aggregation SUM, granularity DAY, format NUMBER, sentiment NONE, temporality OVER_TIME."},
		"pulse definition publish":  {"Source --id/--artifact-name refers to a workspace bundle; creates new objects, not updates. Map every datasource."},
		"pulse definition pull":     {"--overwrite replaces dirty local files. Bundle includes saved variants, not followers."},
		"pulse metric fork":         {"--days only with CUSTOM_N_DAYS (required). Filters: repeat field=value; values are literal.", "Unchanged fields/period inherit the source; selected filter fields replace their prior values."},
		"admin user create":         {"--name is the unique login, not full name. Site roles/auth methods depend on the site."},
		"admin user inspect":        {"--name matches the exact unique login, not full name."},
		"catalog database inspect":  {"--id: REST LUID; --metadata-id: Metadata API identity."},
		"catalog table inspect":     {"--id: REST LUID; --metadata-id: Metadata API identity."},
		"catalog column inspect":    {"--id: column LUID with --table-id; --metadata-id selects directly."},
		"catalog database update":   {"Empty --description/contact-id clears it. Tags: repeated exact values."},
		"catalog table update":      {"Empty --description/contact-id clears it. Tags: repeated exact values."},
		"catalog column update":     {"Changes upstream column metadata, not the published field override. Empty --description clears."},
		"cache refresh":             {"Default: all inventory except permissions. --scope permissions opts into per-item permission reads.", "Refresh replaces requested inventory; independent schema/Pulse observations keep their timestamps."},
		"auth login":                {"Requires --environment and an interactive terminal; prompts for PAT name/secret. Environment credentials take precedence."},
		"auth logout":               {"Removes stored credentials, not the Tableau PAT; configured environment credentials remain usable."},
		"mutation set":              {"Changes write permission, not credentials. Obtain explicit approval for the requested scope."},
		"workspace create":          {"Default root: <home>/TADX/workspaces/<name>; --path overrides it."},
		"workspace clone":           {"Default root: <home>/TADX/workspaces/<name>; --path overrides it."},
		"workspace delete":          {"Deletes the registered root; --force acknowledges dirty/invalid artifacts. Unregister keeps files."},
		"workspace register":        {"Uses existing tadx.yaml identity; optional name must agree."},
		"workspace clean":           {"Only disposable state; canonical artifact files are preserved."},
		"workspace artifact delete": {"Deletes local files only, never the remote content."},
		"agent install":             {"--target auto detects installed harnesses. Installs current TADX-owned skills globally; --force is compatibility-only."},
		"agent uninstall":           {"Removes TADX-owned skills only; edited packages are backed up. --force is compatibility-only."},
		"env add":                   {"--site is the URL slug, not display name. PAT flags name shell variables, never contain credentials."},
		"env update":                {"--clear-* restores defaults or clears the corresponding value. Retargeting does not move content."},
		"env default":               {"Changes the read default, not write targets; with multiple environments, remote writes need --environment."},
		"search":                    {"Term omitted: lists matching types. Live uses Tableau search; --cache searches local observations only."},
		"catalog search":            {"Searches upstream metadata; column search requires --table-id. Types may repeat, without duplicates."},
		"catalog audit":             {"Checks metadata coverage, not data values. Repeated checks must be distinct."},
		"catalog lineage pull":      {"--name requires --project; --id excludes both. --overwrite replaces dirty local metadata.", "Saves lineage.json, not native files. Physical nodes use Metadata API IDs; --full shows bounded nodes/edges."},
		"catalog label update":      {"Changes an asset attachment, not the shared label definition. Empty message clears it."},
		"update":                    {"Updates the executable and installed Guidance; --target may repeat (up to 32)."},
		"last":                      {"Displays the last saved result; never repeats its command or writes."},
		"completion":                {"Writes a shell script to stdout (not JSON). Installers normally enable it automatically.", "Session: Bash source <(tadx completion bash); Fish tadx completion fish | source.", "Zsh: autoload -Uz compinit; compinit; source <(tadx completion zsh)", "PowerShell: tadx completion powershell | Out-String | Invoke-Expression", "For persistence, put the corresponding command in your shell profile."},
	}
	return notes[path]
}

func referenceActionSummary(action *cobra.Command) string {
	path := strings.TrimPrefix(action.CommandPath(), action.Root().Name()+" ")
	summaries := map[string]string{
		"workspace create": "Create a registered local workspace", "workspace clone": "Copy a workspace under a new identity",
		"workspace delete": "Delete a workspace and its files", "workspace unregister": "Forget registration; keep files",
		"workspace register": "Register an existing workspace", "workspace status": "Inspect local artifacts and dirty state",
		"workspace clean": "Remove disposable local state", "workspace set-default": "Set the global workspace default",
		"agent install": "Install or refresh bundled skills", "agent uninstall": "Remove bundled skills",
		"capability get": "Inspect one capability's availability and contract", "capability list": "List capabilities",
		"cache refresh": "Collect inventory into the local cache", "cache status": "Inspect cache age and coverage",
	}
	if summary, ok := summaries[path]; ok {
		return summary
	}
	return strings.TrimSuffix(helpPlainShort(action), ".")
}

func writeReferenceConstraints(out io.Writer, actions []*cobra.Command) {
	entries := map[string][]string{}
	var order []string
	for _, action := range actions {
		var raw strings.Builder
		writeRelationships(&raw, "constraints", action)
		for _, line := range strings.Split(raw.String(), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || line == "constraints:" {
				continue
			}
			if tail, ok := strings.CutPrefix(line, "mutually exclusive: "); ok && strings.Contains(raw.String(), "exactly one of: "+tail+"\n") {
				continue
			}
			if entries[line] == nil {
				order = append(order, line)
			}
			entries[line] = appendUnique(entries[line], action.Name())
		}
	}
	for _, line := range order {
		fmt.Fprintf(out, "  %s: %s\n", strings.Join(entries[line], ","), line)
	}
}

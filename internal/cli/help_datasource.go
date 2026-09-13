package cli

import (
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// This small resource-specific syntax sheet shares selectors, not leaf manuals.
// Inventory and alias spelling still come from the completed command tree.
func writeDatasourceReference(out io.Writer, resource *cobra.Command) {
	_, actions := helpNodes(resource)
	flags := map[string]*pflag.Flag{}
	for _, action := range actions {
		for _, flag := range helpFlags(action) {
			flags[flag.Name] = flag
		}
	}
	seen := map[string]bool{}
	pattern := regexp.MustCompile(`--[a-z][a-z-]*`)
	line := func(text string) {
		text = pattern.ReplaceAllStringFunc(text, func(name string) string {
			flag := flags[strings.TrimPrefix(name, "--")]
			if seen[name] || flag == nil {
				return name
			}
			seen[name] = true
			label, _, _ := strings.Cut(helpFlagSyntax(flag, true), " <")
			return strings.ReplaceAll(label, ", ", ",")
		})
		for len(text) > 145 {
			cut := strings.LastIndex(text[:145], " ")
			if cut <= 2 {
				break
			}
			fmt.Fprintln(out, text[:cut])
			text = "  " + strings.TrimSpace(text[cut:])
		}
		fmt.Fprintln(out, text)
	}
	line("usage: " + resource.Parent().CommandPath() + " " + helpCommandName(resource) + " <verb> [flags]")
	line("shared: --environment <name> (reads:default; writes:sole configured env), --config <path>, --json, --full (details, not rows), --help(-h)")
	line("target: --id <luid> | (--name <name> --project <path>) (exact)")
	line("options: --cache (local only; default live), --preview (no writes; mutation gate may be off), --workspace <name> (registered; directory > env > global)")
	formats := map[string]string{
		"list":    "[--name <name>,--owner <name>,--project-name <leaf>,--type <provider>,--tag <tag>,--updated-after <RFC3339>,--updated-before <RFC3339>] (exact; inclusive UTC) [--limit <1..10000> (default 25)|--all] [--cache]",
		"inspect": "(details) target [--cache]",
		"pull":    "(download) target [--workspace] [--overwrite] (dirty local files) [--preview]",
		"publish": "(upload) source (--project-id <luid>|--project <path>) (destination) (--create|--overwrite|--append|--replace) [--name <name>] (default: source) [--workspace] [--as-job] (waits) [--preview]",
		"schema":  "(fields/tables) --id <luid> [--query <text>] (ID/name/caption/label/formula substring, ignore case) [--role <measure|dimension|date|excluded>,--table <caption>,--field-id <id>...] [--limit <1..10000> (default 20)|--all] [--descriptions,--tags,--cache]",
		"update":  "target [--new-name <name>] [--owner-id <luid>] (one required; omitted unchanged) [--preview]",
		"move":    "target (--destination-project-id <luid>|--destination-project <path>) [--preview]",
		"delete":  "(remote) target [--preview]",
	}
	line("source: --artifact <artifacts/datasource/item> | --file <file.tds|file.tdsx> | --id <luid> | --artifact-name <name> (ID/unique name: workspace item)")
	for _, action := range actions {
		text, ok := formats[action.Name()]
		if !ok {
			text = helpPlainShort(action)
		}
		line(helpCommandName(action) + ": " + text)
	}
	line("publish: create (collision fails), overwrite (replace collision), append/replace (data). Remote writes require enabled mutations.")
	line("schema: field IDs repeat <=10000, must match filters (table disambiguates). Descriptions (inherited), tags (upstream columns); both include sources; cache needs prior observations.")
	line("list --all (incomplete/overflow fails; may save cache).")
	var batch []string
	for _, action := range actions {
		if action.Annotations["tadx.batch.file"] == "true" {
			batch = append(batch, action.Name())
		}
	}
	if len(batch) > 0 {
		line("batch: " + strings.Join(batch, ",") + "; repeat one selector OR --batch-file <path>:")
		line(`{"items":[{"id":"<luid>"}]} (flag keys; lists use arrays; rows override CLI; required per row)`)
		line("repeat id (delete/move/schema/update), id|name (inspect/pull), source (publish; repeated artifact excludes name).")
		line("(env/control flags outside rows; 1-100 sequential items, <=1MiB/100 selections)")
	}
	// A newly registered flag remains visible until its concise syntax is curated.
	for _, name := range sortedHelpKeys(flags) {
		if !seen["--"+name] {
			line(helpFlagText(flags[name]))
		}
	}
	line("examples:")
	line(resource.CommandPath() + " publish --file x.tds --project Ops --create --preview")
	line(resource.CommandPath() + " schema --id <luid> --role measure")
}

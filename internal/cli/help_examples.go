package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

// Category examples show complete invocations, with angle brackets for values
// supplied by the caller. These are presentation metadata, not validation rules.
type helpExampleSet struct {
	path     string
	note     string
	examples []string
}

var categoryHelpExamples = []helpExampleSet{
	{"auth", "Authentication uses PATs. Login prompts for credentials and stores them in the OS credential store.", []string{
		"tadx auth status --env dev", "tadx auth check --env dev", "tadx auth login --env dev",
	}},
	{"env", "Profiles contain non-secret settings. The site value is the Tableau site content URL.", []string{
		"tadx env list", "tadx env add dev --url https://tableau.example.com --site <site-content-url>", "tadx env default dev",
	}},
	{"cache", "Reads contact Tableau by default. Pass --cache on supported read commands to use the local inventory.", []string{
		"tadx cache status --env dev", "tadx cache refresh --env dev --scope workbooks --scope datasources",
	}},
	{"catalog", "Catalog inspects upstream asset metadata, captures lineage, and manages attached labels. REST LUIDs and Metadata API IDs are distinct selectors.", []string{
		"tadx catalog search <query> --env dev --type database --type table", "tadx catalog audit --env dev --type datasource --id <datasource-luid>",
	}},
	{"catalog database", "Inspect accepts exactly one of --id or --metadata-id. Updates require the REST LUID in --id.", []string{
		"tadx catalog database list --env dev", "tadx catalog database inspect --env dev --id <database-luid> --full",
	}},
	{"catalog table", "Inspect accepts exactly one of --id or --metadata-id. Updates require the REST LUID in --id.", []string{
		"tadx catalog table list --env dev --database-id <database-luid>", "tadx catalog table inspect --env dev --id <table-luid> --full",
	}},
	{"catalog column", "List requires --table-id. Inspect requires --id with --table-id, or --metadata-id alone.", []string{
		"tadx catalog column list --env dev --table-id <table-luid>", "tadx catalog column inspect --env dev --table-id <table-luid> --id <column-luid>",
	}},
	{"content", "Selectors use exact names, project paths, or authoritative LUIDs. Ambiguous names fail.", []string{
		"tadx content workbook list --env dev", "tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>",
	}},
	{"content workbook", "Inspect requires --id, or both --name and --project. Pull accepts --id or --name, with --project to scope names.\nPublish requires an artifact selector and a destination project.", []string{
		"tadx content workbook list --env dev --limit 10",
		"tadx content workbook pull --env dev --workspace dev --id <workbook-luid> --id <second-workbook-luid>",
		"tadx content workbook publish --env dev --workspace dev --id <workbook-luid> --project-id <project-luid> --preview",
	}},
	{"content datasource", "Inspect and pull accept --id, or --name with --project. Schema requires --id.\nPublish requires exactly one of --create, --overwrite, --append, or --replace.", []string{
		"tadx content datasource list --env dev", "tadx content datasource schema --env dev --id <datasource-luid> --role measure",
		"tadx content datasource publish --env dev --workspace dev --id <datasource-luid> --project-id <project-luid> --create --preview",
	}},
	{"content datasource schema", "Repeat --field-id for exact fields. Use --all or --limit, but not both.", []string{
		"tadx content datasource schema --env dev --id <datasource-luid> --query <field-term>",
		"tadx content datasource schema --env dev --id <datasource-luid> --field-id <field-id> --field-id <second-field-id> --descriptions",
	}},
	{"content flow", "Inspect and pull require --id, or both --name and --project. Publish requires an artifact selector and a destination project.", []string{
		"tadx content flow list --env dev", "tadx content flow pull --env dev --workspace dev --id <flow-luid>",
		"tadx content flow publish --env dev --workspace dev --id <flow-luid> --project-id <project-luid> --preview",
	}},
	{"content project", "Create requires --name. Inspect and update accept --id or --project; delete requires --id. Project paths use forward slashes.", []string{
		"tadx content project list --env dev --top-level", "tadx content project inspect --env dev --project <project-path>",
		"tadx content project create --env dev --name <project-name> --parent-id <parent-project-luid> --preview",
	}},
	{"catalog lineage", "Pull requires --kind and an exact --id or --name selector. Depth ranges from 1 to 3.", []string{
		"tadx catalog lineage pull --env dev --workspace dev --kind workbook --id <workbook-luid> --direction upstream --depth 2",
	}},
	{"catalog label", "List requires --type and --target-id. Inspect and delete require the attachment --id.\nUpdate requires --id, or --type and --target-id with --value. Supply at least one label change.", []string{
		"tadx catalog label list --env dev --type datasource --target-id <datasource-luid>",
		"tadx catalog label update --env dev --id <label-luid> --message <message> --preview",
	}},
	{"admin", "Use --preview to inspect supported remote changes. Users, groups, and permission principals use exact selectors.", []string{
		"tadx admin user list --env dev", "tadx admin group list --env dev", "tadx admin permission inspect --env dev --kind workbook --id <workbook-luid>",
	}},
	{"admin user", "Create requires --name, --site-role, and exactly one of --auth-setting or --idp-configuration-id.\nInspect accepts --id or --name; update and delete accept --id or --username.", []string{
		"tadx admin user list --env dev --site-role Viewer",
		"tadx admin user create --env dev --name <username> --site-role Viewer --auth-setting ServerDefault --preview",
		"tadx admin user update --env dev --username <username> --site-role Explorer --preview",
	}},
	{"admin group", "Create requires --name. Inspect accepts --id or --name. Update and delete require --id.", []string{
		"tadx admin group inspect --env dev --name <group-name>", "tadx admin group create --env dev --name <group-name> --preview",
	}},
	{"admin group member", "Add and remove require --group-id and exactly one of --user-id or --username.", []string{
		"tadx admin group member add --env dev --group-id <group-luid> --username <username> --preview",
		"tadx admin group member remove --env dev --group-id <group-luid> --user-id <user-luid> --preview",
	}},
	{"admin permission", "Inspect requires --kind and --id. Create and delete also require --principal-type, a principal selector, --capability, and --mode.\nUse --principal-id, or --principal-username with --principal-type user. Repeat --capability for multiple rules.", []string{
		"tadx admin permission inspect --env dev --kind workbook --id <workbook-luid>",
		"tadx admin permission create --env dev --kind workbook --id <workbook-luid> --principal-type group --principal-id <group-luid> --capability Read --mode Allow --preview",
	}},
	{"admin label", "Shared label definitions are separate from labels attached to assets under catalog label.", []string{
		"tadx admin label value list --env dev", "tadx admin label category list --env dev",
	}},
	{"admin label value", "Inspect, update, and delete select an exact --name. Creating a value through update also requires --category.", []string{
		"tadx admin label value inspect --env dev --name <label-value>",
		"tadx admin label value update --env dev --name <label-value> --description <description> --preview",
	}},
	{"admin label category", "Create, inspect, update, and delete select an exact --name.", []string{
		"tadx admin label category list --env dev",
		"tadx admin label category create --env dev --name <category-name> --description <description> --preview",
	}},
	{"pulse", "", []string{
		"tadx pulse definition list --env dev", "tadx pulse metric list --env dev --definition-id <definition-luid>",
	}},
	{"pulse definition", "Create requires --name, --datasource-id, --measure-field, and --date-field. Repeat --dimension for allowed slicers.\nPublish requires one of --artifact, --id, or --artifact-name, plus explicit --datasource-map source=destination mappings.", []string{
		"tadx pulse definition inspect --env dev --id <definition-luid>",
		"tadx pulse definition create --env dev --name <definition-name> --datasource-id <datasource-luid> --measure-field <measure-field> --date-field <date-field> --dimension <dimension-field> --preview",
		"tadx pulse definition publish --env dev --workspace dev --id <definition-luid> --datasource-map <source-luid>=<destination-luid> --preview",
	}},
	{"pulse metric", "Fork requires --id and at least one of --period, --filter, or --exclude-filter. CUSTOM_N_DAYS also requires --days.\nFollow requires --id and exactly one of --user-id or --group-id.", []string{
		"tadx pulse metric list --env dev --definition-id <definition-luid>",
		"tadx pulse metric fork --env dev --id <metric-luid> --filter <field>=<value> --filter <field>=<second-value> --preview",
		"tadx pulse metric follow --env dev --id <metric-luid> --user-id <user-luid> --preview",
	}},
	{"pulse metric followers", "", []string{"tadx pulse metric followers --env dev --id <metric-luid>"}},
	{"workspace", "Workspace names select registered local roots. Managed artifact paths are relative to those roots and use forward slashes.", []string{
		"tadx workspace create dev", "tadx workspace status --workspace dev", "tadx workspace set-default dev",
	}},
	{"workspace artifact", "Select exactly one --artifact path, or both --kind and --id.", []string{
		"tadx workspace artifact delete --workspace dev --kind workbook --id <workbook-luid> --preview",
	}},
	{"agent", "Use --target auto to detect configured agents. Preview reports local file changes without writing them.", []string{
		"tadx agent install --target auto --preview", "tadx agent install --target auto",
	}},
	{"mutation", "Mutation set persists policy for future sessions. Agents need explicit permission for the setting change and its persistent scope.\nTADX_ENABLE_MUTATIONS overrides saved policy for the process. Supported read-only previews remain available when mutations are disabled.", []string{
		"tadx mutation status", "tadx mutation set --enabled=true",
	}},
	{"capability", "Use capability IDs to inspect ownership, availability, and bounded operation details.", []string{
		"tadx capability list --domain content", "tadx capability get workbook.pull",
	}},
}

func applyHelpExamples(root *cobra.Command) {
	for _, entry := range categoryHelpExamples {
		command := helpCommandAt(root, strings.Fields(entry.path))
		if command == nil {
			continue
		}
		if helpCategoryNoteIsCommon(entry.path) {
			appendHelpNote(command, entry.note)
		}
		for _, example := range entry.examples {
			words := strings.Fields(example)
			path := words[1:]
			target, _, err := root.Find(path)
			if err != nil || !target.Runnable() || target == root || !usefulHelpExample(example) {
				continue
			}
			if !strings.Contains(target.Example, example) {
				target.Example = strings.TrimSpace(target.Example + "\n" + example)
			}
		}
	}
	var addSyntax func(*cobra.Command)
	addSyntax = func(command *cobra.Command) {
		if command.Name() == "publish" && command.Parent() != nil && command.Parent().Parent() != nil && command.Parent().Parent().Name() == "content" {
			appendHelpNote(command, "Select one source: --artifact, --file, --id, or --artifact-name. Choose a destination with --project-id or --project.\nRepeat --artifact for a same-action batch. Managed artifact paths are workspace-relative and use forward slashes.")
			if command.Parent().Name() == "datasource" {
				appendHelpNote(command, "Select exactly one publish mode: --create, --overwrite, --append, or --replace.")
			}
		}
		for _, child := range command.Commands() {
			addSyntax(child)
		}
	}
	addSyntax(root)
}

func helpCategoryNoteIsCommon(path string) bool {
	switch path {
	case "auth", "env", "cache", "catalog", "content", "admin", "admin label", "workspace", "mutation", "capability":
		return true
	default:
		return false
	}
}

func usefulHelpExample(example string) bool {
	for _, syntax := range []string{" schema ", " publish ", " fork ", "permission create", "user create", "--parent-id", "workspace artifact", "--batch-file"} {
		if strings.Contains(example, syntax) {
			return true
		}
	}
	seen := map[string]bool{}
	for _, word := range strings.Fields(example) {
		if !strings.HasPrefix(word, "--") {
			continue
		}
		if seen[word] {
			return true
		}
		seen[word] = true
	}
	return false
}

func appendHelpNote(command *cobra.Command, note string) {
	if note == "" || strings.Contains(command.Long, note) {
		return
	}
	if command.Long == "" {
		command.Long = command.Short
	}
	command.Long = strings.TrimSpace(command.Long) + "\n\n" + note
}

func helpCommandAt(root *cobra.Command, path []string) *cobra.Command {
	command := root
	for _, name := range path {
		var next *cobra.Command
		for _, child := range command.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			return nil
		}
		command = next
	}
	return command
}

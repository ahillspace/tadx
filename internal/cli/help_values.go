package cli

import (
	"strconv"
	"strings"

	metricfork "github.com/ahillspace/tadx/actions/pulse/metric/fork"
	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// applyHelpValues adds presentation metadata only. Scope by canonical command
// path: identically named flags can have different contracts in other actions.
func applyHelpValues(root *cobra.Command) {
	var visit func(*cobra.Command, string)
	visit = func(command *cobra.Command, path string) {
		applyHelpSemantics(command, path)
		choices := func(name string, values ...string) { helpFlagAnnotation(command, name, "tadx.help.choices", values) }
		value := func(name, format string) { helpFlagAnnotation(command, name, "tadx.help.value", []string{format}) }
		note := func(name, text string) { appendHelpUsage(command.Flags().Lookup(name), text) }
		switch path {
		case "search":
			types, _ := searchaction.Types("")
			choices("type", append(types, "content", "admin", "pulse")...)
			value("limit", "1..2000")
		case "content datasource schema":
			choices("role", "measure", "dimension", "date", "excluded")
			note("field-id", "at most 10000 distinct identifiers")
		case "catalog search":
			choices("type", "database", "table", "column")
			note("type", "values must be unique; column requires --table-id")
		case "catalog audit":
			choices("type", "database", "table", "datasource")
			choices("check", "descriptions", "tags")
			note("check", "values must be unique")
		case "cache refresh":
			choices("scope", "users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions")
			note("scope", "values must be unique; comma-separated values are also accepted")
		case "content project create", "content project update":
			choices("content-permissions", "ManagedByOwner", "LockedToProject", "LockedToProjectWithoutNested")
		case "admin permission inspect", "admin permission create", "admin permission delete":
			choices("kind", "workbook", "datasource", "flow", "project")
			choices("default-for", "workbooks", "datasources", "flows")
			choices("principal-type", "user", "group")
			choices("mode", "Allow", "Deny")
			note("default-for", "requires --kind project")
			// The action's Long help already receives authoritative per-kind
			// capability lists through its composition-root dependency.
		case "admin user create", "admin user update":
			choices("auth-setting", "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA")
			note("site-role", "accepted roles depend on the Tableau site and version")
			note("language", "Tableau language code supported by the site")
			note("locale", "Tableau locale code supported by the site")
		case "admin group create", "admin group update":
			note("minimum-site-role", "accepted roles depend on the Tableau site and version")
		case "catalog lineage pull":
			choices("kind", "workbook", "datasource", "published_datasource", "flow")
			choices("direction", "upstream", "downstream", "both")
			value("depth", "1..3")
		case "workspace clean":
			choices("class", "temporary", "cache", "logs", "all")
		case "workspace artifact delete", "workspace artifact move":
			choices("kind", "workbook", "datasource", "flow", "pulse-definition", "lineage")
		case "catalog label list", "catalog label inspect", "catalog label update", "catalog label delete":
			choices("type", "database", "table", "column", "datasource", "flow")
		case "pulse definition create":
			choices("aggregation", "SUM", "AVERAGE", "MIN", "MAX", "COUNT", "COUNT_DISTINCT", "USER")
			choices("minimum-granularity", "DAY", "WEEK", "MONTH", "QUARTER", "YEAR")
			choices("number-format", "NUMBER", "CURRENCY", "PERCENT")
			choices("sentiment", "UP", "DOWN", "NONE")
			choices("temporality", "OVER_TIME", "LATEST")
			value("currency", "AAA")
			note("name", "at most 255 Unicode characters")
			note("description", "at most 1024 Unicode characters")
		case "pulse metric fork":
			choices("period", "TODAY", "THIS_WEEK", "MONTH_TO_DATE", "QUARTER_TO_DATE", "YEAR_TO_DATE", "YESTERDAY", "LAST_WEEK", "LAST_MONTH", "LAST_QUARTER", "LAST_YEAR", "LAST_7_DAYS", "LAST_14_DAYS", "LAST_30_DAYS", "LAST_60_DAYS", "LAST_90_DAYS", "CUSTOM_N_DAYS")
			var days []string
			for _, day := range metricfork.SupportedCustomDays() {
				days = append(days, strconv.Itoa(day))
			}
			choices("days", days...)
			note("days", "required with --period CUSTOM_N_DAYS; otherwise omit")
			note("period", "the source definition must allow the selected period's granularity")
			value("filter", "field=value")
			value("exclude-filter", "field=value")
		case "pulse definition publish":
			value("datasource-map", "source-luid=destination-luid")
		case "content datasource list":
			value("updated-after", "RFC3339")
			value("updated-before", "RFC3339")
			note("type", "exact provider datasource type; not a search resource family")
		case "update":
			note("target", "at most 32 entries")
		case "env add", "env update":
			value("cache-max-concurrency", "1..256")
			if path == "env add" {
				helpFlagAnnotation(command, "cache-max-concurrency", "tadx.help.default", []string{"32"})
			} else {
				helpFlagAnnotation(command, "cache-max-concurrency", "tadx.help.omission", []string{"unchanged"})
			}
			value("url", "https://server")
			value("api-version", "major.minor")
		}
		if limit, ok := helpLimitDefaults[path]; ok {
			value("limit", "1..10000")
			helpFlagAnnotation(command, "limit", "tadx.help.default", []string{limit})
		}
		applyHelpRequirements(command, path)
		for _, child := range command.Commands() {
			visit(child, strings.TrimSpace(path+" "+child.Name()))
		}
	}
	visit(root, "")
}

// Semantic names explain existing values without changing accepted syntax.
func applyHelpSemantics(command *cobra.Command, path string) {
	resource := "resource"
	parts := strings.Fields(path)
	if len(parts) > 1 {
		resource = parts[len(parts)-2]
	}
	for _, flag := range collectEffectiveFlags(command) {
		if len(flag.Annotations["tadx.help.value"]) > 0 || len(flag.Annotations["tadx.help.choices"]) > 0 {
			continue
		}
		kind := flag.Value.Type()
		if kind != "string" && kind != "stringArray" && kind != "stringSlice" {
			continue
		}
		value := flag.Name
		switch flag.Name {
		case "config", "file", "batch-file", "path", "source", "destination":
			value = "path"
		case "environment":
			value = "environment-name"
		case "workspace":
			value = "workspace-name"
		case "artifact":
			value = "workspace-relative-path"
		case "id":
			value = resource + "-luid"
			if resource == "artifact" || resource == "permission" || resource == "lineage" {
				value = "resource-luid"
			}
		case "name", "new-name":
			value = resource + "-name"
		case "project", "parent", "destination-project":
			value = "project-path"
		case "query":
			value = "text"
		case "description", "message":
			value = "text"
		case "cursor":
			value = "cursor"
		case "pat-name-env", "pat-secret-env":
			value = "environment-variable"
		case "site":
			value = "site-content-url"
		case "url":
			value = "https://server"
		case "metadata-id":
			value = "metadata-api-id"
		case "field-id":
			value = "field-id"
		case "measure-field", "date-field", "dimension":
			value = "field-id-or-name"
		default:
			if strings.HasSuffix(flag.Name, "-id") {
				value = strings.TrimSuffix(flag.Name, "-id") + "-luid"
			}
		}
		helpFlagAnnotation(command, flag.Name, "tadx.help.value", []string{value})
	}
	if command.Name() == "update" {
		for _, name := range []string{"new-name", "full-name", "email", "description", "owner-id", "site-role", "auth-setting", "idp-configuration-id", "identity-pool", "language", "locale", "content-permissions", "minimum-site-role", "external-user-enabled", "contact-id", "value", "message", "active", "elevated"} {
			helpFlagAnnotation(command, name, "tadx.help.omission", []string{"unchanged"})
		}
		if strings.HasPrefix(path, "env ") {
			for _, name := range []string{"url", "site", "api-version", "pat-name-env", "pat-secret-env", "default-workspace"} {
				helpFlagAnnotation(command, name, "tadx.help.omission", []string{"unchanged"})
			}
		}
	}
	if command.Parent() != nil && command.Parent().Parent() == nil {
		summaries := map[string]string{
			"search": "Find content, users, groups, and Pulse objects", "content": "Workbooks, datasources, flows, and projects",
			"catalog": "Upstream metadata, lineage, and attached labels", "admin": "Users, groups, memberships, permissions, and label definitions",
			"pulse": "Definitions, metric variants, and followers", "cache": "Refresh and inspect local Tableau inventory",
			"workspace": "Registered local workspaces and downloaded files", "last": "Previous saved result, without rerunning it",
			"env": "Tableau site profiles and defaults", "auth": "PAT login, checks, and logout", "mutation": "Mutation execution policy",
			"agent": "Install and remove bundled agent guidance", "doctor": "Diagnose configuration and connectivity",
			"capability": "Feature availability and implementation inventory", "update": "Update the CLI and guidance",
			"version": "Installed version and release checks", "completion": "Shell completion setup",
		}
		if text := summaries[command.Name()]; text != "" {
			if command.Annotations == nil {
				command.Annotations = map[string]string{}
			}
			command.Annotations["tadx.help.summary"] = text
		}
	}
}

func applyHelpRequirements(command *cobra.Command, path string) {
	required := func(names ...string) {
		for _, name := range names {
			helpFlagAnnotation(command, name, "tadx.help.required", []string{"true"})
		}
	}
	group := func(relation string, names ...string) {
		var present []string
		for _, name := range names {
			if command.Flags().Lookup(name) != nil {
				present = append(present, name)
			}
		}
		if len(present) < 2 {
			return
		}
		for _, name := range present {
			helpFlagAnnotation(command, name, "tadx.help."+relation, []string{strings.Join(present, " ")})
		}
	}
	note := func(text string) {
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		if !strings.Contains(command.Annotations["tadx.help.constraints"], text) {
			command.Annotations["tadx.help.constraints"] = strings.TrimSpace(command.Annotations["tadx.help.constraints"] + "\n" + text)
		}
	}
	switch path {
	case "auth login", "auth logout":
		required("environment")
	case "admin group create":
		required("name")
		if command.Annotations == nil {
			command.Annotations = map[string]string{}
		}
		command.Annotations["tadx.help.batch-example"] = `{"items":[{"name":"analysts"},{"name":"publishers"}]}`
	case "admin group inspect":
		group("exactly-one", "id", "name")
	case "admin user inspect":
		group("exactly-one", "id", "name", "username")
	case "admin group delete":
		required("id")
	case "admin group update":
		required("id")
		group("one-required", "new-name", "minimum-site-role", "external-user-enabled", "set-members")
		note("--member-id requires --set-members; --set-members without --member-id removes all direct members.")
	case "admin group-member add", "admin group-member remove":
		required("group-id")
		group("exactly-one", "user-id", "username")
	case "admin user create":
		required("name", "site-role")
		group("exactly-one", "auth-setting", "idp-configuration-id")
	case "admin user delete":
		group("exactly-one", "id", "username")
	case "admin user update":
		group("exactly-one", "id", "username")
		group("exclusive", "auth-setting", "idp-configuration-id")
		group("one-required", "full-name", "email", "site-role", "auth-setting", "identity-pool", "idp-configuration-id", "language", "locale")
	case "admin permission inspect":
		required("kind", "id")
	case "admin permission create", "admin permission delete":
		required("kind", "id", "principal-type", "capability", "mode")
		group("exactly-one", "principal-id", "principal-username")
		note("--principal-username requires --principal-type user; --default-for requires --kind project.")
	case "admin label-value inspect", "admin label-value delete", "admin label-category inspect", "admin label-category delete":
		required("name")
	case "admin label-category create":
		required("name", "description")
	case "admin label-category update":
		required("name")
		group("one-required", "new-name", "description")
	case "admin label-value update":
		required("name")
		group("one-required", "new-name", "category", "description")
		note("Creating a missing label value requires --category and --description; renaming requires an existing value.")
	case "catalog audit":
		required("type", "id")
	case "catalog database inspect", "catalog table inspect":
		group("exactly-one", "id", "metadata-id")
	case "catalog column inspect":
		group("exactly-one", "id", "metadata-id")
		note("--id requires --table-id; --metadata-id selects the column directly.")
	case "catalog column list":
		required("table-id")
	case "catalog database update", "catalog table update":
		required("id")
		group("one-required", "description", "contact-id", "add-tag", "remove-tag")
	case "catalog column update":
		required("id", "table-id")
		group("one-required", "description", "add-tag", "remove-tag")
	case "content datasource schema":
		required("id")
	case "content project create":
		required("name")
		group("exclusive", "parent-id", "parent")
	case "content project inspect":
		group("exactly-one", "id", "project-id", "project")
	case "content project delete":
		group("exactly-one", "id", "project-id")
	case "content project update":
		group("exactly-one", "id", "project-id", "project")
		group("one-required", "new-name", "name", "description", "content-permissions")
	case "content project move":
		group("exactly-one", "id", "project-id", "project")
		group("exactly-one", "parent-id", "parent", "top-level")
	case "catalog lineage pull":
		required("kind")
		group("exactly-one", "id", "name")
	case "catalog label list":
		required("type", "target-id")
	case "catalog label inspect", "catalog label delete":
		required("id")
	case "catalog label update":
		note("Select --id, or both --type and --target-id with --value. Supply at least one of --value, --message, --active, or --elevated.")
	case "pulse definition create":
		required("name", "datasource-id", "measure-field", "date-field", "dimension")
		note("--running-total requires --aggregation SUM and --temporality OVER_TIME; currency applies to --number-format CURRENCY.")
	case "pulse definition inspect", "pulse definition pull", "pulse definition delete", "pulse metric inspect", "pulse metric delete", "pulse metric followers":
		required("id")
	case "pulse definition publish":
		required("datasource-map")
		group("exactly-one", "artifact", "id", "artifact-name")
	case "pulse metric list":
		required("definition-id")
		group("exclusive", "all", "limit")
	case "pulse definition list":
		group("exclusive", "all", "limit")
	case "pulse metric fork":
		required("id")
		group("one-required", "period", "filter", "exclude-filter")
	case "pulse metric follow":
		required("id")
		group("exactly-one", "user-id", "group-id")
	case "pulse metric unfollow":
		note("Select --subscription-id alone, or --id with exactly one of --user-id or --group-id.")
	case "workspace artifact move":
		required("source", "destination")
		helpFlagAnnotation(command, "source", "tadx.help.value", []string{"workspace-name"})
		helpFlagAnnotation(command, "destination", "tadx.help.value", []string{"workspace-name"})
		note("Select --artifact, or both --kind and --id; do not combine these selector forms.")
	case "workspace artifact delete":
		required("workspace")
		note("Select --artifact, or both --kind and --id; do not combine these selector forms.")
	case "workspace clean":
		required("workspace", "class")
	case "workspace register":
		required("path")
	case "workspace clone":
		required("name")
	case "env add":
		required("url")
	case "env update":
		for _, name := range []string{"site", "api-version", "pat-name-env", "pat-secret-env", "default-workspace", "cache-max-concurrency"} {
			group("exclusive", name, "clear-"+name)
		}
	case "agent uninstall":
		required("target")
	}
	for _, resource := range []string{"workbook", "datasource", "flow"} {
		prefix := "content " + resource + " "
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		action := strings.TrimPrefix(path, prefix)
		switch action {
		case "inspect", "pull", "update", "move", "delete":
			group("exactly-one", "id", "name")
			if action == "inspect" && command.Flags().Lookup("project-id") != nil {
				note("--name requires exactly one of --project or --project-id; --id excludes name/project selectors.")
			} else if resource == "workbook" && action == "pull" {
				note("--project scopes an exact --name selector when supplied.")
			} else {
				note("--name requires --project; --id excludes both.")
			}
		case "publish":
			group("exactly-one", "artifact", "file", "id", "artifact-name")
			group("exactly-one", "project-id", "project")
			if resource == "datasource" {
				group("exactly-one", "create", "overwrite", "append", "replace")
			}
		}
		if action == "move" {
			group("exactly-one", "destination-project-id", "destination-project")
		}
		if action == "update" {
			switch resource {
			case "workbook":
				group("one-required", "new-name", "owner-id", "description")
			case "datasource":
				group("one-required", "new-name", "owner-id")
			case "flow":
				required("owner-id")
			}
		}
	}
}

func helpFlagAnnotation(command *cobra.Command, name, key string, values []string) {
	flag := command.Flags().Lookup(name)
	if flag == nil {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[key] = append([]string(nil), values...)
}

func appendHelpUsage(flag *pflag.Flag, text string) {
	if flag != nil && !strings.Contains(flag.Usage, text) {
		flag.Usage += "; " + text
	}
}

// Action-level zero-value fallbacks, independent of Cobra's declared zero.
var helpLimitDefaults = map[string]string{
	"content workbook list": "25", "content datasource list": "25", "content flow list": "25", "content project list": "25",
	"content datasource schema": "20", "catalog label list": "20",
	"admin user list": "25", "admin group list": "25", "admin label-value list": "20", "admin label-category list": "20",
	"catalog database list": "25", "catalog table list": "25", "catalog column list": "25", "catalog search": "25", "catalog audit": "1000",
	"pulse definition list": "25", "pulse metric list": "25",
	"env list": "20", "workspace list": "20", "workspace status": "20", "capability list": "20",
}

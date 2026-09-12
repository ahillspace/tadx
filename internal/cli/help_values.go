package cli

import (
	"strings"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// applyHelpValues adds presentation metadata only. Scope by canonical command
// path: identically named flags can have different contracts in other actions.
func applyHelpValues(root *cobra.Command) {
	var visit func(*cobra.Command, string)
	visit = func(command *cobra.Command, path string) {
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
		case "content lineage pull":
			choices("kind", "workbook", "datasource", "published_datasource", "flow")
			choices("direction", "upstream", "downstream", "both")
			value("depth", "1..3")
		case "workspace clean":
			choices("class", "temporary", "cache", "logs", "all")
		case "content label list", "content label inspect", "content label update", "content label delete":
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
			value("days", "1..3650")
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
				helpFlagAnnotation(command, "cache-max-concurrency", "tadx.help.default", []string{"unchanged"})
			}
			value("url", "https://server")
			value("api-version", "major.minor")
		}
		if limit, ok := helpLimitDefaults[path]; ok {
			value("limit", "1..10000")
			helpFlagAnnotation(command, "limit", "tadx.help.default", []string{limit})
		}
		for _, child := range command.Commands() {
			visit(child, strings.TrimSpace(path+" "+child.Name()))
		}
	}
	visit(root, "")
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
	"content datasource schema": "20", "content label list": "20",
	"admin user list": "25", "admin group list": "25", "admin label value list": "20", "admin label category list": "20",
	"catalog database list": "25", "catalog table list": "25", "catalog column list": "25", "catalog search": "25", "catalog audit": "1000",
	"pulse definition list": "25", "pulse metric list": "25",
	"env list": "20", "workspace list": "20", "workspace status": "20", "capability list": "20",
}

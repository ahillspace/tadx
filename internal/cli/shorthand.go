package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var commandAliases = map[string]string{
	"catalog": "cat", "database": "db", "table": "tbl", "column": "col", "audit": "aud", "label": "lbl", "value": "val", "category": "ctg",
	"admin": "adm", "agent": "agt", "artifact": "art", "auth": "ath",
	"capability": "cap", "cache": "cch", "check": "chk", "clean": "cln",
	"clone": "cl", "completion": "cmp", "content": "con", "create": "new",
	"datasource": "ds", "default": "dft", "definition": "def", "delete": "del",
	"doctor": "doc", "flow": "flw", "follow": "fol", "followers": "fls",
	"fork": "frk", "group": "grp", "inspect": "ins", "install": "ist",
	"last": "lst", "lineage": "lin", "list": "ls", "login": "log",
	"logout": "out", "member": "mem", "metric": "met", "move": "mv",
	"mutation": "mut", "permission": "prm", "project": "prj", "publish": "pub",
	"pull": "pl", "pulse": "pls", "refresh": "ref", "register": "reg",
	"remove": "rm", "schema": "sch", "search": "sea", "set-default": "sdf",
	"status": "st", "uninstall": "uni", "unfollow": "unf", "unregister": "unr",
	"update": "upd", "user": "usr", "version": "ver", "workbook": "wb",
	"workspace": "ws",
}

var flagShorthands = map[string]string{
	"all": "a", "environment": "e", "full": "f", "id": "i", "limit": "l",
	"name": "n", "preview": "p", "query": "q", "workspace": "w",
}

var flagLongAliases = map[string]string{
	"value":        "val",
	"descriptions": "dcs", "tags": "tgs", "metadata-id": "mdi", "table-id": "tbi", "database-id": "dbi", "contact-id": "cti", "add-tag": "atg", "remove-tag": "rtg", "direct-only": "dro", "target-id": "tid", "category": "ctg", "message": "msg", "active": "act", "elevated": "elv",
	"batch-file": "btf", "json": "jsn", "username": "unm", "principal-username": "pun",
	"aggregation": "agg", "api-version": "api", "append": "apd", "artifact": "art",
	"artifact-name": "arn", "as-job": "job", "auth-setting": "aus", "capability": "cap",
	"cache": "cch", "cache-max-concurrency": "cmc", "check": "chk", "class": "cls",
	"clear-api-version": "cav", "clear-cache-max-concurrency": "ccm", "clear-default-workspace": "cdw",
	"clear-pat-name-env": "cpn", "clear-pat-secret-env": "cps", "clear-site": "cst",
	"config": "cfg", "content-permissions": "cpm", "create": "new", "currency": "cur",
	"cursor": "csr", "datasource-id": "did", "datasource-map": "dsm", "date-field": "dtf",
	"days": "day", "default-for": "dft", "default-workspace": "dws", "definition-id": "dfi",
	"depth": "dep", "description": "dsc", "destination": "dst", "destination-project": "dpj",
	"destination-project-id": "dpi", "dimension": "dim", "direction": "dir", "domain": "dom",
	"email": "eml", "enabled": "ena", "environment": "env", "exclude-filter": "exf",
	"external-user-enabled": "eue", "field-id": "fid", "file": "fil", "filter": "flt",
	"force": "frc", "full": "ful", "full-name": "fnm", "group-id": "gid",
	"identity-pool": "idp", "idp-configuration-id": "ici", "include-extract": "iex",
	"include-pds": "ipd", "kind": "knd", "language": "lng", "limit": "lim",
	"locale": "loc", "measure-field": "msf", "member-id": "mid", "members": "mem",
	"minimum-granularity": "mng", "minimum-site-role": "msr", "mode": "mod", "mutation": "mut",
	"name": "nm", "new-name": "nnm", "number-format": "nfm", "overwrite": "ovr",
	"owner": "own", "owner-id": "oid", "parent": "par", "parent-id": "pai",
	"path": "pth", "pat-name-env": "pne", "pat-secret-env": "pse", "period": "per",
	"preview": "pv", "principal-id": "pri", "principal-type": "prt", "product": "prd",
	"project": "prj", "project-id": "pid", "project-name": "pnm", "query": "qry",
	"replace": "rpl", "resource": "res", "role": "rol", "running-total": "rnt",
	"scope": "scp", "sentiment": "snt", "set-members": "stm", "site": "sit",
	"site-role": "srl", "source": "src", "subscription-id": "sid", "table": "tbl",
	"target": "tgt", "temporality": "tmp", "top-level": "top", "type": "typ",
	"updated-after": "uaf", "updated-before": "ubf", "user-id": "uid", "workspace": "ws",
}

// These Cobra-owned commands are intentionally outside the explicit alias table.
var commandAliasExceptions = map[string]string{
	"help":       "Cobra already exposes the standard -h flag for help",
	"__complete": "hidden Cobra completion protocol command",
}

func normalizeFlagName(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	if canonical, ok := canonicalFlagByAlias[name]; ok {
		name = canonical
	}
	return pflag.NormalizedName(name)
}

var canonicalFlagByAlias = reverseAliases(flagLongAliases)

func reverseAliases(aliases map[string]string) map[string]string {
	reversed := make(map[string]string, len(aliases))
	for canonical, alias := range aliases {
		reversed[alias] = canonical
	}
	return reversed
}

func applyShorthand(root *cobra.Command) {
	if root == nil {
		return
	}
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		if alias := commandAliases[command.Name()]; alias != "" {
			command.Aliases = appendUnique(command.Aliases, alias)
			marker := "alias: " + alias
			if !strings.Contains(command.Short, marker) {
				command.Short += " (" + marker + ")"
			}
		}
		rebuildFlagSetsWithShorthand(command)
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	if err := validateShorthandTree(root); err != nil {
		panic(err)
	}
	root.SetGlobalNormalizationFunc(normalizeFlagName)
}

func rebuildFlagSetsWithShorthand(command *cobra.Command) {
	local := collectFlags(command.LocalNonPersistentFlags())
	persistent := collectFlags(command.PersistentFlags())
	for _, flag := range append(append([]*pflag.Flag(nil), local...), persistent...) {
		if shorthand := flagShorthands[flag.Name]; shorthand != "" && flag.Shorthand == "" {
			flag.Shorthand = shorthand
		}
		if alias := flagLongAliases[flag.Name]; alias != "" {
			marker := "alias: --" + alias
			if !strings.Contains(flag.Usage, marker) {
				flag.Usage += " (" + marker + ")"
			}
		}
	}
	command.ResetFlags()
	for _, flag := range persistent {
		command.PersistentFlags().AddFlag(flag)
	}
	for _, flag := range local {
		command.Flags().AddFlag(flag)
	}
}

func collectFlags(set *pflag.FlagSet) []*pflag.Flag {
	var flags []*pflag.Flag
	set.VisitAll(func(flag *pflag.Flag) { flags = append(flags, flag) })
	return flags
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func validateShorthandTree(root *cobra.Command) error {
	var problems []string
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		seenCommands := map[string]string{}
		for _, child := range command.Commands() {
			if len(child.Name()) > 3 && commandAliases[child.Name()] == "" && commandAliasExceptions[child.Name()] == "" {
				problems = append(problems, fmt.Sprintf("%s: command %q has no alias", command.CommandPath(), child.Name()))
			}
			for _, spelling := range append([]string{child.Name()}, child.Aliases...) {
				if prior := seenCommands[spelling]; prior != "" && prior != child.Name() {
					problems = append(problems, fmt.Sprintf("%s: command spelling %q selects both %q and %q", command.CommandPath(), spelling, prior, child.Name()))
				}
				seenCommands[spelling] = child.Name()
			}
		}
		allFlags := collectEffectiveFlags(command)
		shorts := map[string]string{}
		for _, flag := range allFlags {
			if len(flag.Name) > 3 && flagLongAliases[flag.Name] == "" && flagShorthands[flag.Name] == "" {
				problems = append(problems, fmt.Sprintf("%s: flag --%s has no shorthand", command.CommandPath(), flag.Name))
			}
			if canonical := canonicalFlagByAlias[flag.Name]; canonical != "" && canonical != flag.Name {
				problems = append(problems, fmt.Sprintf("%s: flag alias --%s for --%s collides with canonical --%s", command.CommandPath(), flag.Name, canonical, flag.Name))
			}
			if flag.Shorthand != "" {
				if prior := shorts[flag.Shorthand]; prior != "" && prior != flag.Name {
					problems = append(problems, fmt.Sprintf("%s: flag shorthand -%s selects both --%s and --%s", command.CommandPath(), flag.Shorthand, prior, flag.Name))
				}
				shorts[flag.Shorthand] = flag.Name
			}
		}
		for _, child := range command.Commands() {
			walk(child)
		}
	}
	walk(root)
	if len(problems) > 0 {
		return fmt.Errorf("invalid shorthand tree: %s", strings.Join(problems, "; "))
	}
	return nil
}

func collectEffectiveFlags(command *cobra.Command) []*pflag.Flag {
	flags := collectFlags(command.LocalFlags())
	seen := make(map[*pflag.Flag]struct{}, len(flags))
	for _, flag := range flags {
		seen[flag] = struct{}{}
	}
	for _, flag := range collectFlags(command.InheritedFlags()) {
		if _, ok := seen[flag]; ok {
			continue
		}
		flags = append(flags, flag)
		seen[flag] = struct{}{}
	}
	return flags
}

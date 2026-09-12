package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// installCategoryHelp derives help from the completed command tree. It deliberately
// avoids action dependencies, flag values, and configuration resolution.
func installCategoryHelp(root *cobra.Command) {
	root.SetHelpCommand(&cobra.Command{
		Use: "help [command]", Short: "Show help for any category or command",
		Annotations:       map[string]string{groupingAnnotation: "true"},
		PersistentPreRun:  func(*cobra.Command, []string) {},
		PersistentPostRun: func(*cobra.Command, []string) {},
		RunE: func(command *cobra.Command, args []string) error {
			target, remaining, err := root.Find(args)
			if err != nil {
				return err
			}
			if len(remaining) != 0 {
				return fmt.Errorf("unknown help topic %q", strings.Join(args, " "))
			}
			return target.Help()
		},
	})
	root.SetHelpFunc(func(command *cobra.Command, _ []string) {
		out := command.OutOrStdout()
		switch {
		case command == root:
			writeRootHelp(out, command)
		case hasVisibleHelpChildren(command):
			writeCategoryHelp(out, command)
		default:
			writeLeafHelp(out, command)
		}
	})
}

func visibleHelpChildren(command *cobra.Command) []*cobra.Command {
	var children []*cobra.Command
	for _, child := range command.Commands() {
		if !child.Hidden && child.Name() != "help" {
			children = append(children, child)
		}
	}
	return children
}

func hasVisibleHelpChildren(command *cobra.Command) bool {
	return len(visibleHelpChildren(command)) != 0
}

func helpAction(command *cobra.Command) bool {
	return command.Runnable() && command.Annotations[groupingAnnotation] != "true"
}

func helpNodes(category *cobra.Command) (nodes, actions []*cobra.Command) {
	var walk func(*cobra.Command)
	walk = func(command *cobra.Command) {
		nodes = append(nodes, command)
		if helpAction(command) {
			actions = append(actions, command)
		}
		for _, child := range visibleHelpChildren(command) {
			walk(child)
		}
	}
	walk(category)
	return nodes, actions
}

func writeRootHelp(out io.Writer, root *cobra.Command) {
	fmt.Fprintln(out, root.Short)
	fmt.Fprintf(out, "\nusage: %s <category> <command> [flags]\n", root.CommandPath())
	fmt.Fprintf(out, "       %s [flags]\n", root.CommandPath())
	fmt.Fprintf(out, "\nRun %s without arguments for a local, read-only session overview.\n", root.CommandPath())
	fmt.Fprintln(out, "Category help includes every descendant command, its flags, and examples.")
	fmt.Fprintln(out, "\ncategories and commands:")
	children := visibleHelpChildren(root)
	for _, child := range children {
		fmt.Fprintf(out, "  %-16s %s\n", child.Name(), helpShort(child))
	}
	writeFlagSection(out, "global flags", helpFlags(root))
	fmt.Fprintln(out, "\nhelp:")
	count := 0
	for _, child := range children {
		if !hasVisibleHelpChildren(child) {
			continue
		}
		fmt.Fprintf(out, "  %s --help\n", child.CommandPath())
		count++
		if count == 3 {
			break
		}
	}
	fmt.Fprintln(out, "  -h, --help  Show help for any category or command.")
	if root.Example != "" {
		fmt.Fprintln(out, "\nexamples:")
		writeIndented(out, root.Example, "  ")
	}
}

func helpShort(command *cobra.Command) string {
	short := command.Short
	var aliases []string
	for _, alias := range command.Aliases {
		if !strings.Contains(short, "alias: "+alias) {
			aliases = append(aliases, alias)
		}
	}
	if len(aliases) > 0 {
		short += " (aliases: " + strings.Join(aliases, ", ") + ")"
	}
	return short
}

type helpFlagGroup struct {
	flag    *pflag.Flag
	actions []string
}

func writeCategoryHelp(out io.Writer, category *cobra.Command) {
	fmt.Fprintln(out, helpPlainShort(category))
	fmt.Fprintf(out, "\nusage: %s <command> [flags]\n", category.CommandPath())
	writeAliases(out, category)
	nodes, actions := helpNodes(category)
	fmt.Fprintf(out, "\ncommands[%d]:\n", len(actions))
	for _, action := range actions {
		usage := relativeHelpUsage(category, action)
		fmt.Fprintf(out, "  %s\n    %s\n", usage, helpPlainShort(action))
		if aliases := helpPathAliases(category, action); aliases != "" {
			fmt.Fprintf(out, "    aliases: %s\n", aliases)
		}
	}

	groups := map[string]*helpFlagGroup{}
	for _, action := range actions {
		for _, flag := range helpFlags(action) {
			key := helpFlagKey(flag)
			if groups[key] == nil {
				groups[key] = &helpFlagGroup{flag: flag}
			}
			groups[key].actions = append(groups[key].actions, relativeHelpPath(category, action))
		}
	}
	for _, action := range actions {
		path := relativeHelpPath(category, action)
		var specific []*pflag.Flag
		for _, flag := range helpFlags(action) {
			if len(groups[helpFlagKey(flag)].actions) == 1 {
				specific = append(specific, flag)
			}
		}
		writeFlagSection(out, "flags{"+path+"}", specific)
		writeRelationships(out, "constraints{"+path+"}", action)
	}
	// Group shared flags by the exact set of actions accepting the same metadata.
	scopes := map[string][]*pflag.Flag{}
	for _, group := range groups {
		if len(group.actions) < 2 {
			continue
		}
		scope := strings.Join(group.actions, ", ")
		if len(group.actions) == len(actions) {
			scope = "all actions"
		}
		scopes[scope] = append(scopes[scope], group.flag)
	}
	for _, scope := range sortedHelpKeys(scopes) {
		writeFlagSection(out, "shared flags{"+scope+"}", scopes[scope])
	}
	fmt.Fprintln(out, "\n  -h, --help  Show help for this category or one command.")
	writeCategoryNotes(out, category, nodes, actions)
	writeHelpExamples(out, nodes)
}

func helpPlainShort(command *cobra.Command) string {
	return helpWithoutAliasSuffix(command.Short)
}

func helpWithoutAliasSuffix(summary string) string {
	for _, marker := range []string{" (alias:", " (aliases:"} {
		if index := strings.LastIndex(summary, marker); index >= 0 && strings.HasSuffix(summary, ")") {
			summary = summary[:index]
		}
	}
	return summary
}

func writeCategoryNotes(out io.Writer, category *cobra.Command, nodes, actions []*cobra.Command) {
	type note struct {
		text  string
		paths []string
	}
	var notes []*note
	byText := map[string]*note{}
	for _, node := range nodes {
		long := strings.TrimSpace(strings.ReplaceAll(node.Long, "\r\n", "\n"))
		firstLine, rest, multiline := strings.Cut(long, "\n")
		long = helpWithoutAliasSuffix(firstLine)
		if multiline {
			long += "\n" + rest
		}
		short := strings.TrimSuffix(helpPlainShort(node), ".")
		if short != "" && strings.HasPrefix(long, short) {
			rest := strings.TrimPrefix(long, short)
			if rest == "" || strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "\n") {
				long = strings.TrimSpace(strings.TrimPrefix(rest, "."))
			}
		}
		for _, paragraph := range strings.Split(long, "\n\n") {
			paragraph = strings.TrimSpace(paragraph)
			if paragraph == "" {
				continue
			}
			entry := byText[paragraph]
			if entry == nil {
				entry = &note{text: paragraph}
				byText[paragraph] = entry
				notes = append(notes, entry)
			}
			entry.paths = appendUnique(entry.paths, relativeHelpPath(category, node))
		}
	}
	scoped := map[string][]string{}
	for _, entry := range notes {
		scope := strings.Join(entry.paths, ", ")
		if len(entry.paths) == len(actions) && helpPathsAreActions(category, entry.paths, actions) {
			scope = "all actions"
		}
		scoped[scope] = append(scoped[scope], entry.text)
	}
	for _, scope := range sortedHelpKeys(scoped) {
		fmt.Fprintf(out, "\nnotes{%s}:\n", scope)
		writeIndented(out, strings.Join(scoped[scope], "\n\n"), "  ")
	}
}

func helpPathsAreActions(category *cobra.Command, paths []string, actions []*cobra.Command) bool {
	for index, action := range actions {
		if paths[index] != relativeHelpPath(category, action) {
			return false
		}
	}
	return true
}

func relativeHelpPath(category, command *cobra.Command) string {
	if category == command {
		return "(category)"
	}
	return strings.TrimPrefix(command.CommandPath(), category.CommandPath()+" ")
}

func relativeHelpUsage(category, command *cobra.Command) string {
	if category == command {
		return command.Use
	}
	prefix := strings.TrimSuffix(relativeHelpPath(category, command), command.Name())
	return prefix + command.Use
}

func helpPathAliases(category, command *cobra.Command) string {
	var aliases []string
	for node := command; node != category && node != nil; node = node.Parent() {
		if len(node.Aliases) > 0 {
			aliases = append([]string{node.Name() + "=" + strings.Join(node.Aliases, "/")}, aliases...)
		}
	}
	return strings.Join(aliases, ", ")
}

func writeLeafHelp(out io.Writer, command *cobra.Command) {
	description := command.Long
	if description == "" {
		description = helpPlainShort(command)
	}
	if description != "" {
		fmt.Fprintln(out, description)
	}
	fmt.Fprintf(out, "\nUsage:\n  %s", command.CommandPath())
	if suffix := strings.TrimPrefix(command.Use, command.Name()); suffix != "" {
		fmt.Fprint(out, suffix)
	}
	if !command.DisableFlagsInUseLine {
		fmt.Fprint(out, " [flags]")
	}
	fmt.Fprintln(out)
	writeAliases(out, command)
	writeFlagSection(out, "Flags", visibleHelpFlags(command.LocalFlags()))
	writeFlagSection(out, "Global Flags", visibleHelpFlags(command.InheritedFlags()))
	fmt.Fprintln(out, "  -h, --help  Show help for this command.")
	writeRelationships(out, "Constraints", command)
	writeHelpExamples(out, []*cobra.Command{command})
}

func writeAliases(out io.Writer, command *cobra.Command) {
	if len(command.Aliases) > 0 {
		fmt.Fprintf(out, "\nAliases:\n  %s\n", strings.Join(append([]string{command.Name()}, command.Aliases...), ", "))
	}
}

func helpFlags(command *cobra.Command) []*pflag.Flag {
	flags := collectEffectiveFlags(command)
	visible := flags[:0]
	for _, flag := range flags {
		if !flag.Hidden && flag.Name != "help" {
			visible = append(visible, flag)
		}
	}
	return visible
}

func visibleHelpFlags(set *pflag.FlagSet) []*pflag.Flag {
	var flags []*pflag.Flag
	set.VisitAll(func(flag *pflag.Flag) {
		if !flag.Hidden && flag.Name != "help" {
			flags = append(flags, flag)
		}
	})
	return flags
}

func writeFlagSection(out io.Writer, title string, flags []*pflag.Flag) {
	if len(flags) == 0 {
		return
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Name < flags[j].Name })
	fmt.Fprintf(out, "\n%s:\n", title)
	for _, flag := range flags {
		fmt.Fprintf(out, "  %s\n", helpFlagText(flag))
	}
}

func helpFlagText(flag *pflag.Flag) string {
	label := "--" + flag.Name
	if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
		label = "-" + flag.Shorthand + ", " + label
	}
	typeName := flag.Value.Type()
	value := typeName
	if choices := flag.Annotations["tadx.help.choices"]; len(choices) > 0 {
		value = strings.Join(choices, "|")
	} else if values := flag.Annotations["tadx.help.value"]; len(values) > 0 {
		value = values[0]
	} else if strings.HasPrefix(typeName, "int") || strings.HasPrefix(typeName, "uint") || strings.HasPrefix(typeName, "float") || typeName == "count" {
		value = "number"
	} else {
		value = strings.TrimSuffix(strings.TrimSuffix(typeName, "Array"), "Slice")
	}
	if typeName == "bool" {
		label += "[=true|false]"
	} else {
		label += " <" + value + ">"
	}
	usage := flag.Usage
	if alias := flagLongAliases[flag.Name]; alias != "" && !strings.Contains(usage, "alias: --"+alias) {
		usage += " (alias: --" + alias + ")"
	}
	var details []string
	_, slice := flag.Value.(pflag.SliceValue)
	_, selector := flag.Value.(*repeatedSelector)
	if slice || selector || typeName == "count" {
		details = append(details, "repeatable")
	}
	if strings.HasSuffix(typeName, "Slice") {
		details = append(details, "comma-separated values accepted")
	}
	if helpAnnotationTrue(flag, cobra.BashCompOneRequiredFlag) || helpAnnotationTrue(flag, "tadx.help.required") {
		details = append(details, "required")
	}
	if declared := helpDeclaredDefault(flag); declared != "" {
		details = append(details, "default: "+declared)
	}
	if flag.Deprecated != "" {
		details = append(details, "deprecated: "+flag.Deprecated)
	}
	if len(details) > 0 {
		label += " (" + strings.Join(details, "; ") + ")"
	}
	return label + "  " + usage
}

func helpAnnotationTrue(flag *pflag.Flag, key string) bool {
	values := flag.Annotations[key]
	return len(values) > 0 && values[0] == "true"
}

func helpDeclaredDefault(flag *pflag.Flag) string {
	// Config can be initialized from caller state. Its default is intentionally
	// never rendered, including if a caller forgets to clear DefValue.
	declared := flag.DefValue
	if defaults := flag.Annotations["tadx.help.default"]; len(defaults) > 0 {
		declared = defaults[0]
	}
	if flag.Name == "config" || declared == "" || declared == "[]" {
		return ""
	}
	if flag.Value.Type() == "string" {
		return strconv.Quote(declared)
	}
	return declared
}

func helpFlagKey(flag *pflag.Flag) string {
	key := helpFlagText(flag)
	for _, annotation := range sortedHelpKeys(flag.Annotations) {
		key += "\x00" + annotation + "=" + strings.Join(flag.Annotations[annotation], "\x00")
	}
	return key
}

func writeRelationships(out io.Writer, title string, command *cobra.Command) {
	var lines []string
	for _, relationship := range []struct{ annotation, label string }{
		{"cobra_annotation_one_required", "at least one of"},
		{"cobra_annotation_mutually_exclusive", "mutually exclusive"},
		{"cobra_annotation_required_if_others_set", "required together"},
	} {
		seen := map[string]bool{}
		for _, flag := range helpFlags(command) {
			for _, group := range flag.Annotations[relationship.annotation] {
				if seen[group] {
					continue
				}
				seen[group] = true
				var names []string
				for _, name := range strings.Fields(group) {
					candidate := command.Flags().Lookup(name)
					if candidate == nil || candidate.Hidden {
						continue
					}
					names = append(names, "--"+name)
				}
				if len(names) > 1 {
					lines = append(lines, relationship.label+": "+strings.Join(names, ", "))
				}
			}
		}
	}
	if len(lines) > 0 {
		fmt.Fprintf(out, "\n%s:\n", title)
		for _, line := range lines {
			fmt.Fprintf(out, "  %s\n", line)
		}
	}
}

func writeHelpExamples(out io.Writer, nodes []*cobra.Command) {
	seen := map[string]bool{}
	heading := false
	for _, node := range nodes {
		example := strings.TrimSpace(node.Example)
		if example == "" || seen[example] {
			continue
		}
		seen[example] = true
		if !heading {
			fmt.Fprintln(out, "\nexamples:")
			heading = true
		}
		writeIndented(out, example, "  ")
	}
}

func writeIndented(out io.Writer, value, prefix string) {
	for _, line := range strings.Split(strings.TrimSpace(value), "\n") {
		fmt.Fprintln(out, prefix+strings.TrimSuffix(line, "\r"))
	}
}

func sortedHelpKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

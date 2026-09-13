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
		if writeContentPilotHelp(out, command) {
			return
		}
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
	fmt.Fprintln(out, "TADX: discover, inspect, download, and manage Tableau assets.")
	fmt.Fprintf(out, "\nusage:\n  %s                            Local session overview\n  %s <command> --help           Commands, inputs, and examples\n", root.Name(), root.Name())
	groups := []struct {
		title string
		names []string
	}{
		{"discover and manage Tableau", []string{"search", "content", "catalog", "admin", "pulse"}},
		{"local state", []string{"cache", "workspace", "last"}},
		{"setup and diagnostics", []string{"env", "auth", "mutation", "agent", "doctor", "capability", "update", "version", "completion"}},
	}
	remaining := map[string]*cobra.Command{}
	for _, child := range visibleHelpChildren(root) {
		remaining[child.Name()] = child
	}
	for _, group := range groups {
		var commands []*cobra.Command
		for _, name := range group.names {
			if command := remaining[name]; command != nil {
				commands = append(commands, command)
				delete(remaining, name)
			}
		}
		writeRoadmapGroup(out, group.title, commands)
	}
	var others []*cobra.Command
	for _, name := range sortedHelpKeys(remaining) {
		others = append(others, remaining[name])
	}
	writeRoadmapGroup(out, "other commands", others)
	writeFlagSection(out, "global flags", helpFlags(root))
	writeHelpSyntax(out)
	fmt.Fprintln(out, "\nUse resource help for action syntax. Inspect reads details; pull writes local files.")
	if root.Example != "" {
		writeExamples(out, root.Example)
	} else {
		var examples []string
		for _, path := range []string{"content workbook", "admin group", "pulse"} {
			if command := helpCommandAt(root, strings.Fields(path)); command != nil {
				examples = append(examples, command.CommandPath()+" --help")
			}
		}
		if len(examples) == 0 {
			for _, child := range visibleHelpChildren(root) {
				examples = append(examples, child.CommandPath()+" --help")
				if len(examples) == 3 {
					break
				}
			}
		}
		writeExamples(out, strings.Join(examples, "\n"))
	}
}

func writeRoadmapGroup(out io.Writer, title string, commands []*cobra.Command) {
	if len(commands) == 0 {
		return
	}
	fmt.Fprintf(out, "\n%s:\n", title)
	for _, command := range commands {
		description := command.Annotations["tadx.help.summary"]
		if description == "" {
			description = helpPlainShort(command)
		}
		fmt.Fprintf(out, "  %-20s %s\n", helpCommandName(command), description)
	}
}

func helpCommandName(command *cobra.Command) string {
	name := command.Name()
	if len(command.Aliases) > 0 {
		name += " (" + strings.Join(command.Aliases, ", ") + ")"
	}
	return name
}

func helpDisplayPath(category, command *cobra.Command) string {
	var names []string
	for node := command; node != nil && node != category; node = node.Parent() {
		names = append([]string{helpCommandName(node)}, names...)
	}
	if len(names) == 0 {
		return helpCommandName(command)
	}
	return strings.Join(names, " ")
}

func writeCategoryHelp(out io.Writer, category *cobra.Command) {
	fmt.Fprintf(out, "%s: %s\n", helpCommandName(category), helpPlainShort(category))
	fmt.Fprintf(out, "\nusage: %s <command> [flags]\n", category.CommandPath())
	nodes, actions := helpNodes(category)
	common := commonHelpFlags(actions)
	fmt.Fprintf(out, "\ncommands[%d]:\n", len(actions))
	for _, action := range actions {
		writeActionHelp(out, category, action, common)
	}
	var flags []*pflag.Flag
	for _, flag := range common {
		flags = append(flags, flag)
	}
	writeFlagSection(out, "common flags (all commands)", flags)
	writeHelpSyntax(out)
	for _, node := range nodes {
		if helpAction(node) {
			continue
		}
		if note := helpLongNotes(node); note != "" {
			fmt.Fprintf(out, "\nnotes (%s):\n", helpDisplayPath(category, node))
			writeIndented(out, note, "  ")
		}
	}
	writeCategoryBatchHelp(out, actions)
}

func commonHelpFlags(actions []*cobra.Command) map[string]*pflag.Flag {
	common := map[string]*pflag.Flag{}
	if len(actions) < 2 {
		return common
	}
	for _, flag := range helpFlags(actions[0]) {
		if !helpRequired(flag) {
			common[flag.Name] = flag
		}
	}
	for _, action := range actions[1:] {
		flags := map[string]*pflag.Flag{}
		for _, flag := range helpFlags(action) {
			flags[flag.Name] = flag
		}
		for name, flag := range common {
			candidate := flags[name]
			if candidate == nil || helpFlagKey(candidate) != helpFlagKey(flag) {
				delete(common, name)
			}
		}
	}
	return common
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

func helpLongNotes(command *cobra.Command) string {
	long := strings.TrimSpace(strings.ReplaceAll(command.Long, "\r\n", "\n"))
	first, rest, multiline := strings.Cut(long, "\n")
	long = helpWithoutAliasSuffix(first)
	if multiline {
		long += "\n" + rest
	}
	short := strings.TrimSuffix(helpPlainShort(command), ".")
	if short != "" && strings.HasPrefix(long, short) {
		rest := strings.TrimPrefix(long, short)
		if rest == "" || strings.HasPrefix(rest, ".") || strings.HasPrefix(rest, "\n") {
			long = strings.TrimSpace(strings.TrimPrefix(rest, "."))
		}
	}
	var paragraphs []string
	for _, paragraph := range strings.Split(long, "\n\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph != "" && !strings.HasPrefix(paragraph, "Batch JSON:") {
			paragraphs = append(paragraphs, paragraph)
		}
	}
	return strings.Join(paragraphs, "\n\n")
}

func writeLeafHelp(out io.Writer, command *cobra.Command) {
	writeActionHelp(out, command.Parent(), command, nil)
	writeHelpSyntax(out)
	for parent := command.Parent(); parent != nil && parent.Parent() != nil; parent = parent.Parent() {
		if note := helpLongNotes(parent); note != "" {
			fmt.Fprintf(out, "\nnotes (%s):\n", helpCommandName(parent))
			writeIndented(out, note, "  ")
		}
	}
	writeCategoryBatchHelp(out, []*cobra.Command{command})
}

func writeActionHelp(out io.Writer, category, command *cobra.Command, common map[string]*pflag.Flag) {
	fmt.Fprintf(out, "\n%s:\n  %s\n", helpDisplayPath(category, command), helpPlainShort(command))
	fmt.Fprintf(out, "  usage: %s", command.CommandPath())
	if suffix := strings.TrimPrefix(command.Use, command.Name()); suffix != "" {
		fmt.Fprint(out, suffix)
	}
	var required, optional []*pflag.Flag
	for _, flag := range helpFlags(command) {
		if helpRequired(flag) {
			required = append(required, flag)
		} else if common[flag.Name] == nil {
			optional = append(optional, flag)
		}
	}
	sort.Slice(required, func(i, j int) bool { return required[i].Name < required[j].Name })
	for _, flag := range required {
		fmt.Fprintf(out, " %s", helpFlagSyntax(flag, false))
	}
	if !command.DisableFlagsInUseLine {
		fmt.Fprint(out, " [flags]")
	}
	fmt.Fprintln(out)
	if len(required) > 0 && command.Flags().Lookup("batch-file") != nil {
		fmt.Fprintln(out, "  Required inputs apply to each item when using --batch-file.")
	}
	var section strings.Builder
	writeFlagSection(&section, "required", required)
	writeFlagSection(&section, "options", optional)
	writeRelationships(&section, "constraints", command)
	if command.Annotations["tadx.batch.file"] == "true" {
		fmt.Fprintln(&section, "\nbatch:")
		if selectors := command.Annotations["tadx.batch.selectors"]; selectors != "" {
			names := strings.Split(selectors, ",")
			for index := range names {
				names[index] = "--" + names[index]
			}
			fmt.Fprintf(&section, "  Repeat one selector dimension: %s. Use explicit file rows for different selector pairs.\n", strings.Join(names, ", "))
		}
		if command.Annotations["tadx.batch.positional"] == "true" {
			fmt.Fprintln(&section, `  Positional targets are repeatable; file rows provide them as "args":["<value>"].`)
		}
		if example := command.Annotations["tadx.help.batch-example"]; example != "" {
			fmt.Fprintln(&section, "  JSON: "+example)
		}
	}
	if note := helpLongNotes(command); note != "" {
		fmt.Fprintln(&section, "\nnotes:")
		writeIndented(&section, note, "  ")
	}
	writeExamples(&section, command.Example)
	if section.Len() > 0 {
		writeIndented(out, section.String(), "  ")
	}
}

func writeHelpSyntax(out io.Writer) {
	fmt.Fprintln(out, "\n  --help (-h)  Show help only; never run the operation.")
	fmt.Fprintln(out, "  Boolean flags accept =true or =false; a bare flag means true. Omitted options keep their documented default.")
}

func writeCategoryBatchHelp(out io.Writer, actions []*cobra.Command) {
	found := false
	for _, action := range actions {
		if action.Flags().Lookup("batch-file") != nil {
			found = true
			break
		}
	}
	if !found {
		return
	}
	fmt.Fprintln(out, "\nbatch syntax:")
	fmt.Fprintln(out, "  Applies to commands with --batch-file. Each item supplies that action's required inputs.")
	fmt.Fprintln(out, `  JSON: {"items":[{"<flag-name>":"<value>"}]}. Use canonical flag names without leading dashes.`)
	fmt.Fprintln(out, "  Arrays are accepted for list-valued flags. Put repeated scalar selectors in separate items.")
	fmt.Fprintln(out, "  Keep config, preview, json, full, raw, force, version, help, and batch-file outside items.")
	fmt.Fprintln(out, "  Environment stays on the command and is shared by every item.")
	fmt.Fprintln(out, "  Choose --batch-file or repeated selectors, not both. Files support 1-100 items and at most 1 MiB.")
	fmt.Fprintln(out, "  Duplicate items and duplicate JSON keys are rejected; at most 100 expanded selections are accepted.")
}

func writeExamples(out io.Writer, examples string) {
	if strings.TrimSpace(examples) == "" {
		return
	}
	fmt.Fprintln(out, "\nexamples:")
	writeIndented(out, examples, "  ")
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

func helpFlagSyntax(flag *pflag.Flag, aliases bool) string {
	label := "--" + flag.Name
	if aliases {
		var names []string
		if alias := flagLongAliases[flag.Name]; alias != "" {
			names = append(names, "--"+alias)
		}
		if flag.Shorthand != "" && flag.ShorthandDeprecated == "" {
			names = append(names, "-"+flag.Shorthand)
		}
		if len(names) > 0 {
			label += " (" + strings.Join(names, ", ") + ")"
		}
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
	if typeName != "bool" {
		label += " <" + value + ">"
	}
	return label
}

func helpFlagText(flag *pflag.Flag) string {
	label := helpFlagSyntax(flag, true)
	typeName := flag.Value.Type()
	usage := flag.Usage
	for _, marker := range []string{" (alias:", " (aliases:"} {
		for {
			index := strings.Index(usage, marker)
			if index < 0 {
				break
			}
			end := strings.Index(usage[index:], ")")
			if end < 0 {
				break
			}
			usage = usage[:index] + usage[index+end+1:]
		}
	}
	var details []string
	_, slice := flag.Value.(pflag.SliceValue)
	_, selector := flag.Value.(*repeatedSelector)
	if slice || selector || typeName == "count" || helpAnnotationTrue(flag, "tadx.help.repeatable") {
		details = append(details, "repeatable")
	}
	if strings.HasSuffix(typeName, "Slice") {
		details = append(details, "comma-separated values accepted")
	}
	if helpRequired(flag) {
		details = append(details, "required")
	}
	if omitted := flag.Annotations["tadx.help.omission"]; len(omitted) > 0 {
		details = append(details, "omitted: "+omitted[0])
	} else if declared := helpDeclaredDefault(flag); declared != "" {
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

func helpRequired(flag *pflag.Flag) bool {
	return helpAnnotationTrue(flag, cobra.BashCompOneRequiredFlag) || helpAnnotationTrue(flag, "tadx.help.required")
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
	if flag.Name == "config" || declared == "" || declared == "[]" || (flag.Value.Type() == "bool" && declared == "false" && len(flag.Annotations["tadx.help.default"]) == 0) {
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
		{"tadx.help.one-required", "at least one of"},
		{"tadx.help.exclusive", "mutually exclusive"},
		{"tadx.help.exactly-one", "exactly one of"},
		{"tadx.help.together", "required together"},
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
	for _, line := range strings.Split(command.Annotations["tadx.help.constraints"], "\n") {
		if strings.TrimSpace(line) != "" {
			lines = appendUnique(lines, line)
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
		if strings.TrimSpace(line) == "" {
			fmt.Fprintln(out)
		} else {
			fmt.Fprintln(out, prefix+strings.TrimSuffix(line, "\r"))
		}
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

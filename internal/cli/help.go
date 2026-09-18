package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// installCategoryHelp derives help from the completed command tree. It deliberately
// avoids action dependencies, flag values, and configuration resolution.
func installCategoryHelp(root *cobra.Command) {
	helpCommand := &cobra.Command{
		Use: "help [command ...]", Short: "Show help for any category or command",
		Annotations:       map[string]string{groupingAnnotation: "true"},
		PersistentPreRun:  func(*cobra.Command, []string) {},
		PersistentPostRun: func(*cobra.Command, []string) {},
		RunE: func(command *cobra.Command, args []string) error {
			target, remaining, err := root.Find(args)
			if err != nil {
				return helpPathError(root, target, args, err)
			}
			if len(remaining) != 0 {
				return helpPathError(root, target, args, nil)
			}
			return target.Help()
		},
	}
	// Route validation must see an invalid path even when a trailing flag would
	// otherwise be rejected by the help command's own flag set first.
	helpCommand.FParseErrWhitelist.UnknownFlags = true
	root.SetHelpCommand(helpCommand)
	root.SetHelpFunc(func(command *cobra.Command, _ []string) {
		out := command.OutOrStdout()
		if writeContentPilotHelp(out, command) {
			return
		}
		writeStructuredHelp(out, command)
	})
}

func helpPathError(root, target *cobra.Command, args []string, cause error) error {
	path := helpPathTokens(args)
	if len(path) == 0 {
		path = append(path, args...)
	}
	nearest := root
	if target != nil {
		nearest = target
	}
	nearestPath := nearest.CommandPath()
	if nearestPath == "" {
		nearestPath = root.Name()
	}
	summary := fmt.Sprintf("unknown help path %q", strings.Join(path, " "))
	if len(path) == 0 {
		summary = "help accepts a command path"
	}
	if cause != nil && len(path) == 0 {
		summary = cause.Error()
	}
	return &errs.Error{
		Kind:             errs.KindUsage,
		Operation:        "cli.help",
		Summary:          summary,
		Cause:            cause,
		CorrectiveAction: fmt.Sprintf("Use %s -h for a supported route.", nearestPath),
		Phase:            errs.PhaseValidation,
		Outcome:          errs.OutcomeNotAttempted,
	}
}

func helpPathTokens(args []string) []string {
	path := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--" || strings.HasPrefix(arg, "-") {
			break
		}
		path = append(path, arg)
	}
	return path
}

// ValidateHelpArgs validates a help-bearing invocation before Cobra dispatches
// a grouping command's built-in help short circuit.
func ValidateHelpArgs(root *cobra.Command, args []string) error {
	if root == nil || !hasHelpArgument(args) {
		return nil
	}
	pathArgs := args
	if len(pathArgs) > 0 && pathArgs[0] == "help" {
		pathArgs = pathArgs[1:]
	}
	path := helpRoutePath(root, pathArgs)
	target, remaining, err := root.Find(path)
	if err != nil {
		return helpPathError(root, target, path, err)
	}
	if len(remaining) == 0 {
		return nil
	}
	// Keep the established root-help fallback for the not-registered config
	// spelling. Configuration remains a global --config flag, not a command.
	if len(path) == 1 && path[0] == "config" && target == root {
		return nil
	}
	return helpPathError(root, target, path, nil)
}

func hasHelpArgument(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || strings.HasPrefix(arg, "--help=") {
			return true
		}
	}
	return false
}

func helpRoutePath(root *cobra.Command, args []string) []string {
	path := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if len(path) > 0 {
				break
			}
			name := strings.TrimLeft(arg, "-")
			if flagName, _, found := strings.Cut(name, "="); found {
				name = flagName
			}
			flag := root.PersistentFlags().Lookup(name)
			if flag != nil && flag.NoOptDefVal == "" && !strings.Contains(arg, "=") && index+1 < len(args) {
				index++
			}
			continue
		}
		path = append(path, arg)
	}
	return path
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
		{"setup and diagnostics", []string{"env", "auth", "mutation", "policy", "agent", "doctor", "capability", "update", "version", "completion"}},
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
	fmt.Fprintln(out, "\nGlobal flags:")
	for _, flag := range helpFlags(root) {
		if referenceSharedFlag(flag.Name) {
			fmt.Fprintln(out, "  "+referenceSharedText(flag))
		} else {
			fmt.Fprintf(out, "  %s (%s)\n", referenceFlagSyntax(flag), flag.Usage)
		}
	}
	fmt.Fprintln(out, "  --help (-h) help only")
	fmt.Fprintln(out, "\nUse complete resource help for shared details, or direct known verb help for focused action syntax. Inspect reads details; pull writes local files.")
	if root.Example != "" {
		writeExamples(out, root.Example)
	} else {
		var examples []string
		for _, path := range []string{"auth check", "content workbook publish", "pulse metric fork"} {
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
		fmt.Fprintf(out, "  %s: %s\n", command.Name(), description)
		var resources []string
		for _, child := range visibleHelpChildren(command) {
			resources = append(resources, child.Name())
		}
		if len(resources) > 0 {
			fmt.Fprintf(out, "    %s\n", strings.Join(resources, ", "))
		}
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

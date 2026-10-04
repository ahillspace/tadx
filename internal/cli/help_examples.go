package cli

import (
	"strings"

	admincli "github.com/ahillspace/tadx/internal/cli/admin"
	agentcli "github.com/ahillspace/tadx/internal/cli/agent"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	cachecli "github.com/ahillspace/tadx/internal/cli/cache"
	capabilitycli "github.com/ahillspace/tadx/internal/cli/capability"
	catalogcli "github.com/ahillspace/tadx/internal/cli/catalog"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	envcli "github.com/ahillspace/tadx/internal/cli/env"
	"github.com/ahillspace/tadx/internal/cli/helpmeta"
	mutationcli "github.com/ahillspace/tadx/internal/cli/mutation"
	pulsecli "github.com/ahillspace/tadx/internal/cli/pulse"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/spf13/cobra"
)

// The root gathers presentation metadata; command owners define every fact.
var categoryHelpExamples = func() []helpmeta.ExampleSet {
	var examples []helpmeta.ExampleSet
	examples = append(examples, authcli.HelpExamples()...)
	examples = append(examples, envcli.HelpExamples()...)
	examples = append(examples, cachecli.HelpExamples()...)
	examples = append(examples, catalogcli.HelpExamples()...)
	examples = append(examples, contentcli.HelpExamples()...)
	examples = append(examples, admincli.HelpExamples()...)
	examples = append(examples, pulsecli.HelpExamples()...)
	examples = append(examples, workspacecli.HelpExamples()...)
	examples = append(examples, agentcli.HelpExamples()...)
	examples = append(examples, mutationcli.HelpExamples()...)
	examples = append(examples, capabilitycli.HelpExamples()...)
	return examples
}()

func applyHelpExamples(root *cobra.Command) {
	for _, entry := range categoryHelpExamples {
		command := helpCommandAt(root, strings.Fields(entry.Path))
		if command == nil {
			continue
		}
		if entry.Common {
			helpmeta.AppendNote(command, entry.Note)
		}
		for _, example := range entry.Examples {
			words := strings.Fields(example)
			target, _, err := root.Find(words[1:])
			if err != nil || !target.Runnable() || target == root {
				continue
			}
			if !strings.Contains(target.Example, example) {
				target.Example = strings.TrimSpace(target.Example + "\n" + example)
			}
		}
	}
	contentcli.ApplySyntaxNotes(root)
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

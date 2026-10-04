package cli

import (
	"maps"
	"strings"

	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

func isContentReference(resource *cobra.Command) bool {
	return resource != nil && resource.Parent() != nil && resource.Parent().Name() == "content"
}

// Both complete and focused help use the registered flags and the same notes.
// Presentation copies never change parsing, validation, or caller-supplied values.
func contentReferenceFlagSyntax(resource, action *cobra.Command, flag *pflag.Flag) string {
	copy := *flag
	copy.Annotations = maps.Clone(flag.Annotations)
	if copy.Annotations == nil {
		copy.Annotations = map[string][]string{}
	}
	value := contentcli.ReferenceFlagValue(resource.Name(), action.Name(), flag.Name)
	if value != "" {
		copy.Annotations["tadx.help.value"] = []string{value}
	}
	if helpRequired(flag) {
		delete(copy.Annotations, "tadx.help.omission")
	}
	return referenceFlagSyntax(&copy)
}

func contentReferenceTerms(resource, action *cobra.Command) []string {
	var terms []string
	used := map[string]bool{}
	for _, flag := range helpFlags(action) {
		if used[flag.Name] || flag.Name == "batch-file" || referenceSharedFlag(flag.Name) && !helpRequired(flag) {
			continue
		}
		var alternatives []string
		for _, group := range flag.Annotations["tadx.help.exactly-one"] {
			for _, name := range strings.Fields(group) {
				if candidate := action.Flags().Lookup(name); candidate != nil && !candidate.Hidden {
					alternatives = append(alternatives, contentReferenceFlagSyntax(resource, action, candidate))
					used[name] = true
				}
			}
			break
		}
		if len(alternatives) > 1 {
			terms = append(terms, "("+strings.Join(alternatives, " | ")+")")
			continue
		}
		term := contentReferenceFlagSyntax(resource, action, flag)
		if !helpRequired(flag) {
			term = "[" + term + "]"
		}
		terms = append(terms, term)
	}
	return terms
}

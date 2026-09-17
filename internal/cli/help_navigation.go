package cli

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

type navigationResource struct {
	command *cobra.Command
	verbs   []string
}

// writeCategoryNavigation shares verb names only, never their flags or behavior.
func writeCategoryNavigation(out io.Writer, category *cobra.Command, summary func(*cobra.Command) string) {
	var resources []navigationResource
	for _, child := range visibleHelpChildren(category) {
		entry := navigationResource{command: child}
		for _, action := range visibleHelpChildren(child) {
			entry.verbs = append(entry.verbs, action.Name())
		}
		resources = append(resources, entry)
	}
	fmt.Fprintf(out, "Usage: %s <resource> <verb> [flags]\n", category.CommandPath())
	var shared []string
	for {
		scope, verbs, line := navigationSharedVerbs(resources)
		if len(scope) == 0 {
			break
		}
		shared = append(shared, line)
		for _, index := range scope {
			resources[index].verbs = slices.DeleteFunc(resources[index].verbs, func(verb string) bool {
				return slices.Contains(verbs, verb)
			})
		}
	}
	if len(shared) > 0 {
		fmt.Fprintln(out, "\n"+strings.Join(shared, "\n"))
	}
	fmt.Fprintln(out, "\nResources:")
	for _, entry := range resources {
		fmt.Fprintf(out, "  %s: %s\n", entry.command.Name(), summary(entry.command))
		if len(entry.verbs) > 0 {
			fmt.Fprintf(out, "    %s\n", strings.Join(entry.verbs, ", "))
		}
	}
	fmt.Fprintf(out, "\nUse %s <resource> -h for complete syntax.\n", category.CommandPath())
}

// Candidate scopes come from actual verb membership, avoiding invented groups
// and exponential subset enumeration. Pick only a grouping that shortens output.
func navigationSharedVerbs(resources []navigationResource) (bestScope []int, bestVerbs []string, bestLine string) {
	var seen [][]int
	bestSavings := 0
	for _, entry := range resources {
		for _, verb := range entry.verbs {
			var scope []int
			for index, candidate := range resources {
				if slices.Contains(candidate.verbs, verb) {
					scope = append(scope, index)
				}
			}
			if len(scope) < 2 || slices.ContainsFunc(seen, func(previous []int) bool { return slices.Equal(previous, scope) }) {
				continue
			}
			seen = append(seen, scope)
			var common, names []string
			for _, candidate := range resources[scope[0]].verbs {
				if !slices.ContainsFunc(scope, func(index int) bool { return !slices.Contains(resources[index].verbs, candidate) }) {
					common = append(common, candidate)
				}
			}
			for _, index := range scope {
				names = append(names, resources[index].command.Name())
			}
			label := "Shared verbs"
			if len(scope) != len(resources) {
				label += " (" + strings.Join(names, ", ") + ")"
			}
			line := label + ": " + strings.Join(common, ", ")
			savings := -len(line) - 2 // Include the shared section's separating newline.
			for _, index := range scope {
				remaining := slices.DeleteFunc(slices.Clone(resources[index].verbs), func(candidate string) bool {
					return slices.Contains(common, candidate)
				})
				savings += navigationVerbLineSize(resources[index].verbs) - navigationVerbLineSize(remaining)
			}
			if savings > bestSavings {
				bestScope, bestVerbs, bestLine, bestSavings = scope, common, line, savings
			}
		}
	}
	return bestScope, bestVerbs, bestLine
}

func navigationVerbLineSize(verbs []string) int {
	if len(verbs) == 0 {
		return 0
	}
	return len("    ") + len(strings.Join(verbs, ", ")) + len("\n")
}

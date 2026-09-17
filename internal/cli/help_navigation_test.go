package cli

import (
	"bytes"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCategoryNavigationPreservesExactVerbCoverage(t *testing.T) {
	for _, test := range []struct {
		name      string
		resources map[string]string
		shared    string
	}{
		{"content", map[string]string{
			"datasource": "delete inspect list move publish pull schema update",
			"flow":       "delete inspect list move publish pull update",
			"project":    "create delete inspect list move update",
			"workbook":   "delete inspect list move publish pull update",
		}, "Shared verbs: delete, inspect, list, move, update"},
		{"catalog", map[string]string{
			"audit": "", "search": "", "lineage": "pull",
			"column": "inspect list update", "database": "inspect list update",
			"label": "delete inspect list update", "table": "inspect list update",
		}, "Shared verbs (column, database, label, table): inspect, list, update"},
		{"admin", map[string]string{
			"group": "create delete inspect list update", "group-member": "add remove",
			"label-category": "create delete inspect list update", "label-value": "delete inspect list update",
			"permission": "create delete inspect", "user": "create delete inspect list update",
		}, "Shared verbs (group, label-category, label-value, user): delete, inspect, list, update"},
		{"pulse", map[string]string{
			"definition": "create delete inspect list publish pull",
			"metric":     "delete follow followers fork inspect list unfollow",
		}, "Shared verbs: delete, inspect, list"},
		{"sparse", map[string]string{"one": "list", "two": "inspect", "action": ""}, ""},
		{"singleton", map[string]string{"one": "delete inspect list update"}, ""},
		{"short", map[string]string{"one": "list", "two": "list"}, ""},
		{"costly-scope", map[string]string{"long-first-resource": "inspect", "long-second-resource": "inspect", "status": ""}, ""},
		{"empty", map[string]string{}, ""},
		{"custom", map[string]string{"alpha": "archive inspect restore", "beta": "archive inspect restore", "status": ""}, "Shared verbs (alpha, beta): archive, inspect, restore"},
	} {
		t.Run(test.name, func(t *testing.T) {
			category := navigationTestCategory(test.name, test.resources)
			var output bytes.Buffer
			writeCategoryNavigation(&output, category, helpPlainShort)
			got := output.String()
			if test.shared != "" && !strings.Contains(got, test.shared) {
				t.Errorf("missing shared group %q:\n%s", test.shared, got)
			}
			if test.shared == "" && strings.Contains(got, "Shared verbs") {
				t.Errorf("unexpected shared group:\n%s", got)
			}
			assertNavigationCoverage(t, got, test.resources)
			for resource := range test.resources {
				if !strings.Contains(got, "  "+resource+": Description of "+resource) {
					t.Errorf("missing description for %s:\n%s", resource, got)
				}
			}
			if test.shared != "" {
				if strings.Index(got, "Shared verbs") > strings.Index(got, "Resources:") {
					t.Errorf("shared verbs follow resources:\n%s", got)
				}
				var baseline bytes.Buffer
				baseline.WriteString("Usage: tadx " + test.name + " <resource> <verb> [flags]\n\nResources:\n")
				for _, resource := range category.Commands() {
					baseline.WriteString("  " + resource.Name() + ": " + resource.Short + "\n")
					if verbs := strings.Fields(test.resources[resource.Name()]); len(verbs) > 0 {
						baseline.WriteString("    " + strings.Join(verbs, ", ") + "\n")
					}
				}
				baseline.WriteString("\nUse tadx " + test.name + " <resource> -h for complete syntax.\n")
				if len(got) >= baseline.Len() {
					t.Errorf("shared groups did not shorten output: %d >= %d", len(got), baseline.Len())
				}
			}
		})
	}
}

func TestCategoryNavigationUsesCurrentVisibleTree(t *testing.T) {
	resources := map[string]string{"alpha": "inspect publish", "beta": "inspect publish"}
	category := navigationTestCategory("custom", resources)
	category.AddCommand(&cobra.Command{Use: "secret", Hidden: true})
	alpha := category.Commands()[0]
	alpha.AddCommand(&cobra.Command{Use: "private", Hidden: true})
	var before bytes.Buffer
	writeCategoryNavigation(&before, category, helpPlainShort)
	if strings.Contains(before.String(), "secret") || strings.Contains(before.String(), "private") {
		t.Fatal("hidden command leaked into navigation")
	}
	alpha.AddCommand(&cobra.Command{Use: "create", Run: func(*cobra.Command, []string) { panic("help ran action") }})
	resources["alpha"] += " create"
	var after bytes.Buffer
	writeCategoryNavigation(&after, category, helpPlainShort)
	assertNavigationCoverage(t, after.String(), resources)
	if before.String() == after.String() {
		t.Fatal("navigation did not reflect a newly registered verb")
	}
}

func navigationTestCategory(name string, resources map[string]string) *cobra.Command {
	root := &cobra.Command{Use: "tadx"}
	category := &cobra.Command{Use: name}
	root.AddCommand(category)
	for _, name := range slices.Sorted(maps.Keys(resources)) {
		resource := &cobra.Command{Use: name, Short: "Description of " + name}
		for verb := range strings.FieldsSeq(resources[name]) {
			resource.AddCommand(&cobra.Command{Use: verb, Run: func(*cobra.Command, []string) { panic("help ran action") }})
		}
		category.AddCommand(resource)
	}
	return category
}

// Read the displayed scopes and additions as a user would, checking every
// resource independently rather than reconstructing the grouping algorithm.
func assertNavigationCoverage(t *testing.T, output string, expected map[string]string) {
	t.Helper()
	actual := map[string][]string{}
	for name := range expected {
		actual[name] = nil
	}
	current := ""
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "Shared verbs") {
			label, verbs, ok := strings.Cut(line, ": ")
			if !ok {
				t.Fatalf("invalid shared verb row: %s", line)
			}
			scope := slices.Sorted(maps.Keys(expected))
			if label != "Shared verbs" {
				if !strings.HasPrefix(label, "Shared verbs (") || !strings.HasSuffix(label, ")") {
					t.Fatalf("invalid shared scope: %s", label)
				}
				scope = strings.Split(strings.TrimSuffix(strings.TrimPrefix(label, "Shared verbs ("), ")"), ", ")
			}
			for _, name := range scope {
				if _, ok := expected[name]; !ok {
					t.Fatalf("invented resource %s", name)
				}
				actual[name] = append(actual[name], strings.Split(verbs, ", ")...)
			}
		} else if strings.HasPrefix(line, "    ") {
			actual[current] = append(actual[current], strings.Split(strings.TrimSpace(line), ", ")...)
		} else if strings.HasPrefix(line, "  ") {
			current, _, _ = strings.Cut(strings.TrimSpace(line), ": ")
		}
	}
	for name, verbs := range expected {
		want := strings.Fields(verbs)
		slices.Sort(want)
		slices.Sort(actual[name])
		if !slices.Equal(actual[name], want) {
			t.Errorf("%s verbs = %v, want %v:\n%s", name, actual[name], want, output)
		}
	}
}

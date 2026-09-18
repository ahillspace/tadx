package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func contentPresentationTree(resourceName string, actionNames ...string) (*cobra.Command, *cobra.Command) {
	root := &cobra.Command{Use: "tadx"}
	root.PersistentFlags().Bool("json", false, "render JSON")
	category := &cobra.Command{Use: "content"}
	resource := &cobra.Command{Use: resourceName}
	for _, name := range actionNames {
		action := &cobra.Command{Use: name, Run: func(*cobra.Command, []string) { panic("help ran action") }}
		action.Flags().String("environment", "", "environment")
		resource.AddCommand(action)
	}
	category.AddCommand(resource)
	root.AddCommand(category)
	return root, resource
}

func contentPresentationOutput(t *testing.T, command *cobra.Command) string {
	t.Helper()
	var out bytes.Buffer
	if !writeContentPilotHelp(&out, command) {
		t.Fatal("command did not resolve to content pilot")
	}
	return out.String()
}

func TestContentPresentationRequiredOmissionDoesNotMutateFlags(t *testing.T) {
	root, resource := contentPresentationTree("flow", "update")
	action := resource.Commands()[0]
	action.Flags().String("owner-id", "", "replacement owner")
	applyHelpValues(root)
	flag := action.Flags().Lookup("owner-id")
	before := helpFlagText(flag)
	annotations := make(map[string][]string, len(flag.Annotations))
	for key, values := range flag.Annotations {
		annotations[key] = append([]string(nil), values...)
	}
	got := contentPresentationOutput(t, resource)
	if !strings.Contains(got, "(required) replacement owner LUID") || strings.Contains(got, "omitted: unchanged") {
		t.Fatalf("required owner reference is contradictory:\n%s", got)
	}
	if after := helpFlagText(flag); after != before || !reflect.DeepEqual(flag.Annotations, annotations) {
		t.Fatal("content help changed original flag presentation metadata")
	}
	if !strings.Contains(before, "omitted: unchanged") {
		t.Fatal("fixture did not exercise contradictory legacy omission metadata")
	}
}

func TestContentPresentationDescriptionsAndEnvironmentScope(t *testing.T) {
	root, resource := contentPresentationTree("project", "create", "update")
	for _, action := range resource.Commands() {
		action.Flags().String("description", "", "description")
	}
	applyHelpValues(root)
	got := contentPresentationOutput(t, resource)
	create := contentPresentationSection(t, got, "flags{create}:")
	update := contentPresentationSection(t, got, "flags{update}:")
	if strings.Contains(create, "clears") || !strings.Contains(update, "empty clears") {
		t.Fatalf("description semantics escaped their action scope:\n%s", got)
	}
	if !strings.Contains(got, "reads use read default; remote writes require exactly one configured environment") {
		t.Fatal("environment guidance does not distinguish remote writes from pull")
	}
}

func TestContentPresentationGroupsAndValuesAreDeterministic(t *testing.T) {
	root, resource := contentPresentationTree("workbook", "publish", "pull", "inspect")
	for _, action := range resource.Commands() {
		action.Flags().String("id", "", "content identity")
		if action.Name() != "publish" {
			action.Flags().String("extra-filter", "", "shared filter")
		}
		if action.Name() == "pull" {
			action.Flags().Bool("overwrite", false, "replace local artifact")
		}
	}
	applyHelpValues(root)
	baseline := contentPresentationOutput(t, resource)
	subset := contentPresentationSection(t, baseline, "flags{inspect,pull}:")
	if !strings.Contains(subset, "--extra-filter") || !strings.Contains(subset, "authoritative LUID") {
		t.Fatalf("shared subset flags lost their scope:\n%s", baseline)
	}
	publish := contentPresentationSection(t, baseline, "flags{publish}:")
	if strings.Contains(publish, "--extra-filter") || !strings.Contains(publish, "source LUID in workspace") {
		t.Fatalf("publish flag semantics incorrectly merged:\n%s", baseline)
	}
	if strings.Count(baseline, "--json ") != 1 || strings.Count(baseline, "--extra-filter ") != 1 {
		t.Fatal("identical flag explanations were repeated")
	}
	for _, action := range resource.Commands() {
		if err := action.Flags().Set("id", "different-supplied-value"); err != nil {
			t.Fatal(err)
		}
		focused := contentPresentationOutput(t, action)
		if !strings.Contains(focused, "Usage: tadx content workbook "+action.Name()+" [flags]") {
			t.Fatalf("%s help is not focused:\n%s", action.Name(), focused)
		}
		if focused == baseline {
			t.Fatalf("%s help repeats the complete resource reference", action.Name())
		}
	}
	for range 10 {
		if got := contentPresentationOutput(t, resource); got != baseline {
			t.Fatal("help ordering is not deterministic")
		}
	}
}

func TestContentFocusedHelpKeepsSharedConstraintsAndRelatedRoute(t *testing.T) {
	root, resource := contentPresentationTree("workbook", "publish", "pull")
	var publish *cobra.Command
	for _, action := range resource.Commands() {
		if action.Name() == "publish" {
			publish = action
		}
	}
	if publish == nil {
		t.Fatal("publish action missing")
	}
	publish.Flags().String("id", "", "content identity")
	publish.Flags().String("project", "", "destination project")
	publish.MarkFlagsOneRequired("id", "project")
	applyHelpValues(root)
	focused := contentPresentationOutput(t, publish)
	for _, want := range []string{
		"Usage: tadx content workbook publish [flags]",
		"flags{publish}:",
		"--json",
		"at least one of: --id, --project",
		"tadx admin permission -h",
	} {
		if !strings.Contains(focused, want) {
			t.Errorf("focused content help missing %q:\n%s", want, focused)
		}
	}
	if strings.Contains(focused, "\n  pull:") || strings.Contains(focused, "--extra-filter") {
		t.Fatalf("focused content help contains sibling details:\n%s", focused)
	}
}

func TestContentPresentationListCacheEffectsAreScoped(t *testing.T) {
	for _, resourceName := range []string{"workbook", "flow", "project"} {
		t.Run(resourceName, func(t *testing.T) {
			_, resource := contentPresentationTree(resourceName, "list")
			got := contentPresentationOutput(t, resource)
			for _, want := range []string{"bounded live reads do not touch cache", "Live --all collects <=10000", "unfiltered replaces the resource scope", "filtered saves observations", "Cache failures warn without losing the live result", "--all rejects incomplete coverage/overflow", "--cache remains local-only"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing list behavior %q", want)
				}
			}
			_, withoutList := contentPresentationTree(resourceName, "inspect")
			if strings.Contains(contentPresentationOutput(t, withoutList), "Live --all") {
				t.Fatal("list note rendered without an available list action")
			}
		})
	}
}

func contentPresentationSection(t *testing.T, output, heading string) string {
	t.Helper()
	_, section, found := strings.Cut(output, heading+"\n")
	if !found {
		t.Fatalf("missing section %s", heading)
	}
	section, _, _ = strings.Cut(section, "\n\n")
	return section
}

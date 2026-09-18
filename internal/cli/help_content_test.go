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
	if !strings.Contains(got, "--owner-id <luid>") || strings.Contains(got, "[--owner-id") || strings.Contains(got, "omitted: unchanged") {
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
	create := contentPresentationSection(t, got, "  create: Create a remote project")
	update := contentPresentationSection(t, got, "  update: Change remote metadata")
	if strings.Contains(create, "clears") || !strings.Contains(update, "Empty --description clears") {
		t.Fatalf("description semantics escaped their action scope:\n%s", got)
	}
	if !strings.Contains(got, "reads: default; writes: sole configured env") {
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
	subset := contentPresentationSection(t, baseline, "  inspect: Read details, not files")
	if !strings.Contains(subset, "--extra-filter") || !strings.Contains(subset, "--id <luid>") {
		t.Fatalf("shared subset flags lost their scope:\n%s", baseline)
	}
	publish := contentPresentationSection(t, baseline, "  publish: Publish local content to Tableau")
	if strings.Contains(publish, "--extra-filter") || !strings.Contains(publish, "workspace ID/name/relative artifact") {
		t.Fatalf("publish flag semantics incorrectly merged:\n%s", baseline)
	}
	if strings.Count(baseline, "--json") != 1 || strings.Count(baseline, "--extra-filter ") != 2 {
		t.Fatal("global flags repeated or action-local scope was lost")
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
		"  publish: Publish local content to Tableau",
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
			for _, want := range []string{"Bounded reads never cache", "list --all: <=10000", "unfiltered replaces scope", "filtered merges", "cache failures retain live results", "incomplete/overflow fails"} {
				if !strings.Contains(got, want) {
					t.Errorf("missing list behavior %q", want)
				}
			}
			_, withoutList := contentPresentationTree(resourceName, "inspect")
			if strings.Contains(contentPresentationOutput(t, withoutList), "list --all") {
				t.Fatal("list note rendered without an available list action")
			}
		})
	}
}

func TestContentHelpUsesRegisteredDefinitionsAtBothLevels(t *testing.T) {
	for _, name := range []string{"workbook", "datasource", "flow", "project"} {
		t.Run(name, func(t *testing.T) {
			actions := []string{"delete", "inspect", "list", "move", "publish", "pull", "update"}
			if name == "datasource" {
				actions = append(actions, "schema")
			}
			if name == "project" {
				actions = []string{"create", "delete", "inspect", "list", "move", "update"}
			}
			_, resource := contentPresentationTree(name, actions...)
			action, _, err := resource.Find([]string{"inspect"})
			if err != nil {
				t.Fatal(err)
			}
			action.Flags().String("fixture-scope", "", "a newly registered scope")
			action.Flags().String("fixture-id", "", "a newly registered identity")
			action.MarkFlagsMutuallyExclusive("fixture-scope", "fixture-id")
			for _, node := range []*cobra.Command{resource, action} {
				got := contentPresentationOutput(t, node)
				for _, want := range []string{"--fixture-scope", "--fixture-id", "mutually exclusive:"} {
					if !strings.Contains(got, want) {
						t.Errorf("%s output omits newly registered %q:\n%s", node.Name(), want, got)
					}
				}
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

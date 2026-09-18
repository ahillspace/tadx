package app

import (
	"os"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/capability"
	"github.com/ahillspace/tadx/internal/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Build the content inventory with the same service registrations as Run; execute
// every help request below through Run rather than through this discovery tree.
func contentHelpPilotTree(t *testing.T, options Options) *cobra.Command {
	t.Helper()
	runtime, err := newRuntime(options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := runtime.Close(); err != nil {
			t.Error(err)
		}
	})
	root := cli.NewRoot(cli.Dependencies{
		Content: newRemoteContentCommands(runtime).dependencies(), ContentLabels: contentLabelDependencies(runtime),
		WorkbookPuller: &pullService{runtime: runtime}, WorkbookPublisher: &publishService{runtime: runtime},
		WorkbookPullUse: registryLeafUse("workbook.pull"), WorkbookPullShort: registryShort("workbook.pull"),
		WorkbookPublishUse: registryLeafUse("workbook.publish"), WorkbookPublishShort: registryShort("workbook.publish"),
		BatchSelectors: capability.BatchSelectors(), BatchOptions: capability.BatchOptions(),
	})
	content, _, err := root.Find([]string{"content"})
	if err != nil || content.Name() != "content" {
		t.Fatalf("content command missing: %v", err)
	}
	return content
}

func contentHelpPilotSetup(t *testing.T) (string, Options) {
	t.Helper()
	dir := t.TempDir()
	options := overviewOptions(t, dir)
	if err := os.WriteFile(options.ConfigPath, []byte("invalid: ["), 0600); err != nil {
		t.Fatal(err)
	}
	return dir, options
}

func contentHelpPilotRun(t *testing.T, dir string, options Options, args ...string) string {
	t.Helper()
	code, output := runOverview(t, dir, args, options)
	if code != 0 {
		t.Fatalf("%v exited %d: %s", args, code, output)
	}
	if strings.Contains(output, "private-help-input") || strings.Contains(output, options.ConfigPath) {
		t.Fatalf("%v exposed supplied values or configuration location", args)
	}
	return output
}

func TestContentHelpPilotNavigation(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	content := contentHelpPilotTree(t, options)
	output := contentHelpPilotRun(t, dir, options, "content", "--help")
	if size := utf8.RuneCountInString(output); size > 1800 {
		t.Errorf("content navigation has %d characters; budget is 1800", size)
	}
	if strings.Contains(output, "--artifact") || strings.Contains(output, "flags{publish}") {
		t.Error("content navigation expands operational flag manuals")
	}
	for _, resource := range content.Commands() {
		if resource.Hidden {
			continue
		}
		if !strings.Contains(output, resource.Name()) {
			t.Errorf("navigation omits resource %s", resource.Name())
		}
		for _, action := range resource.Commands() {
			if !action.Hidden && !strings.Contains(output, action.Name()) {
				t.Errorf("navigation omits %s %s", resource.Name(), action.Name())
			}
		}
	}
	for _, args := range [][]string{{"content", "-h"}, {"help", "content"}, {"con", "--help"}} {
		if got := contentHelpPilotRun(t, dir, options, args...); got != output {
			t.Errorf("%v differs from content navigation", args)
		}
	}
}

func TestContentHelpPilotResourceReferencesAndVerbMirrors(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	content := contentHelpPilotTree(t, options)
	resources := []string{"workbook", "datasource", "flow", "project"}
	// Complete generated references use the same 4000-character ceiling as every
	// other resource, including selectors and constraints omitted by the old text pages.
	budgets := map[string]int{"workbook": 4000, "datasource": 4000, "flow": 4000, "project": 4000}
	for _, name := range resources {
		t.Run(name, func(t *testing.T) {
			resource, _, err := content.Find([]string{name})
			if err != nil || resource.Name() != name {
				t.Fatalf("resource missing: %v", err)
			}
			want := contentHelpPilotRun(t, dir, options, "content", name, "--help")
			for _, section := range []string{"Usage:", "Shared:", "Commands:", "Rules:", "Batch:", "Examples:"} {
				if !strings.Contains(want, section) {
					t.Errorf("%s reference omits formatted section %q", name, section)
				}
			}
			for _, obsolete := range []string{"flags{", "--id (-i)", "--full (--ful", "inspect (ins)", "delete (del)"} {
				if strings.Contains(want, obsolete) {
					t.Errorf("%s reference restores rejected help decoration %q", name, obsolete)
				}
			}
			facts := map[string][]string{
				"workbook": {"--project scopes an exact --name selector when supplied", "(default: true)", "Empty --description clears"},
				"flow":     {"--owner-id <luid>", "--file <file.tfl|file.tflx>", "--project-id <luid>"},
				"project":  {"--id <luid>", "LockedToProjectWithoutNested", "--top-level", "--name = --new-name (update)"},
			}
			for _, fact := range facts[name] {
				if !strings.Contains(want, fact) {
					t.Errorf("%s reference lost resource-specific fact %q", name, fact)
				}
			}
			if strings.Contains(want, "--as-job") {
				t.Errorf("%s reference advertises removed publication job mode", name)
			}
			if name != "project" {
				for _, meaning := range []string{"Read details, not files", "Download local files", "Publish local content to Tableau"} {
					if !strings.Contains(want, meaning) {
						t.Errorf("%s reference omits operation meaning %q", name, meaning)
					}
				}
			}
			if size := utf8.RuneCountInString(want); size > budgets[name] {
				t.Errorf("%s reference has %d characters; budget is %d", name, size, budgets[name])
			}
			if name == "datasource" {
				flat := strings.Join(strings.Fields(want), " ")
				for _, fact := range []string{
					"local only; default: live", "no writes; mutation gate can be off", "details, not rows",
					"--id <luid>", "--name <name>", "--project or --project-id",
					"(default: 25)", "(default: 20)", "measure|dimension|date|excluded",
					"dirty local files", "Project: destination", "create: collision fails", "append/replace: data",
					"at least one of: --new-name, --owner-id", "omitted settings stay unchanged", "--field-id: <=10000",
					"--batch-file", "File OR repeated selectors", "1-100 sequential items", "Env/control flags outside rows",
				} {
					if !strings.Contains(flat, fact) {
						t.Errorf("compact reference lost %q", fact)
					}
				}
			}
			for _, sibling := range resources {
				if sibling != name && strings.Contains(want, "tadx content "+sibling+" ") {
					t.Errorf("%s reference includes sibling operation %s", name, sibling)
				}
			}
			assertMirror := func(args []string) {
				t.Helper()
				if got := contentHelpPilotRun(t, dir, options, args...); got != want {
					t.Errorf("%v does not mirror its resource reference", args)
				}
			}
			assertFocused := func(args []string, action *cobra.Command) {
				t.Helper()
				got := contentHelpPilotRun(t, dir, options, args...)
				if got == want {
					t.Errorf("%v repeats the complete resource reference", args)
				}
				usage := "Usage: " + action.CommandPath() + " [flags]"
				if !strings.Contains(got, usage) {
					t.Errorf("%v omits focused usage %q", args, usage)
				}
				for _, sibling := range resource.Commands() {
					if sibling.Hidden || sibling == action {
						continue
					}
					if strings.Contains(got, "\n  "+sibling.Name()+" (") {
						t.Errorf("%v includes sibling operation %s", args, sibling.Name())
					}
				}
				action.LocalNonPersistentFlags().VisitAll(func(flag *pflag.Flag) {
					if !flag.Hidden && !strings.Contains(got, "--"+flag.Name) {
						t.Errorf("%v focused help omits --%s", args, flag.Name)
					}
				})
			}
			assertMirror([]string{"content", name, "-h"})
			assertMirror([]string{"help", "content", name})
			for _, alias := range resource.Aliases {
				assertMirror([]string{"content", alias, "--help"})
			}
			for _, alias := range content.Aliases {
				assertMirror([]string{alias, name, "--help"})
			}
			for _, action := range resource.Commands() {
				if action.Hidden {
					continue
				}
				verbPath := []string{"content", name, action.Name()}
				assertFocused(append([]string{"help"}, verbPath...), action)
				for _, help := range []string{"-h", "--help"} {
					assertFocused(append(append([]string{}, verbPath...), help), action)
				}
				args := append(append([]string{}, verbPath...), "--environment", "private-help-input-environment", "--full", "--json")
				if action.Flags().Lookup("name") != nil {
					args = append(args, "--name", "private-help-input-name")
				}
				assertFocused(append(args, "--help"), action)
				for _, alias := range action.Aliases {
					assertFocused([]string{"content", name, alias, "--help"}, action)
				}
				if !strings.Contains(want, action.Name()) {
					t.Errorf("resource reference omits action %s", action.Name())
				}
				checkFlag := func(flag *pflag.Flag) {
					if !flag.Hidden && !strings.Contains(want, "--"+flag.Name) {
						t.Errorf("resource reference omits %s flag --%s", action.Name(), flag.Name)
					}
				}
				action.LocalNonPersistentFlags().VisitAll(checkFlag)
				action.InheritedFlags().VisitAll(checkFlag)
			}
		})
	}
}

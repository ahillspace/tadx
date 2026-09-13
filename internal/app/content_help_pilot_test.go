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
	budgets := map[string]int{"workbook": 5800, "datasource": 6800, "flow": 5200, "project": 4700}
	for _, name := range resources {
		t.Run(name, func(t *testing.T) {
			resource, _, err := content.Find([]string{name})
			if err != nil || resource.Name() != name {
				t.Fatalf("resource missing: %v", err)
			}
			want := contentHelpPilotRun(t, dir, options, "content", name, "--help")
			if size := utf8.RuneCountInString(want); size > budgets[name] {
				t.Errorf("%s reference has %d characters; budget is %d", name, size, budgets[name])
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
				assertMirror(append([]string{"help"}, verbPath...))
				for _, help := range []string{"-h", "--help"} {
					assertMirror(append(append([]string{}, verbPath...), help))
				}
				args := append(append([]string{}, verbPath...), "--environment", "private-help-input-environment", "--full", "--json")
				if action.Flags().Lookup("name") != nil {
					args = append(args, "--name", "private-help-input-name")
				}
				assertMirror(append(args, "--help"))
				for _, alias := range action.Aliases {
					assertMirror([]string{"content", name, alias, "--help"})
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

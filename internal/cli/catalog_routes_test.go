package cli_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	"github.com/ahillspace/tadx/internal/cli"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

type catalogLineageSpy struct{ spy *previewActionSpy }

func (s catalogLineageSpy) PullLineage(_ context.Context, input lineagepull.Input) (lineagepull.Output, error) {
	s.spy.record(input.Preview)
	return lineagepull.Output{}, nil
}

func TestCatalogOwnsLineageAndLabels(t *testing.T) {
	root := cli.NewRoot(previewDependencies(&previewActionSpy{}))
	for path, capabilityID := range map[string]string{
		"catalog lineage pull":  "lineage.pull",
		"cat lin pl":            "lineage.pull",
		"catalog label list":    "content.label.list",
		"catalog label inspect": "content.label.inspect",
		"catalog label update":  "content.label.update",
		"catalog label delete":  "content.label.delete",
		"cat lbl upd":           "content.label.update",
	} {
		command, rest, err := root.Find(strings.Fields(path))
		if err != nil || len(rest) != 0 || command.Annotations[cli.CapabilityAnnotation] != capabilityID {
			t.Errorf("%s: command=%s rest=%v error=%v", path, command.CommandPath(), rest, err)
		}
	}
	for _, path := range []string{"content lineage", "content label", "content lin", "content lbl"} {
		_, rest, err := root.Find(strings.Fields(path))
		if err == nil && len(rest) == 0 {
			t.Errorf("obsolete route still resolves: %s", path)
		}
	}
	content, _, err := root.Find([]string{"content"})
	if err != nil {
		t.Fatal(err)
	}
	var resources []string
	for _, command := range content.Commands() {
		resources = append(resources, command.Name())
	}
	if got := strings.Join(resources, ","); got != "datasource,flow,project,workbook" {
		t.Fatalf("content resources = %s", got)
	}
}

func TestCatalogRoutesWithIndependentDependencies(t *testing.T) {
	for _, test := range []struct {
		name string
		deps cli.Dependencies
		path string
	}{
		{"lineage", cli.Dependencies{Content: &contentcli.Dependencies{LineagePuller: &previewActionSpy{}}}, "catalog lineage pull"},
		{"label", cli.Dependencies{ContentLabels: &contentcli.LabelDependencies{}}, "catalog label list"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := cli.NewRoot(test.deps)
			command, rest, err := root.Find(strings.Fields(test.path))
			if err != nil || len(rest) != 0 || command.CommandPath() != "tadx "+test.path {
				t.Fatalf("independent route missing: rest=%v error=%v", rest, err)
			}
			catalog, _, _ := root.Find([]string{"catalog"})
			if children := catalog.Commands(); len(children) != 1 || children[0].Name() != test.name {
				t.Fatal("independent dependencies exposed unrelated unconfigured actions")
			}
			var out bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs(append([]string{"help"}, strings.Fields(test.path)...))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "usage: tadx "+test.path) {
				t.Fatal("catalog help topic did not render operation")
			}
		})
	}
}

func TestCatalogRelocatedActionsPreservePreviewAndBatchBindings(t *testing.T) {
	for _, test := range []struct {
		path, capabilityID, selector, flags string
	}{
		{"catalog lineage pull", "lineage.pull", "id", "--kind workbook --id workbook-id --workspace test"},
		{"catalog label update", "content.label.update", "id", "--id label-id --message Meaning"},
		{"catalog label delete", "content.label.delete", "id", "--id label-id"},
	} {
		t.Run(test.capabilityID, func(t *testing.T) {
			spy := &previewActionSpy{}
			deps := previewDependencies(spy)
			deps.Content.LineagePuller = catalogLineageSpy{spy: spy}
			deps.BatchSelectors = map[string]string{test.capabilityID: test.selector}
			root := cli.NewRoot(deps)
			command, _, err := root.Find(strings.Fields(test.path))
			if err != nil {
				t.Fatal(err)
			}
			if command.Flags().Lookup("batch-file") == nil || command.Annotations["tadx.batch.selectors"] != test.selector {
				t.Fatal("stable capability ID did not receive batch bindings after relocation")
			}
			root.SetArgs(strings.Fields(test.path + " " + test.flags + " --env test --preview"))
			if err := root.Execute(); err != nil {
				t.Fatal(err)
			}
			if spy.calls != 1 || spy.writes != 0 {
				t.Fatalf("preview calls=%d writes=%d", spy.calls, spy.writes)
			}
		})
	}
}

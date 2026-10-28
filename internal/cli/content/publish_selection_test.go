package content_test

import (
	"reflect"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

func TestWorkbookPublishConvenientSelectors(t *testing.T) {
	for _, selection := range [][]string{{"--file", "Finance.twb"}, {"--id", "source-1"}, {"--artifact-name", "Finance"}} {
		t.Run(selection[0], func(t *testing.T) {
			actions := &contentActions{}
			command := contentcli.New(contentcli.Dependencies{Puller: actions, Publisher: publisher{actions}, Renderer: renderer{}, MutationsEnabled: true})
			args := []string{"workbook", "publish", "--preview", "--environment", "production", "--project-id", "project-1"}
			command.SetArgs(append(args, selection...))
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if len(actions.publishInputs) != 1 {
				t.Fatalf("calls = %d", len(actions.publishInputs))
			}
		})
	}
}

func TestWorkbookPublishParsesRepeatedDatasourceBindings(t *testing.T) {
	actions := &contentActions{}
	command := contentcli.New(contentcli.Dependencies{Puller: actions, Publisher: publisher{actions}, Renderer: renderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"workbook", "publish", "--file", "Finance.twb", "--environment", "production", "--project-id", "project-1", "--datasource-binding", "Profit = Revenue - Cost=ds-1", "--datasource-binding", "Orders=ds-2"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := []workbookops.DatasourceBinding{{Source: "Profit = Revenue - Cost", TargetLUID: "ds-1"}, {Source: "Orders", TargetLUID: "ds-2"}}
	if len(actions.publishInputs) != 1 || !reflect.DeepEqual(actions.publishInputs[0].DatasourceBindings, want) {
		t.Fatalf("publish inputs = %#v", actions.publishInputs)
	}
}

func TestWorkbookPublishRejectsIncompleteDatasourceBindings(t *testing.T) {
	for _, binding := range []string{"Orders", "=ds-1", "Orders=", ""} {
		actions := &contentActions{}
		command := contentcli.New(contentcli.Dependencies{Puller: actions, Publisher: publisher{actions}, Renderer: renderer{}, MutationsEnabled: true})
		command.SetArgs([]string{"workbook", "publish", "--file", "Finance.twb", "--environment", "production", "--project-id", "project-1", "--datasource-binding", binding})
		if err := command.Execute(); err == nil || len(actions.publishInputs) != 0 || !strings.Contains(err.Error(), "<source name or content URL>=<target LUID>") {
			t.Fatalf("binding %q: err = %v calls = %d", binding, err, len(actions.publishInputs))
		}
	}
}

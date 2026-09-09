package content_test

import (
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
	"testing"
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

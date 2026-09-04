package content

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCatalogFlagIsLimitedToEligibleContentReads(t *testing.T) {
	eligible := map[string]bool{
		"workbook list":      newWorkbookList(nil, nil).Flags().Lookup("catalog") != nil,
		"workbook inspect":   newWorkbookInspect(nil, nil).Flags().Lookup("catalog") != nil,
		"datasource list":    childCommand(newDatasourceInventory(nil, nil, nil), "list").Flags().Lookup("catalog") != nil,
		"datasource inspect": childCommand(newDatasourceInventory(nil, nil, nil), "inspect").Flags().Lookup("catalog") != nil,
		"project list":       newProjectList(Dependencies{}).Flags().Lookup("catalog") != nil,
		"project inspect":    newProjectInspect(Dependencies{}).Flags().Lookup("catalog") != nil,
		"flow list":          newFlowList(Dependencies{}).Flags().Lookup("catalog") != nil,
		"flow inspect":       newFlowInspect(Dependencies{}).Flags().Lookup("catalog") != nil,
	}
	for command, present := range eligible {
		if !present {
			t.Errorf("%s is missing --catalog", command)
		}
	}
	if newPull(Dependencies{}).Flags().Lookup("catalog") != nil || newFlowPull(Dependencies{}).Flags().Lookup("catalog") != nil {
		t.Fatal("artifact pull commands must remain live-only")
	}
}

func childCommand(parent *cobra.Command, name string) *cobra.Command {
	for _, command := range parent.Commands() {
		if command.Name() == name {
			return command
		}
	}
	return &cobra.Command{}
}

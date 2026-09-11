package content

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestCacheFlagIsLimitedToEligibleContentReads(t *testing.T) {
	eligible := map[string]bool{
		"workbook list":      newWorkbookList(nil, nil).Flags().Lookup("cache") != nil,
		"workbook inspect":   newWorkbookInspect(nil, nil).Flags().Lookup("cache") != nil,
		"datasource list":    childCommand(newDatasourceInventory(nil, nil, nil), "list").Flags().Lookup("cache") != nil,
		"datasource inspect": childCommand(newDatasourceInventory(nil, nil, nil), "inspect").Flags().Lookup("cache") != nil,
		"project list":       newProjectList(Dependencies{}).Flags().Lookup("cache") != nil,
		"project inspect":    newProjectInspect(Dependencies{}).Flags().Lookup("cache") != nil,
		"flow list":          newFlowList(Dependencies{}).Flags().Lookup("cache") != nil,
		"flow inspect":       newFlowInspect(Dependencies{}).Flags().Lookup("cache") != nil,
	}
	for command, present := range eligible {
		if !present {
			t.Errorf("%s is missing --cache", command)
		}
	}
	if newPull(Dependencies{}).Flags().Lookup("cache") != nil || newFlowPull(Dependencies{}).Flags().Lookup("cache") != nil {
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

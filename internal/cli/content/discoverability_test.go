package content

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestContentMutationCommandsStayDiscoverableWhenDisabled(t *testing.T) {
	commands := map[string]*cobra.Command{
		"workbook publish":   newPublish(Dependencies{MutationsEnabled: false}),
		"workbook delete":    newWorkbookDelete(nil, nil, false),
		"datasource publish": newDatasourcePublish(datasourceLifecycleDependencies{mutationsEnabled: false}),
		"datasource delete":  newDatasourceDelete(datasourceLifecycleDependencies{mutationsEnabled: false}),
		"project create":     newProjectCreate(Dependencies{MutationsEnabled: false}),
		"project update":     newProjectUpdate(Dependencies{MutationsEnabled: false}),
		"flow publish":       newFlowPublish(Dependencies{MutationsEnabled: false}),
		"flow move":          newFlowMove(Dependencies{MutationsEnabled: false}),
		"flow delete":        newFlowDelete(Dependencies{MutationsEnabled: false}),
	}
	for name, command := range commands {
		t.Run(name, func(t *testing.T) {
			if command.Hidden {
				t.Fatalf("%s is hidden", name)
			}
		})
	}
}

func TestPublishArtifactHelpExplainsManagedWorkspacePath(t *testing.T) {
	commands := map[string]struct {
		usage   string
		example string
	}{
		"workbook": {
			usage:   newPublish(Dependencies{}).Flags().Lookup("artifact").Usage,
			example: "artifacts/workbook/Finance--identity",
		},
		"datasource": {
			usage:   newDatasourcePublish(datasourceLifecycleDependencies{}).Flags().Lookup("artifact").Usage,
			example: "artifacts/datasource/Sales--identity",
		},
		"flow": {
			usage:   newFlowPublish(Dependencies{}).Flags().Lookup("artifact").Usage,
			example: "artifacts/flow/DailyPrep--identity",
		},
	}
	for name, test := range commands {
		t.Run(name, func(t *testing.T) {
			for _, required := range []string{"logical workspace", "relative", "forward slashes", test.example} {
				if !strings.Contains(test.usage, required) {
					t.Fatalf("artifact help %q does not contain %q", test.usage, required)
				}
			}
		})
	}
}

func TestListProjectFilterHelpDistinguishesNamesPathsAndLUIDs(t *testing.T) {
	tests := map[string]string{
		"workbook":   newWorkbookList(nil, nil).Flags().Lookup("project-name").Usage,
		"datasource": newDatasourceList(datasourceInventoryDependencies{}).Flags().Lookup("project-name").Usage,
		"flow-name":  newFlowList(Dependencies{}).Flags().Lookup("project-name").Usage,
		"flow-luid":  newFlowList(Dependencies{}).Flags().Lookup("project-id").Usage,
	}
	for name, usage := range tests {
		t.Run(name, func(t *testing.T) {
			if strings.Contains(name, "luid") {
				if !strings.Contains(usage, "authoritative project LUID filter") {
					t.Fatalf("project LUID help = %q", usage)
				}
				return
			}
			if !strings.Contains(usage, "exact leaf project name filter; not a project path") {
				t.Fatalf("project-name help = %q", usage)
			}
		})
	}
}

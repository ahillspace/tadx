package output_test

import (
	"bytes"
	"strings"
	"testing"

	groupops "github.com/ahillspace/tadx/actions/admin/group"
	userops "github.com/ahillspace/tadx/actions/admin/user"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	envlist "github.com/ahillspace/tadx/actions/env/profile"
	flowops "github.com/ahillspace/tadx/actions/flow"
	projectops "github.com/ahillspace/tadx/actions/project"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/output"
)

func TestAllInventoryProjectionsHideCursors(t *testing.T) {
	for name, value := range map[string]any{
		"workbooks":        workbookops.ListOutput{Page: workbookops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"datasources":      datasourceops.ListOutput{Page: datasourceops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"flows":            flowops.ListOutput{Page: flowops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"projects":         projectops.ListOutput{Page: projectops.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"users":            userops.ListOutput{Page: userops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"groups":           groupops.ListOutput{Page: groupops.ListOutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"capabilities":     capabilitylist.Output{Page: capabilitylist.Pagination{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"environments":     envlist.ListOutput{Page: envlist.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"workspaces":       workspaceaction.ListOutput{Page: workspaceaction.ListPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"workspace status": workspaceaction.StatusOutput{Inventory: workspaceaction.StatusInventory{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
	} {
		t.Run(name, func(t *testing.T) {
			for _, full := range []bool{false, true} {
				var rendered bytes.Buffer
				if err := output.RenderWithOptions(&rendered, value, output.Options{Full: full}); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(rendered.String(), "next_cursor") || strings.Contains(rendered.String(), "private-cursor") || !strings.Contains(rendered.String(), "more_available: true") {
					t.Fatalf("full=%t output=%s", full, rendered.String())
				}
			}
		})
	}
}

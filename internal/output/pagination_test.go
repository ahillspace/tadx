package output_test

import (
	"bytes"
	"strings"
	"testing"

	grouplist "github.com/ahillspace/tadx/actions/admin/group/list"
	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	capabilitylist "github.com/ahillspace/tadx/actions/capability/list"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	envlist "github.com/ahillspace/tadx/actions/env/profile/list"
	flowlist "github.com/ahillspace/tadx/actions/flow/list"
	projectlist "github.com/ahillspace/tadx/actions/project/list"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	workspacestatus "github.com/ahillspace/tadx/actions/workspace/status"
	"github.com/ahillspace/tadx/internal/output"
)

func TestAllInventoryProjectionsHideCursors(t *testing.T) {
	for name, value := range map[string]any{
		"workbooks":        workbooklist.Output{Page: workbooklist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"datasources":      datasourcelist.Output{Page: datasourcelist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"flows":            flowlist.Output{Page: flowlist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"projects":         projectlist.Output{Page: projectlist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"users":            userlist.Output{Page: userlist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"groups":           grouplist.Output{Page: grouplist.OutputPage{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"capabilities":     capabilitylist.Output{Page: capabilitylist.Pagination{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"environments":     envlist.Output{Page: envlist.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"workspaces":       workspacelist.Output{Page: workspacelist.Page{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
		"workspace status": workspacestatus.Output{Inventory: workspacestatus.Inventory{Returned: 1, Total: 2, Limit: 1, NextCursor: "private-cursor"}},
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

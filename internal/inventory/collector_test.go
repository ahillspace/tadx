package inventory

import (
	"strings"
	"testing"
	"time"

	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestInventoryResourceEntriesSkipsInvalidProjectReferencesAcrossContentScopes(t *testing.T) {
	projectDependency := tableaucache.InventoryTable{
		Scope: tableaucache.ScopeProjects,
		Rows:  [][]any{{"project-ops", "Ops", ""}},
	}
	tests := []struct {
		name  string
		scope tableaucache.Scope
		rows  [][]any
	}{
		{name: "datasources", scope: tableaucache.ScopeDatasources, rows: [][]any{
			{"datasource-valid", "Valid", "project-ops", "", "", `{"luid":"datasource-valid","name":"Valid"}`},
			{"datasource-invalid", "Invalid", "project-missing", "", "", `{"luid":"datasource-invalid","name":"Invalid"}`},
		}},
		{name: "flows", scope: tableaucache.ScopeFlows, rows: [][]any{
			{"flow-valid", "Valid", "project-ops", "", "tflx", "", `{"luid":"flow-valid","name":"Valid"}`},
			{"flow-invalid", "Invalid", "", "", "tflx", "", `{"luid":"flow-invalid","name":"Invalid"}`},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entries, skipped, err := resourceEntries(tableaucache.InventorySnapshot{
				Scope: test.scope, Rows: test.rows, Dependencies: []tableaucache.InventoryTable{projectDependency},
			}, "production", "team-site", time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || skipped != 1 || !strings.HasSuffix(entries[0].LUID, "-valid") {
				t.Fatalf("entries = %#v, skipped = %d", entries, skipped)
			}
		})
	}
}

func TestInventoryKeepsHealthyBranchesWhenProjectHierarchyIsMalformed(t *testing.T) {
	entries, skipped, err := resourceEntries(tableaucache.InventorySnapshot{
		Scope: tableaucache.ScopeWorkbooks,
		Dependencies: []tableaucache.InventoryTable{{Scope: tableaucache.ScopeProjects, Rows: [][]any{
			{"healthy", "Ops", ""}, {"slash", "Ops/Reports", ""}, {"orphan", "Orphan", "missing"},
		}}},
		Rows: [][]any{
			{"good", "Good", "healthy", "", nil, "", `{}`},
			{"good-slash", "Slash Project", "slash", "", nil, "", `{}`},
			{"bad-orphan", "Bad Orphan", "orphan", "", nil, "", `{}`},
		},
	}, "dev", "site", time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC))
	if err != nil || skipped != 1 || len(entries) != 2 || entries[0].LUID != "good" || entries[0].ProjectPath != "Ops" || entries[1].LUID != "good-slash" || entries[1].ProjectPath != "Ops/Reports" {
		t.Fatalf("entries = %#v skipped = %d err = %v", entries, skipped, err)
	}
}

func TestInventoryPreservesSlashProjectDisplayName(t *testing.T) {
	projects, err := projects(tableaucache.InventorySnapshot{Scope: tableaucache.ScopeProjects, Rows: [][]any{{"p", "A/B", ""}}})
	if err != nil || projects["p"].path != "A/B" {
		t.Fatalf("slash project = %#v, error = %v", projects, err)
	}
}

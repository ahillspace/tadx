package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestInventoryBoundaryAllowsOnlyExactRequiredEdges(t *testing.T) {
	for _, tc := range []struct {
		name, file, imported string
	}{
		{"app construction", "internal/app/inventory.go", "internal/inventory"},
		{"inventory cache", "internal/inventory/collector.go", "internal/cache"},
		{"inventory errors", "internal/inventory/collector.go", "internal/errs"},
		{"inventory source", "internal/inventory/collector.go", "internal/readsource"},
		{"inventory native collector", "internal/inventory/collector.go", "internal/tableau/cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/"+tc.imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
	}
}

func TestInventoryBoundaryRejectsUnapprovedDirections(t *testing.T) {
	for _, tc := range []struct {
		name, file, imported, reason string
	}{
		{"inventory action", "internal/inventory/collector.go", "actions/workbook", "foundation packages must not import higher layers"},
		{"inventory app", "internal/inventory/collector.go", "internal/app", "foundation packages must not import higher layers"},
		{"inventory CLI", "internal/inventory/collector.go", "internal/cli/content", "foundation packages must not import higher layers"},
		{"inventory resource", "internal/inventory/collector.go", "internal/resources/project", "foundation packages must not import higher layers"},
		{"inventory other native", "internal/inventory/collector.go", "internal/tableau/project", "foundation packages must not import higher layers"},
		{"inventory config", "internal/inventory/collector.go", "internal/config", "foundation packages must not import unapproved local packages"},
		{"nested inventory", "internal/inventory/nested/collector.go", "internal/cache", "foundation packages must not import unapproved local packages"},
		{"action consumer", "actions/workbook/action.go", "internal/inventory", "actions must not import unapproved local packages"},
		{"resource consumer", "internal/resources/project/adapter.go", "internal/inventory", "resource adapters must not import unapproved local packages"},
		{"CLI consumer", "internal/cli/content/command.go", "internal/inventory", "CLI plumbing must not import unapproved local packages"},
		{"foundation consumer", "internal/cache/store.go", "internal/inventory", "foundation packages must not import unapproved local packages"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := moduleFixture(t)
			imported := "example.test/tadx/" + tc.imported
			writeGo(t, root, tc.file, fmt.Sprintf("package fixture\nimport _ %q\n", imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{tc.file + " imports " + imported + ": " + tc.reason})
		})
	}
}

package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestCheckAllowsMappedAdapterConsumerContracts(t *testing.T) {
	pairs := []struct {
		adapter  string
		consumer string
	}{
		{"workbook", "workbook"},
		{"datasource", "datasource"},
		{"flow", "flow"},
		{"project", "project"},
		{"search", "search"},
		{"lineage", "lineage"},
		{"job", "job"},
		{"contentlabel", "contentlabel"},
		{"admin", "admin/user"},
		{"admin", "admin/group"},
		{"admin", "admin/permission"},
		{"admin", "admin/labelcategory"},
		{"admin", "admin/labelvalue"},
		{"pulse", "pulse/definition"},
		{"pulse", "pulse/metric"},
		{"pulse", "pulse/subscription"},
	}
	for _, pair := range pairs {
		t.Run(pair.adapter+"/"+pair.consumer, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/resources/"+pair.adapter+"/adapter.go", fmt.Sprintf(`package adapter
import consumer "example.test/tadx/actions/%s"
var _ consumer.Reader
`, pair.consumer))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
	}
}

func TestProjectCachePortAllowsOnlyItsRequiredInfrastructure(t *testing.T) {
	for _, imported := range []string{"internal/cache", "internal/readsource"} {
		t.Run(imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/resources/project/cache.go", fmt.Sprintf("package project\nimport _ %q\n", "example.test/tadx/"+imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
	}
	for _, tc := range []struct {
		name     string
		file     string
		imported string
	}{
		{"other resource cache", "internal/resources/lineage/cache.go", "internal/cache"},
		{"other resource read coverage", "internal/resources/lineage/cache.go", "internal/readsource"},
		{"nested project package", "internal/resources/project/nested/cache.go", "internal/cache"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, fmt.Sprintf("package adapter\nimport _ %q\n", "example.test/tadx/"+tc.imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{
				tc.file + " imports example.test/tadx/" + tc.imported + ": resource adapters must not import unapproved local packages",
			})
		})
	}
}

func TestAdminCachePortsAllowOnlyTheirRequiredInfrastructure(t *testing.T) {
	for _, imported := range []string{"internal/cache", "internal/readsource", "internal/inventory", "internal/errs"} {
		t.Run(imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/resources/admin/cache.go", fmt.Sprintf("package admin\nimport _ %q\n", "example.test/tadx/"+imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
	}
	for _, tc := range []struct {
		name, file, imported string
	}{
		{"other resource inventory", "internal/resources/lineage/cache.go", "internal/inventory"},
		{"other resource admin errors", "internal/resources/workbook/cache.go", "internal/errs"},
		{"nested admin package", "internal/resources/admin/nested/cache.go", "internal/cache"},
		{"sibling action inventory", "actions/admin/group/action.go", "internal/inventory"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/"+tc.imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 || violations[0].File != tc.file {
				t.Fatalf("violations=%v", violations)
			}
		})
	}
}

func TestLineageArtifactPortAllowsOnlyExactOwner(t *testing.T) {
	for _, imported := range []string{"internal/artifact", "internal/errs"} {
		t.Run("allowed/"+imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/resources/lineage/ports.go", fmt.Sprintf("package lineage\nimport _ %q\n", "example.test/tadx/"+imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
		for _, file := range []string{"internal/resources/workbook/ports.go", "internal/resources/lineage/nested/ports.go"} {
			t.Run("rejected/"+file+"/"+imported, func(t *testing.T) {
				root := moduleFixture(t)
				writeGo(t, root, file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/"+imported))
				violations, err := architecture.Check(root)
				if err != nil {
					t.Fatal(err)
				}
				if len(violations) != 1 || violations[0].File != file {
					t.Fatalf("violations=%v", violations)
				}
			})
		}
	}
}

func TestContentReadPortsAllowOnlyExactInfrastructureEdges(t *testing.T) {
	for _, resource := range []string{"workbook", "datasource", "flow"} {
		for _, imported := range []string{"internal/cache", "internal/readsource", "internal/inventory"} {
			t.Run(resource+"/"+imported, func(t *testing.T) {
				root := moduleFixture(t)
				file := "internal/resources/" + resource + "/read_ports.go"
				writeGo(t, root, file, fmt.Sprintf("package adapter\nimport _ %q\n", "example.test/tadx/"+imported))
				violations, err := architecture.Check(root)
				if err != nil {
					t.Fatal(err)
				}
				assertViolationStrings(t, violations, nil)
			})
		}
	}
	for _, tc := range []struct{ file, imported string }{
		{"internal/resources/lineage/read_ports.go", "internal/inventory"},
		{"internal/resources/pulse/read_ports.go", "internal/cache"},
		{"internal/resources/workbook/nested/read_ports.go", "internal/cache"},
		{"internal/resources/datasource/nested/read_ports.go", "internal/readsource"},
		{"internal/resources/flow/nested/read_ports.go", "internal/inventory"},
		{"internal/resources/workbook/read_ports.go", "internal/operationrun"},
	} {
		t.Run(tc.file+"/"+tc.imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, tc.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/"+tc.imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 || violations[0].File != tc.file {
				t.Fatalf("violations=%v", violations)
			}
		})
	}
}

func TestCheckRejectsUnmappedAdapterActionImports(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		consumer string
	}{
		{"different resource", "internal/resources/workbook/adapter.go", "datasource"},
		{"different domain", "internal/resources/admin/adapter.go", "pulse/metric"},
		{"unlisted admin consumer", "internal/resources/admin/adapter.go", "admin/audit"},
		{"unlisted pulse consumer", "internal/resources/pulse/adapter.go", "pulse/insight"},
		{"obsolete project verb", "internal/resources/project/adapter.go", "project/move"},
		{"obsolete permission verb", "internal/resources/admin/adapter.go", "admin/permission/inspect"},
		{"obsolete subscription verb", "internal/resources/pulse/adapter.go", "pulse/subscription/list"},
		{"action prefix lookalike", "internal/resources/workbook/adapter.go", "workbookextra"},
		{"adapter prefix lookalike", "internal/resources/workbookextra/adapter.go", "workbook"},
		{"nested adapter package", "internal/resources/workbook/nested/adapter.go", "workbook"},
		{"unmapped adapter", "internal/resources/unknown/adapter.go", "workbook"},
		{"removed forwarding adapter", "internal/resources/catalog/adapter.go", "catalog"},
		{"action namespace", "internal/resources/admin/adapter.go", "admin"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := moduleFixture(t)
			imported := "example.test/tadx/actions/" + test.consumer
			writeGo(t, root, test.file, fmt.Sprintf("package adapter\nimport consumer %q\nvar _ consumer.Reader\n", imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{
				test.file + " imports " + imported + ": resource adapters must import only approved consumer action contracts",
			})
		})
	}
}

func TestCheckAdapterContractAllowancePreservesForbiddenDirections(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		imported string
		reason   string
	}{
		{"action to adapter", "actions/workbook/action.go", "example.test/tadx/internal/resources/workbook", "actions must not import resource adapters"},
		{"action to native client", "actions/workbook/action.go", "example.test/tadx/internal/tableau/workbook", "actions must not import Tableau clients"},
		{"action to action", "actions/workbook/action.go", "example.test/tadx/actions/datasource", "actions must not import another action package"},
		{"native client to action", "internal/tableau/workbook/client.go", "example.test/tadx/actions/workbook", "Tableau clients must not import action packages"},
		{"native client to adapter", "internal/tableau/workbook/client.go", "example.test/tadx/internal/resources/workbook", "Tableau clients must not import resource adapters"},
		{"adapter to app", "internal/resources/workbook/adapter.go", "example.test/tadx/internal/app", "resource adapters must not import the composition root"},
		{"adapter to cli", "internal/resources/workbook/adapter.go", "example.test/tadx/internal/cli/content", "resource adapters must not import CLI packages"},
		{"adapter to auth", "internal/resources/workbook/adapter.go", "example.test/tadx/internal/auth", "resource adapters must not import authentication logic"},
		{"adapter to http", "internal/resources/workbook/adapter.go", "net/http", "resource adapters must not use net/http directly"},
		{"adapter to cobra", "internal/resources/workbook/adapter.go", "github.com/spf13/cobra", "resource adapters must not import Cobra"},
		{"adapter to unmapped mechanism", "internal/resources/workbook/adapter.go", "example.test/tadx/internal/operationrun", "resource adapters must not import unapproved local packages"},
		{"cli to adapter", "internal/cli/content/command.go", "example.test/tadx/internal/resources/workbook", "CLI plumbing must not import resource adapters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, test.file, fmt.Sprintf("package fixture\nimport _ %q\n", test.imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{test.file + " imports " + test.imported + ": " + test.reason})
		})
	}
}

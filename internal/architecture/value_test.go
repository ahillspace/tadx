package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestSharedValuesPermitOnlyStandardLibraryImports(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/value/values.go", `package value
import (
	_ "time"
	_ "example.test/tadx/internal/config"
	_ "example.test/tadx/internal/value/helpers"
	_ "github.com/example/models"
)
`)
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, []string{
		"internal/value/values.go imports example.test/tadx/internal/config: shared value types must depend only on the standard library",
		"internal/value/values.go imports example.test/tadx/internal/value/helpers: shared value types must depend only on the standard library",
		"internal/value/values.go imports github.com/example/models: shared value types must depend only on the standard library",
	})
}

func TestSharedValuesAreAllowedAcrossBehaviorLayers(t *testing.T) {
	root := moduleFixture(t)
	for _, file := range []string{"actions/workbook/workbook.go", "internal/app/mapping.go", "internal/resources/lineage/adapter.go", "internal/tableau/fieldcatalog/types.go"} {
		writeGo(t, root, file, `package fixture
import _ "example.test/tadx/internal/value"
`)
	}
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	assertViolationStrings(t, violations, nil)
}

func TestPagingImportsOnlySharedValues(t *testing.T) {
	for _, dependency := range []string{
		"internal/value", "internal/value/helpers", "internal/config", "internal/output",
		"actions/catalog/database/list", "internal/app", "internal/cli/catalog",
		"internal/resources/catalog", "internal/tableau/metadataassets",
	} {
		t.Run(dependency, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/paging/metadata.go", "package paging\nimport _ \"example.test/tadx/"+dependency+"\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			allowed := dependency == "internal/value"
			if (len(violations) == 0) != allowed {
				t.Fatalf("dependency %s: allowed=%t, violations=%v", dependency, allowed, violations)
			}
		})
	}
}

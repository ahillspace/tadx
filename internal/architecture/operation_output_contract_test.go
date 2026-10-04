package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestOperationOutputNeutralRecordEdgesAreExact(t *testing.T) {
	for _, test := range []struct {
		file    string
		allowed bool
	}{
		{"internal/output/operation.go", true},
		{"internal/operationrun/projection.go", true},
		{"internal/output/nested/operation.go", false},
		{"internal/operationrun/nested/projection.go", false},
		{"internal/config/projection.go", false},
	} {
		t.Run(test.file, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, test.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/internal/value"))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if test.allowed {
				assertViolationStrings(t, violations, nil)
			} else if len(violations) != 1 || violations[0].File != test.file {
				t.Fatalf("violations=%v", violations)
			}
		})
	}
}

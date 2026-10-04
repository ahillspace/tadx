package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestOperationCoordinatorErrorEdgeIsExact(t *testing.T) {
	for _, test := range []struct {
		file    string
		allowed bool
	}{
		{"internal/operationrun/coordinator.go", true},
		{"internal/operationrun/nested/coordinator.go", false},
		{"internal/config/coordinator.go", false},
	} {
		t.Run(test.file, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, test.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/internal/errs"))
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

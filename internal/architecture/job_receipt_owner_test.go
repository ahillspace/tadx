package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestJobReceiptErrorsStayAtCoordinatorBoundary(t *testing.T) {
	root := moduleFixture(t)
	for _, file := range []string{"internal/jobmonitor/publication.go", "internal/jobmonitor/nested/publication.go", "internal/operationrun/nested/record.go"} {
		writeGo(t, root, file, "package fixture\nimport _ \"example.test/tadx/internal/errs\"\n")
	}
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations=%v", violations)
	}
	for _, violation := range violations {
		if violation.File == "internal/jobmonitor/publication.go" {
			t.Fatalf("owner rejected: %v", violation)
		}
	}
}

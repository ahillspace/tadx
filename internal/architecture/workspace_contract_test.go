package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestWorkspaceMechanismEdgesAreExact(t *testing.T) {
	for _, imported := range []string{"internal/artifact", "internal/workspace"} {
		t.Run("allowed/"+imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "actions/workspace/ports.go", fmt.Sprintf("package workspace\nimport _ %q\n", "example.test/tadx/"+imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
		for _, file := range []string{"actions/workbook/ports.go", "actions/workspace/nested/ports.go"} {
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

func TestArtifactAmbiguityEdgesAreExact(t *testing.T) {
	for _, imported := range []string{"internal/commandhint", "internal/errs"} {
		t.Run("allowed/"+imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/artifact/ambiguity.go", fmt.Sprintf("package artifact\nimport _ %q\n", "example.test/tadx/"+imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
		for _, file := range []string{"internal/config/ambiguity.go", "internal/artifact/nested/ambiguity.go"} {
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

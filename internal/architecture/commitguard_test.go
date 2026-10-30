package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestCommitAdmissionImportBoundary(t *testing.T) {
	for _, consumer := range []string{
		"internal/app",
		"internal/artifact",
		"internal/tableau/workbook",
		"internal/tableau/datasource",
		"internal/tableau/flow",
	} {
		t.Run(consumer, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/commitguard/admission.go", "package commitguard\nimport _ \"context\"\n")
			writeGo(t, root, consumer+"/consumer.go", "package consumer\nimport _ \"example.test/tadx/internal/commitguard\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, nil)
		})
		t.Run(consumer+"/subpackage", func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, consumer+"/consumer.go", "package consumer\nimport _ \"example.test/tadx/internal/commitguard/unapproved\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 {
				t.Fatalf("subpackage import bypassed the boundary: %v", violations)
			}
		})
	}
	for _, consumer := range []string{"actions/workbook", "internal/cli/job", "internal/resources/workbook", "internal/tableau/cache", "internal/workspace"} {
		t.Run(consumer+"/unapproved", func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, consumer+"/consumer.go", "package consumer\nimport _ \"example.test/tadx/internal/commitguard\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 {
				t.Fatalf("unapproved consumer bypassed the boundary: %v", violations)
			}
		})
	}
}

func TestCommitAdmissionUsesOnlyStandardLibrary(t *testing.T) {
	for _, imported := range []string{
		"example.test/tadx/internal/app",
		"example.test/tadx/internal/operationrun",
		"example.test/tadx/internal/artifact",
		"example.test/tadx/internal/tableau",
		"github.com/spf13/cobra",
	} {
		t.Run(imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, "internal/commitguard/admission.go", fmt.Sprintf("package commitguard\nimport _ %q\n", imported))
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			assertViolationStrings(t, violations, []string{
				"internal/commitguard/admission.go imports " + imported + ": commit admission must depend only on the standard library",
			})
		})
	}
}

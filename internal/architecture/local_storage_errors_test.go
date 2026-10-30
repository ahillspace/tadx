package architecture_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestLocalStorageErrorContractsKeepFoundationBoundaries(t *testing.T) {
	for _, test := range []struct {
		file, imported, rejection string
	}{
		{"internal/artifact/error_paths.go", "internal/errs", ""},
		{"internal/workspace/download.go", "internal/errs", ""},
		{"internal/artifact/workbook.go", "internal/app", "foundation packages must not import higher layers"},
		{"internal/artifact/workbook.go", "internal/cli", "foundation packages must not import higher layers"},
		{"internal/workspace/download.go", "internal/app", "foundation packages must not import higher layers"},
		{"internal/workspace/download.go", "internal/cli", "foundation packages must not import higher layers"},
		{"internal/errs/workspace_paths.go", "internal/workspace", "foundation packages must not import unapproved local packages"},
		{"internal/errs/workspace_paths.go", "internal/artifact", "foundation packages must not import unapproved local packages"},
		{"internal/errs/workspace_paths.go", "internal/config", "foundation packages must not import unapproved local packages"},
		{"internal/artifact/error_paths.go", "internal/errs/extra", "foundation packages must not import unapproved local packages"},
		{"internal/workspace/download.go", "internal/errs/extra", "foundation packages must not import unapproved local packages"},
		{"internal/artifact/workbook.go", "github.com/spf13/cobra", "foundation packages must not import Cobra"},
		{"internal/workspace/download.go", "github.com/spf13/cobra", "foundation packages must not import Cobra"},
		{"internal/errs/workspace_paths.go", "github.com/spf13/cobra", "foundation packages must not import Cobra"},
	} {
		t.Run(test.file+"_"+test.imported, func(t *testing.T) {
			root := moduleFixture(t)
			imported := test.imported
			if strings.HasPrefix(imported, "internal/") {
				imported = "example.test/tadx/" + imported
			}
			writeGo(t, root, test.file, "package example\nimport _ \""+imported+"\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if test.rejection == "" {
				if len(violations) != 0 {
					t.Fatalf("required error-contract dependency rejected: %v", violations)
				}
				return
			}
			assertReasons(t, violations, []string{test.rejection})
		})
	}
}

package architecture_test

import (
	"github.com/ahillspace/tadx/internal/architecture"
	"testing"
)

func TestCommandHintsAreLeafDependencies(t *testing.T) {
	root := moduleFixture(t)
	writeGo(t, root, "internal/commandhint/command.go", `package commandhint
import (
 _ "runtime"
 _ "strings"
 _ "example.test/tadx/internal/config"
 _ "github.com/example/shell"
)
`)
	for _, file := range []string{"actions/workbook/move.go", "internal/app/hints.go", "internal/cli/root.go"} {
		writeGo(t, root, file, "package fixture\nimport _ \"example.test/tadx/internal/commandhint\"\n")
	}
	violations, err := architecture.Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 2 {
		t.Fatalf("violations=%v", violations)
	}
	for _, violation := range violations {
		if violation.File != "internal/commandhint/command.go" {
			t.Fatal(violation)
		}
	}
}

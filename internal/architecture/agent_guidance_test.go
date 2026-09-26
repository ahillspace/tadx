package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestAgentGuidanceImportBoundaries(t *testing.T) {
	for _, test := range []struct {
		file, dependency string
		allowed          bool
	}{
		{"actions/agent/install/action.go", "internal/agenttarget", true},
		{"actions/agent/uninstall/action.go", "internal/agenttarget", true},
		{"internal/cli/agent/command.go", "internal/agenttarget", true},
		{"internal/agent/install.go", "internal/agenttarget", true},
		{"internal/guidancenotice/notice.go", "internal/agenttarget", true},
		{"cmd/tadx/main.go", "internal/guidancenotice", true},
		{"actions/workbook/pull.go", "internal/agenttarget", false},
		{"actions/agent/install/action.go", "internal/agent", false},
		{"internal/cli/content/command.go", "internal/agenttarget", false},
		{"internal/guidancenotice/notice.go", "internal/auth", false},
		{"internal/guidancenotice/notice.go", "internal/app", false},
		{"internal/agenttarget/targets.go", "internal/config", false},
		{"internal/app/update.go", "internal/update", true},
		{"internal/update/runtime.go", "actions/update", true},
		{"internal/update/runtime.go", "internal/version", true},
		{"internal/update/runtime.go", "internal/agenttarget", true},
		{"internal/update/runtime.go", "scripts", true},
		{"internal/update/runtime.go", "internal/app", false},
		{"internal/update/runtime.go", "internal/auth", false},
		{"internal/update/runtime.go", "actions/workbook", false},
		{"actions/update/action.go", "internal/update", false},
		{"internal/cli/update/command.go", "internal/update", false},
	} {
		t.Run(test.file+"_"+test.dependency, func(t *testing.T) {
			root := t.TempDir()
			writeGo(t, root, "go.mod", "module example.test/tadx\n\ngo 1.26\n")
			writeGo(t, root, test.file, "package example\nimport _ \"example.test/tadx/"+test.dependency+"\"\n")
			violations, err := architecture.Check(root)
			if err != nil {
				t.Fatal(err)
			}
			if (len(violations) == 0) != test.allowed {
				t.Fatalf("allowed=%v, violations=%v", test.allowed, violations)
			}
		})
	}
}

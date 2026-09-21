package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestManagedPolicyKeepsExplicitFoundationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		file, imported string
		allowed        bool
	}{
		{"internal/app/managed_policy.go", "internal/managedpolicy", true},
		{"internal/managedpolicy/policy.go", "internal/capability", true},
		{"internal/managedpolicy/policy.go", "internal/value", true},
		{"actions/policy/status/action.go", "internal/value", true},
		{"actions/policy/status/action.go", "internal/managedpolicy", false},
		{"actions/auth/check/action.go", "internal/auth", false},
		{"internal/managedpolicy/policy.go", "internal/auth", false},
		{"internal/managedpolicy/policy.go", "internal/config", false},
		{"internal/managedpolicy/policy.go", "internal/app", false},
		{"internal/managedpolicy/policy.go", "internal/cli", false},
		{"internal/managedpolicy/policy.go", "internal/tableau", false},
		{"internal/managedpolicy/policy.go", "actions/policy/status", false},
		{"internal/managedpolicy/policy.go", "internal/unregistered", false},
		{"internal/unregistered/example.go", "internal/managedpolicy", false},
		{"internal/managedpolicyextra/example.go", "internal/capability", false},
		{"internal/value/policy.go", "internal/managedpolicy", false},
	} {
		t.Run(tc.file+"_"+tc.imported, func(t *testing.T) {
			root := t.TempDir()
			writeGo(t, root, "go.mod", "module example.test/tadx\n\ngo 1.26\n")
			writeGo(t, root, tc.file, "package example\nimport _ \"example.test/tadx/"+tc.imported+"\"\n")
			got, err := architecture.Check(root)
			if err != nil || (len(got) == 0) != tc.allowed {
				t.Fatalf("allowed=%v violations=%v err=%v", tc.allowed, got, err)
			}
		})
	}
}

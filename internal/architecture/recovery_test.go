package architecture_test

import (
	"github.com/ahillspace/tadx/internal/architecture"
	"testing"
)

func TestRecoveryHelpersKeepFoundationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		file, imported string
		allowed        bool
	}{
		{"internal/output/output.go", "internal/commandhint", true},
		{"internal/workspace/recovery.go", "internal/commandhint", true},
		{"internal/commandhint/context.go", "internal/config", false},
		{"internal/commandhint/context.go", "internal/app", false},
		{"internal/output/output.go", "internal/cli", false},
		{"internal/output/output.go", "internal/auth", false},
		{"internal/workspace/recovery.go", "internal/tableau", false},
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

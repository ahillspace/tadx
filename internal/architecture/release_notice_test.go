package architecture_test

import (
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestReleaseNoticeImportBoundary(t *testing.T) {
	for _, test := range []struct {
		file, dependency string
		allowed          bool
	}{
		{"cmd/tadx/main.go", "internal/releasenotice", true},
		{"internal/releasenotice/notice.go", "internal/version", true},
		{"internal/releasenotice/notice.go", "internal/auth", false},
		{"internal/releasenotice/notice.go", "internal/app", false},
		{"internal/releasenotice/notice.go", "internal/cli/progress", false},
		{"internal/cli/root.go", "internal/releasenotice", false},
		{"cmd/tadx/main.go", "internal/version", false},
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

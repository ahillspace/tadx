package architecture_test

import (
	"fmt"
	"testing"

	"github.com/ahillspace/tadx/internal/architecture"
)

func TestSearchCachePortDependencyIsExact(t *testing.T) {
	for _, test := range []struct {
		file     string
		imported string
		allowed  bool
	}{
		{"internal/resources/search/cache.go", "internal/cache", true},
		{"internal/resources/search/nested/cache.go", "internal/cache", false},
		{"internal/resources/lineage/cache.go", "internal/cache", false},
		{"internal/resources/search/cache.go", "internal/config", false},
		{"actions/search/cache.go", "internal/cache", false},
	} {
		t.Run(test.file+"/"+test.imported, func(t *testing.T) {
			root := moduleFixture(t)
			writeGo(t, root, test.file, fmt.Sprintf("package fixture\nimport _ %q\n", "example.test/tadx/"+test.imported))
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

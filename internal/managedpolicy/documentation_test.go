package managedpolicy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/capability"
)

func TestDocumentedPolicyJSONUsesCanonicalCapabilities(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "managed-policy.md")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var example strings.Builder
	inside, examples := false, 0
	for line := range strings.SplitSeq(string(document), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if !inside && line == "```json" {
			inside = true
			example.Reset()
			continue
		}
		if !inside {
			continue
		}
		if line == "```" {
			examples++
			if _, err := Parse([]byte(example.String()), capability.All()); err != nil {
				t.Errorf("%s JSON example %d: %v", path, examples, err)
			}
			inside = false
			continue
		}
		example.WriteString(line)
		example.WriteByte('\n')
	}
	if inside {
		t.Fatal("unclosed JSON example fence")
	}
	if examples == 0 {
		t.Fatal("managed policy documentation must include a JSON example")
	}
}

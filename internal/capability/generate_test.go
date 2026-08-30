package capability

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedCapabilityReferenceIsClean(t *testing.T) {
	generated, err := GenerateMarkdown(All())
	if err != nil {
		t.Fatalf("GenerateMarkdown returned error: %v", err)
	}
	path := filepath.Join("..", "..", "docs", "reference", "capabilities.md")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated reference: %v", err)
	}
	if string(generated) != string(committed) {
		t.Fatalf("%s is stale; run go run ./cmd/gencapdocs -out %s", path, path)
	}

	again, err := GenerateMarkdown(All())
	if err != nil {
		t.Fatalf("second GenerateMarkdown returned error: %v", err)
	}
	if string(generated) != string(again) {
		t.Fatal("GenerateMarkdown is not deterministic")
	}
}

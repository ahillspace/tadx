package capability

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestGeneratedCapabilityMapDataMatchesTypedRegistry(t *testing.T) {
	data, err := GenerateJSON(All())
	if err != nil {
		t.Fatal(err)
	}
	committed, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "capabilities.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, committed) {
		t.Fatal("capability map data is stale; run go generate ./internal/capability")
	}
	var definitions []Definition
	if err := json.Unmarshal(data, &definitions); err != nil {
		t.Fatal(err)
	}
	if err := Validate(definitions); err != nil {
		t.Fatal(err)
	}
	if len(definitions) != len(All()) {
		t.Fatal("generated data lost capabilities")
	}
	for i, expected := range All() {
		left, _ := json.Marshal(expected)
		right, _ := json.Marshal(definitions[i])
		if !bytes.Equal(left, right) {
			t.Fatalf("metadata changed for %s", expected.ID)
		}
	}
}

func TestCapabilityMapGenerationIsDeterministicAndValidated(t *testing.T) {
	definitions := All()
	first, err := GenerateJSON(definitions)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(definitions)-1; i < j; i, j = i+1, j-1 {
		definitions[i], definitions[j] = definitions[j], definitions[i]
	}
	second, err := GenerateJSON(definitions)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("capability data must be independent of declaration order")
	}
	definitions[0].ID = ""
	if _, err := GenerateJSON(definitions); err == nil {
		t.Fatal("invalid typed definitions must not generate documentation data")
	}
}

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

// TestCapabilityProseIsPlainTextAndEscapedInMarkdown keeps registry prose
// readable in terminal help while markdown viewers still show placeholders
// such as <name> instead of treating them as HTML tags.
func TestCapabilityProseIsPlainTextAndEscapedInMarkdown(t *testing.T) {
	for _, definition := range All() {
		prose := strings.Join([]string{definition.Outcome, definition.Surface, definition.Selectors, definition.Availability, definition.SafetyGuard, definition.ArtifactEffect, definition.Upstream, definition.Evidence, definition.Validation}, " ")
		if strings.Contains(prose, "&lt;") || strings.Contains(prose, "&gt;") {
			t.Errorf("%s stores HTML entities; write plain text", definition.ID)
		}
	}
	generated, err := GenerateMarkdown(All())
	if err != nil {
		t.Fatal(err)
	}
	for number, line := range strings.Split(string(generated), "\n")[1:] {
		prose := regexp.MustCompile("`[^`]*`").ReplaceAllString(line, "")
		if strings.ContainsAny(prose, "<>") {
			t.Errorf("line %d has unescaped angle brackets outside code: %s", number+2, line)
		}
	}
}

package capability

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestCapabilityMapMetadataMatchesRegistry(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "docs", "reference", "capability-map.html"))
	if err != nil {
		t.Fatal(err)
	}
	const marker = `<script id="capability-data" type="application/json">`
	if strings.Count(string(document), marker) != 1 {
		t.Fatal("capability map must contain one embedded registry snapshot")
	}
	_, script, _ := strings.Cut(string(document), marker)
	data, _, closed := strings.Cut(script, "</script>")
	if !closed {
		t.Fatal("embedded registry snapshot has no closing script tag")
	}
	var definitions []Definition
	if err := json.Unmarshal([]byte(data), &definitions); err != nil {
		t.Fatalf("decode embedded registry snapshot: %v", err)
	}
	byID := make(map[string]Definition, len(definitions))
	for _, definition := range definitions {
		if _, duplicate := byID[definition.ID]; duplicate {
			t.Fatalf("duplicate embedded capability %q", definition.ID)
		}
		byID[definition.ID] = definition
	}
	expectedDefinitions := All()
	if len(definitions) != len(expectedDefinitions) {
		t.Errorf("map has %d capabilities; registry has %d; run go generate ./internal/capability", len(definitions), len(expectedDefinitions))
	}
	for _, expected := range expectedDefinitions {
		id := expected.ID
		t.Run(id, func(t *testing.T) {
			actual, exists := byID[id]
			if !exists {
				t.Fatalf("capability %q is missing from the map", id)
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Errorf("embedded capability metadata differs from the registry:\n got: %+v\nwant: %+v", actual, expected)
			}
		})
	}
}

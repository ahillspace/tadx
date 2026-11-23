package app

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/toon"
)

func runStructure(t *testing.T, options Options, args ...string) (int, map[string]any, string) {
	t.Helper()
	code, out := runPreviewCommand(t, append([]string{"content"}, args...), options)
	decoded, err := toon.Decode([]byte(out))
	if err != nil {
		t.Fatalf("%v output is not TOON: %v\n%s", args, err, out)
	}
	document, _ := decoded.(map[string]any)
	return code, document, out
}

func structureField(t *testing.T, document map[string]any, path ...string) any {
	t.Helper()
	var current any = document
	for _, key := range path {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%v: %v is not an object in %#v", path, key, document)
		}
		current = object[key]
	}
	return current
}

// An agent asks the schema what an element holds before writing it, with no
// package, credential, or network involved.
func TestStructureDescribesWhatAnElementHolds(t *testing.T) {
	options := overviewOptions(t, t.TempDir())
	options.managedPolicy = indirectTestPolicy{}
	code, document, out := runStructure(t, options, "workbook", "structure", "worksheet > table")
	if code != 0 || structureField(t, document, "schema") != "2026.3" || !strings.HasSuffix(structureField(t, document, "details").(string), "--full") {
		t.Fatalf("workbook structure: code=%d\n%s", code, out)
	}
	elements, _ := structureField(t, document, "elements").([]any)
	if len(elements) != 1 {
		t.Fatalf("elements = %#v\n%s", elements, out)
	}
	table := elements[0].(map[string]any)
	if children, _ := table["children"].(string); !strings.HasPrefix(children, "(view, style, panes, mark-layout?, rows, cols,") {
		t.Fatalf("table children = %q\n%s", children, out)
	}
	if paths, _ := table["paths"].([]any); len(paths) != 1 || paths[0] != "workbook > worksheets > worksheet > table" {
		t.Fatalf("table paths = %#v\n%s", table["paths"], out)
	}

	code, document, out = runStructure(t, options, "datasource", "structure", "aliases", "--attribute", "enabled", "--schema-version", "2025.1", "--full")
	if code != 0 || structureField(t, document, "schema") != "2025.1" {
		t.Fatalf("datasource structure: code=%d\n%s", code, out)
	}
	elements, _ = structureField(t, document, "elements").([]any)
	if len(elements) != 1 {
		t.Fatalf("elements = %#v\n%s", elements, out)
	}
	attributes, _ := elements[0].(map[string]any)["attributes"].([]any)
	if len(attributes) != 1 || attributes[0].(map[string]any)["name"] != "enabled" || attributes[0].(map[string]any)["type"] != "one of: yes, no" {
		t.Fatalf("aliases attributes = %#v\n%s", attributes, out)
	}

	code, document, out = runStructure(t, options, "datasource", "structure", "worksheet")
	if code != 2 || structureField(t, document, "error", "id") != "datasource.structure.not_found" || structureField(t, document, "error", "summary") != "a Tableau 2026.3 datasource has no element <worksheet>" {
		t.Fatalf("unknown element: code=%d\n%s", code, out)
	}
	code, document, out = runStructure(t, options, "workbook", "structure", "table", "--schema-version", "2017.1")
	if code != 2 || structureField(t, document, "error", "id") != "workbook.structure.usage" || !strings.Contains(structureField(t, document, "error", "summary").(string), "no Tableau schema for \"2017.1\"; available: 2018.1,") {
		t.Fatalf("unknown version: code=%d\n%s", code, out)
	}
}

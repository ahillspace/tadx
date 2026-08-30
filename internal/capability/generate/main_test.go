package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedRegistryIsClean(t *testing.T) {
	contract := filepath.Join("..", "..", "..", "tadx-v1-capability-contract-final.md")
	rows, err := readRows(contract)
	if err != nil {
		t.Fatalf("readRows returned error: %v", err)
	}
	if got, want := len(rows), 71; got != want {
		t.Fatalf("readRows returned %d capability rows, want %d", got, want)
	}
	generated, err := render(rows)
	if err != nil {
		t.Fatalf("render returned error: %v", err)
	}
	path := filepath.Join("..", "registry_gen.go")
	committed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated registry: %v", err)
	}
	if string(generated) != string(committed) {
		t.Fatalf("%s is stale; run go generate ./internal/capability", path)
	}
}

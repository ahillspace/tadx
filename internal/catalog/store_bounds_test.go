package catalog

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileStoreRejectsOversizedGenerationFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog", "production.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(path, maxGenerationBytes+1); err != nil {
		t.Fatal(err)
	}

	store := NewFileStore(root, time.Now)
	_, err := store.Search(context.Background(), Query{Environment: "production", Site: "marketing", SiteSelected: true})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestFileStoreRejectsExcessiveRecordCount(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog", "production.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	writer := bufio.NewWriter(file)
	if _, err := writer.WriteString(`{"id":"generation-1","environment":"production","site":"marketing","generated_at":"2026-08-30T00:00:00Z","complete":true,"records":[`); err != nil {
		t.Fatal(err)
	}
	for index := 0; index <= maxGenerationRecords; index++ {
		if index > 0 {
			if err := writer.WriteByte(','); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := fmt.Fprintf(writer, `{"luid":"wb-%d","kind":"workbook","name":"Finance"}`, index); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.WriteString(`]}`); err != nil {
		t.Fatal(err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	store := NewFileStore(root, time.Now)
	_, err = store.Search(context.Background(), Query{Environment: "production", Site: "marketing", SiteSelected: true})
	if err == nil || !strings.Contains(err.Error(), "record limit") {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestFileStoreRejectsOversizedRecordField(t *testing.T) {
	root := t.TempDir()
	generation := Generation{
		ID: "generation-1", Environment: "production", Site: "marketing",
		GeneratedAt: time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC), Complete: true,
		Records: []Record{{LUID: "wb-1", Kind: "workbook", Name: strings.Repeat("n", maxFieldBytes+1)}},
	}
	data, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "catalog", "production.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	store := NewFileStore(root, time.Now)
	_, err = store.Search(context.Background(), Query{Environment: "production", Site: "marketing", SiteSelected: true})
	if err == nil || !strings.Contains(err.Error(), "byte limit") {
		t.Fatalf("Search() error = %v", err)
	}
}

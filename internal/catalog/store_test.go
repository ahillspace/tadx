package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/catalog"
)

func TestFileStoreSearchIsBoundedAndReportsGenerationStaleness(t *testing.T) {
	root := t.TempDir()
	generated := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	generation := catalog.Generation{
		ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: generated, Complete: true,
		Records: []catalog.Record{
			{LUID: "wb-1", Kind: "workbook", Name: "Finance North", ProjectPath: "Ops", Owner: "alice"},
			{LUID: "wb-2", Kind: "workbook", Name: "Finance South", ProjectPath: "Ops", Owner: "bob"},
			{LUID: "ds-1", Kind: "datasource", Name: "Finance Source", ProjectPath: "Data", Owner: "alice"},
		},
	}
	data, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "production.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	store := catalog.NewFileStore(root, func() time.Time { return generated.Add(13 * time.Hour) })
	result, err := store.Search(context.Background(), catalog.Query{Environment: "production", Text: "Finance", Kind: "workbook", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Page.Returned != 1 || result.Page.Total != 2 || result.Page.NextCursor != "1" {
		t.Fatalf("page = %#v", result.Page)
	}
	if !result.Stale || result.GenerationID != "generation-1" || result.Environment != "production" || result.Site != "marketing" || len(result.Records) != 1 || result.Records[0].LUID != "wb-1" {
		t.Fatalf("result = %#v", result)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestFileStoreRejectsIncompleteGeneration(t *testing.T) {
	root := t.TempDir()
	data, _ := json.Marshal(catalog.Generation{ID: "generation-bad", Environment: "production", Complete: false})
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", "production.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := catalog.NewFileStore(root, time.Now)
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production"}); err == nil {
		t.Fatal("Search() error = nil")
	}
}

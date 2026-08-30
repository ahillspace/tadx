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
	result, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true, Text: "Finance", Kind: "workbook", Limit: 1})
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
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", SiteSelected: true}); err == nil {
		t.Fatal("Search() error = nil")
	}
}

func TestFileStoreRejectsDefaultSiteSourceMismatch(t *testing.T) {
	root := t.TempDir()
	writeGeneration(t, root, "production.json", catalog.Generation{
		ID: "generation-1", Environment: "production", Site: "other-site", GeneratedAt: time.Now(), Complete: true,
	})
	store := catalog.NewFileStore(root, time.Now)
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "", SiteSelected: true}); err == nil {
		t.Fatal("Search() error = nil")
	}
}

func TestFileStoreAcceptsMatchingDefaultSite(t *testing.T) {
	root := t.TempDir()
	generated := time.Now()
	writeGeneration(t, root, "production.json", catalog.Generation{
		ID: "generation-1", Environment: "production", Site: "", GeneratedAt: generated, Complete: true,
	})
	store := catalog.NewFileStore(root, func() time.Time { return generated })
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "", SiteSelected: true}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
}

func TestFileStoreRequiresResolvedSourceSite(t *testing.T) {
	root := t.TempDir()
	writeGeneration(t, root, "production.json", catalog.Generation{
		ID: "generation-1", Environment: "production", Site: "", GeneratedAt: time.Now(), Complete: true,
	})
	store := catalog.NewFileStore(root, time.Now)
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production"}); err == nil {
		t.Fatal("Search() error = nil")
	}
}

func TestFileStoreRejectsMissingGenerationProvenance(t *testing.T) {
	tests := []struct {
		name       string
		generation catalog.Generation
	}{
		{name: "ID", generation: catalog.Generation{Environment: "production", Site: "marketing", GeneratedAt: time.Now(), Complete: true}},
		{name: "generation time", generation: catalog.Generation{ID: "generation-1", Environment: "production", Site: "marketing", Complete: true}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeGeneration(t, root, "production.json", test.generation)
			store := catalog.NewFileStore(root, time.Now)
			if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true}); err == nil {
				t.Fatal("Search() error = nil")
			}
		})
	}
}

func TestFileStoreSupportsPortableEnvironmentAliasFilenames(t *testing.T) {
	tests := []struct {
		alias    string
		filename string
	}{
		{alias: "prod/us", filename: "~70726f642f7573.json"},
		{alias: `prod\us`, filename: "~70726f645c7573.json"},
		{alias: "Production", filename: "~50726f64756374696f6e.json"},
		{alias: "con", filename: "~636f6e.json"},
	}
	for _, test := range tests {
		t.Run(test.alias, func(t *testing.T) {
			root := t.TempDir()
			generated := time.Now()
			filename, err := catalog.GenerationFilename(test.alias)
			if err != nil {
				t.Fatal(err)
			}
			if filename != test.filename {
				t.Fatalf("filename = %q", filename)
			}
			writeGeneration(t, root, filename, catalog.Generation{
				ID: "generation-1", Environment: test.alias, Site: "marketing", GeneratedAt: generated, Complete: true,
			})
			store := catalog.NewFileStore(root, func() time.Time { return generated })
			result, err := store.Search(context.Background(), catalog.Query{Environment: test.alias, Site: "marketing", SiteSelected: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Environment != test.alias {
				t.Fatalf("environment = %q", result.Environment)
			}
		})
	}
}

func writeGeneration(t *testing.T, root, filename string, generation catalog.Generation) {
	t.Helper()
	data, err := json.Marshal(generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", filename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

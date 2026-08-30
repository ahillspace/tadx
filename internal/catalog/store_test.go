package catalog_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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
	if result.Page.Returned != 1 || result.Page.Total != 2 || result.Page.NextCursor == "" {
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

func TestFileStoreRejectsDefaultSiteGenerationWithoutSourceSite(t *testing.T) {
	root := t.TempDir()
	writeGenerationData(t, root, "production.json", []byte(`{
		"id":"generation-1",
		"environment":"production",
		"generated_at":"2026-08-30T00:00:00Z",
		"complete":true,
		"records":[]
	}`))
	store := catalog.NewFileStore(root, time.Now)
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "", SiteSelected: true}); err == nil {
		t.Fatal("Search() error = nil")
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

func TestFileStoreRejectsInvalidRecordIdentity(t *testing.T) {
	tests := []struct {
		name    string
		records []catalog.Record
	}{
		{name: "missing LUID", records: []catalog.Record{{Kind: "workbook", Name: "Finance"}}},
		{name: "missing kind", records: []catalog.Record{{LUID: "wb-1", Name: "Finance"}}},
		{name: "missing name", records: []catalog.Record{{LUID: "wb-1", Kind: "workbook"}}},
		{name: "duplicate LUID", records: []catalog.Record{
			{LUID: "wb-1", Kind: "workbook", Name: "Finance"},
			{LUID: "wb-1", Kind: "datasource", Name: "Finance Source"},
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeGeneration(t, root, "production.json", catalog.Generation{
				ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: time.Now(), Complete: true, Records: test.records,
			})
			store := catalog.NewFileStore(root, time.Now)
			if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true}); err == nil {
				t.Fatal("Search() error = nil")
			}
		})
	}
}

func TestFileStoreCursorIsBoundToGeneration(t *testing.T) {
	root := t.TempDir()
	generated := time.Now()
	records := []catalog.Record{
		{LUID: "wb-1", Kind: "workbook", Name: "Finance A"},
		{LUID: "wb-2", Kind: "workbook", Name: "Finance B"},
	}
	writeGeneration(t, root, "production.json", catalog.Generation{
		ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: generated, Complete: true, Records: records,
	})
	store := catalog.NewFileStore(root, func() time.Time { return generated })
	query := catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true, Limit: 1}
	first, err := store.Search(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = first.Page.NextCursor
	continued, err := store.Search(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	if len(continued.Records) != 1 || continued.Records[0].LUID != "wb-2" {
		t.Fatalf("continued records = %#v", continued.Records)
	}
	writeGeneration(t, root, "production.json", catalog.Generation{
		ID: "generation-2", Environment: "production", Site: "marketing", GeneratedAt: generated, Complete: true, Records: records,
	})
	if _, err := store.Search(context.Background(), query); err == nil {
		t.Fatal("Search() with replaced generation error = nil")
	}
}

func TestFileStoreSupportsPortableEnvironmentAliasFilenames(t *testing.T) {
	tests := []string{
		"prod/us",
		`prod\us`,
		"Production",
		"con",
		strings.Repeat("A", 126),
		strings.Repeat("a", 300),
	}
	for _, alias := range tests {
		t.Run(alias, func(t *testing.T) {
			root := t.TempDir()
			generated := time.Now()
			filename, err := catalog.GenerationFilename(alias)
			if err != nil {
				t.Fatal(err)
			}
			if len(filename) > 255 || filepath.Base(filename) != filename || !strings.HasSuffix(filename, ".json") {
				t.Fatalf("filename = %q", filename)
			}
			writeGeneration(t, root, filename, catalog.Generation{
				ID: "generation-1", Environment: alias, Site: "marketing", GeneratedAt: generated, Complete: true,
			})
			store := catalog.NewFileStore(root, func() time.Time { return generated })
			result, err := store.Search(context.Background(), catalog.Query{Environment: alias, Site: "marketing", SiteSelected: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.Environment != alias {
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
	writeGenerationData(t, root, filename, data)
}

func writeGenerationData(t *testing.T, root, filename string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "catalog"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "catalog", filename), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

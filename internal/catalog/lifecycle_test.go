package catalog_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/catalog"
)

func TestFileStoreReplacePublishesDeterministicSortedGeneration(t *testing.T) {
	root := t.TempDir()
	generatedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewFileStore(root, func() time.Time { return generatedAt })
	generation := catalog.Generation{
		Environment: "production", Site: "marketing", GeneratedAt: generatedAt, Complete: true, Source: "tableau-rest",
		Records: []catalog.Record{
			{LUID: "wb-2", Kind: "workbook", Name: "Zulu", ProjectPath: "Ops"},
			{LUID: "ds-1", Kind: "datasource", Name: "Alpha", ProjectPath: "Data"},
		},
	}
	first, err := store.Replace(context.Background(), generation)
	if err != nil {
		t.Fatal(err)
	}
	if first.GenerationID == "" || first.Path != "catalog/production.json" || first.RecordCount != 2 {
		t.Fatalf("replace result = %#v", first)
	}
	firstBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.Path)))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Replace(context.Background(), generation)
	if err != nil {
		t.Fatal(err)
	}
	secondBytes, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(second.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if first.GenerationID != second.GenerationID || string(firstBytes) != string(secondBytes) {
		t.Fatalf("replacement is not deterministic: first=%#v second=%#v", first, second)
	}
	result, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Records[0].LUID != "ds-1" || result.Records[1].LUID != "wb-2" {
		t.Fatalf("records = %#v", result.Records)
	}
}

func TestFileStoreReplaceNeverPublishesIncompleteGeneration(t *testing.T) {
	root := t.TempDir()
	store := catalog.NewFileStore(root, time.Now)
	_, err := store.Replace(context.Background(), catalog.Generation{Environment: "production", Site: "marketing", Complete: false})
	if err == nil {
		t.Fatal("Replace() error = nil")
	}
	if _, statErr := os.Stat(filepath.Join(root, "catalog", "production.json")); !os.IsNotExist(statErr) {
		t.Fatalf("published generation stat error = %v", statErr)
	}
}

func TestFileStoreGetUsesLUIDAsAuthoritativeIdentity(t *testing.T) {
	root := t.TempDir()
	generatedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewFileStore(root, func() time.Time { return generatedAt })
	_, err := store.Replace(context.Background(), catalog.Generation{
		Environment: "production", Site: "marketing", GeneratedAt: generatedAt, Complete: true,
		Records: []catalog.Record{
			{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops"},
			{LUID: "wb-2", Kind: "workbook", Name: "Finance", ProjectPath: "Executive"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	byID, err := store.Get(context.Background(), catalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "wb-1", Name: "wrong"})
	if err != nil {
		t.Fatal(err)
	}
	if byID.Record.LUID != "wb-1" {
		t.Fatalf("record = %#v", byID.Record)
	}
	_, err = store.Get(context.Background(), catalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, Kind: "workbook", Name: "Finance"})
	var ambiguous interface{ AmbiguousCatalogSelector() bool }
	if err == nil || !asAmbiguous(err, &ambiguous) {
		t.Fatalf("ambiguous lookup error = %#v", err)
	}
}

func TestFileStoreStatusReportsBoundedStalenessAndRelativePath(t *testing.T) {
	root := t.TempDir()
	generatedAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	store := catalog.NewFileStore(root, func() time.Time { return generatedAt.Add(13 * time.Hour) })
	_, err := store.Replace(context.Background(), catalog.Generation{
		Environment: "production", Site: "marketing", GeneratedAt: generatedAt, Complete: true, Source: "tableau-rest",
		Records: []catalog.Record{{LUID: "wb-1", Kind: "workbook", Name: "Finance"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(context.Background(), catalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stale || status.Age != 13*time.Hour || status.Source != "tableau-rest" || status.Path != "catalog/production.json" || len(status.Warnings) != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func asAmbiguous(err error, target *interface{ AmbiguousCatalogSelector() bool }) bool {
	for err != nil {
		if value, ok := err.(interface{ AmbiguousCatalogSelector() bool }); ok {
			*target = value
			return value.AmbiguousCatalogSelector()
		}
		value, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = value.Unwrap()
	}
	return false
}

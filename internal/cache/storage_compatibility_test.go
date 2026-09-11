package cache_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/cache"
)

func TestCacheRenamePreservesExistingSQLiteStorage(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 11, 12, 0, 0, 0, time.UTC)
	store := cache.NewTargetStore(root, "https://tableau.example.com", "site", func() time.Time { return now })
	if !strings.HasPrefix(store.RelativePath(), "catalog/target-") || !strings.HasSuffix(store.RelativePath(), ".sqlite") {
		t.Fatalf("target-bound legacy storage path changed: %q", store.RelativePath())
	}
	entry := cache.ResourceEntry{Environment: "dev", Site: "site", Kind: "workbook", LUID: "wb-1", Name: "Existing workbook", Coverage: "detail", ObservedAt: now}
	if err := store.UpsertResources(context.Background(), []cache.ResourceEntry{entry}); err != nil {
		t.Fatal(err)
	}

	// Check the established on-disk contract independently of cache's API names.
	db := openRaw(t, filepath.Join(root, filepath.FromSlash(store.RelativePath())))
	var version int
	var signature string
	if err := db.QueryRow(`SELECT version,signature FROM catalog_schema WHERE singleton=1`).Scan(&version, &signature); err != nil || version != 7 || signature != "tadx-catalog-v7" {
		t.Fatalf("storage version=%d signature=%q error=%v", version, signature, err)
	}
	var replacedTables int
	if err := db.QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name IN ('cache_schema','cache_records')`).Scan(&replacedTables); err != nil || replacedTables != 0 {
		t.Fatalf("SQLite tables were renamed: count=%d error=%v", replacedTables, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := cache.NewTargetStore(root, "https://TABLEAU.example.com:443/", "site", func() time.Time { return now })
	result, err := reopened.ReadResources(context.Background(), cache.ResourceQuery{Environment: "dev", Site: "site", Kind: "workbook", Limit: 1})
	if err != nil || len(result.Entries) != 1 || result.Entries[0].LUID != "wb-1" {
		t.Fatalf("saved observation did not survive reopen: result=%#v error=%v", result, err)
	}
}

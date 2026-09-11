package cache

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPartialInventoryCursorIsBoundedScopedAndExpires(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	input := ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", GeneratedAt: now, Entries: []ResourceEntry{
		{Environment: "dev", Site: "site", Kind: "workbook", LUID: "a", Name: "Alpha"},
		{Environment: "dev", Site: "site", Kind: "workbook", LUID: "b", Name: "Beta"},
		{Environment: "dev", Site: "site", Kind: "workbook", LUID: "c", Name: "Gamma"},
	}}
	id, err := store.SavePartialInventory(ctx, input, "One row skipped.")
	if err != nil {
		t.Fatal(err)
	}
	query := ResourceQuery{Environment: "dev", Site: "site", Kind: "workbook", Limit: 1, Offset: 1}
	query.Cursor = PartialInventoryCursor(id, query)
	query.Offset = 0
	page, err := NewStore(store.root, store.now).ReadResources(ctx, query)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].LUID != "b" || page.NextCursor == "" || page.Coverage != "partial" || page.GenerationID != "" || page.InventoryWarning != "One row skipped." {
		t.Fatalf("page = %#v %v", page, err)
	}
	for _, mutate := range []func(*ResourceQuery){
		func(q *ResourceQuery) { q.Site = "other" },
		func(q *ResourceQuery) { q.Environment = "other" },
		func(q *ResourceQuery) { q.Kind = "datasource" },
		func(q *ResourceQuery) { q.Limit = 2 },
		func(q *ResourceQuery) { q.Offset = 1 },
		func(q *ResourceQuery) { q.Name = "Beta" },
	} {
		changed := query
		mutate(&changed)
		if _, err := store.ReadResources(ctx, changed); err == nil {
			t.Fatalf("accepted changed query: %#v", changed)
		}
	}
	query.Cursor = page.NextCursor
	page, err = store.ReadResources(ctx, query)
	if err != nil || len(page.Entries) != 1 || page.Entries[0].LUID != "c" || page.NextCursor != "" {
		t.Fatalf("last page = %#v %v", page, err)
	}
	now = now.Add(partialInventoryLifetime)
	if _, err := store.ReadResources(ctx, query); err == nil {
		t.Fatal("expired cursor accepted")
	}
	if _, err := store.SavePartialInventory(ctx, input, "One row skipped."); err != nil {
		t.Fatal(err)
	}
	db, err := store.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM partial_inventory_rows WHERE snapshot_id=?`, id).Scan(&count); err != nil || count != 0 {
		t.Fatalf("expired rows retained: %d %v", count, err)
	}
}

func TestFailedExplicitRefreshPreservesVersionFiveInventory(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	store := NewStore(t.TempDir(), func() time.Time { return now })
	seed, err := store.ReplaceResourceScope(ctx, ResourceScopeReplacement{Environment: "dev", Site: "site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: []ResourceEntry{{LUID: "old", Name: "Old"}}})
	if err != nil {
		t.Fatal(err)
	}
	db, err := store.open(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{`DROP TABLE partial_inventory_rows`, `DROP TABLE partial_inventories`, `UPDATE catalog_schema SET version=5,signature='tadx-catalog-v5'`, `PRAGMA user_version=5`} {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	_, err = store.ReadResources(ctx, ResourceQuery{Environment: "dev", Site: "site", Kind: "workbook", Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "tadx cache refresh") {
		t.Fatalf("read error=%v", err)
	}
	writer, err := store.BeginRefreshGeneration(ctx, GenerationMetadata{Environment: "dev", Site: "site", GeneratedAt: now.Add(time.Minute), RequestedScopes: []string{"workbooks"}})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if _, err := writer.Publish(ctx); err == nil {
		t.Fatal("incomplete rebuild published")
	}
	if err := writer.Rollback(); err != nil {
		t.Fatal(err)
	}
	db, err = sql.Open("sqlite", filepath.Join(store.root, "catalog", "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var signature string
	if err := db.QueryRowContext(ctx, `SELECT signature FROM catalog_schema`).Scan(&signature); err != nil || signature != "tadx-catalog-v5" {
		t.Fatalf("signature = %s %v", signature, err)
	}
	var version, retained int
	if err := db.QueryRowContext(ctx, `PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("version=%d error=%v", version, err)
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM resource_scope_snapshots WHERE generation_id=?`, seed.GenerationID).Scan(&retained); err != nil || retained != 1 {
		t.Fatalf("retained generation=%d error=%v", retained, err)
	}
	var name string
	if err := db.QueryRowContext(ctx, `SELECT name FROM resource_entries WHERE luid='old'`).Scan(&name); err != nil || name != "Old" {
		t.Fatalf("old record=%q error=%v", name, err)
	}
}

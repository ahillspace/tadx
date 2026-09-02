package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/catalog"
	_ "modernc.org/sqlite"
)

func TestSQLiteStorePublishesTypedGenerationAtomically(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: now, Source: "tableau-rest",
		RequestedScopes: []string{"workbooks"}, ImplicitScopes: []string{"projects", "users"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, batch := range []catalog.Batch{
		{Scope: "projects", Columns: []string{"id", "name", "parent_project_id", "description", "owner_id"}, Rows: [][]any{{"p1", "Ops", "", "", "u1"}}},
		{Scope: "users", Columns: []string{"id", "name", "email", "site_role", "last_login"}, Rows: [][]any{{"u1", "alice", "a@example.com", "Creator", "2026-09-01T00:00:00Z"}}},
		{Scope: "workbooks", Columns: []string{"id", "name", "project_id", "owner_id", "size", "updated_at"}, Rows: [][]any{{"w1", "Finance", "p1", "u1", int64(42), "2026-09-01T00:00:00Z"}}},
	} {
		if err := writer.WriteBatch(context.Background(), batch); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.CompleteScopes(context.Background(), []string{"projects", "users", "workbooks"}); err != nil {
		t.Fatal(err)
	}
	result, err := writer.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Path != "catalog/catalog.sqlite" || result.RecordCount != 3 || result.GenerationID == "" {
		t.Fatalf("publish result = %#v", result)
	}

	search, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true, Kind: "workbook"})
	if err != nil {
		t.Fatal(err)
	}
	if len(search.Records) != 1 || search.Records[0] != (catalog.Record{LUID: "w1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "u1"}) {
		t.Fatalf("records = %#v", search.Records)
	}
	_, err = store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true, Kind: "project"})
	var unavailable interface{ CatalogScopeUnavailable() bool }
	if !errors.As(err, &unavailable) || !unavailable.CatalogScopeUnavailable() {
		t.Fatalf("implicit scope search error = %#v", err)
	}
}

func TestSQLiteStoreFailedGenerationKeepsCurrentVisible(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	first, err := store.Replace(context.Background(), catalog.Generation{
		Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true,
		Records: []catalog.Record{{LUID: "w1", Kind: "workbook", Name: "Published"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: now.Add(time.Minute), RequestedScopes: []string{"workbooks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "workbooks", Columns: []string{"id", "name", "project_id", "owner_id", "size", "updated_at"},
		Rows: [][]any{{"w2", "", "", "", 0, ""}},
	})
	if err == nil {
		_, err = writer.Publish(context.Background())
	}
	if err == nil {
		t.Fatal("invalid generation published")
	}
	_ = writer.Rollback()
	got, err := store.Status(context.Background(), catalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.GenerationID != first.GenerationID || got.RecordCount != 1 {
		t.Fatalf("status after failure = %#v", got)
	}
}

func TestSQLiteStoreCursorBindsGenerationAndFilters(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	publishRecords(t, store, now, "A", "B", "C")
	query := catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true, Kind: "workbook", Limit: 1}
	first, err := store.Search(context.Background(), query)
	if err != nil {
		t.Fatal(err)
	}
	query.Cursor = first.Page.NextCursor
	query.Text = "A"
	_, err = store.Search(context.Background(), query)
	var invalid interface{ InvalidCatalogCursor() bool }
	if !errors.As(err, &invalid) || !invalid.InvalidCatalogCursor() {
		t.Fatalf("filter-bound cursor error = %#v", err)
	}
	query.Text = ""
	publishRecords(t, store, now.Add(time.Minute), "A", "B")
	_, err = store.Search(context.Background(), query)
	if !errors.As(err, &invalid) || !invalid.InvalidCatalogCursor() {
		t.Fatalf("generation-bound cursor error = %#v", err)
	}
}

func TestSQLiteStoreRejectsSchemaVersionAndCorruption(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*testing.T, string)
	}{
		{name: "version", mutate: func(t *testing.T, path string) {
			db := openRaw(t, path)
			defer db.Close()
			if _, err := db.Exec(`PRAGMA user_version=999`); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "corruption", mutate: func(t *testing.T, path string) {
			db := openRaw(t, path)
			defer db.Close()
			if _, err := db.Exec(`DROP TABLE catalog_records`); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			store := catalog.NewStore(root, time.Now)
			publishRecords(t, store, time.Now().UTC(), "A")
			test.mutate(t, filepath.Join(root, "catalog", "catalog.sqlite"))
			_, err := store.Status(context.Background(), catalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
			if err == nil || (!strings.Contains(err.Error(), "schema") && !strings.Contains(err.Error(), "integrity")) {
				t.Fatalf("Status() error = %v", err)
			}
		})
	}
}

func TestSQLiteStoreSupportsConcurrentReaders(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	store := catalog.NewStore(root, func() time.Time { return now })
	publishRecords(t, store, now, "A", "B")
	var group sync.WaitGroup
	errorsSeen := make(chan error, 32)
	for i := 0; i < 32; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true})
			errorsSeen <- err
		}()
	}
	group.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestSQLiteStorePublishesWorkbookScopeWithoutUsersAndAllowsMissingSize(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	store := catalog.NewStore(root, func() time.Time { return now })
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: now,
		RequestedScopes: []string{"workbooks"}, ImplicitScopes: []string{"projects"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "projects", Columns: []string{"id", "name", "parent_project_id", "description", "owner_id"},
		Rows: [][]any{{"p1", "Ops", "", "", "project-owner-luid"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "workbooks", Columns: []string{"id", "name", "project_id", "owner_id", "size", "updated_at"},
		Rows: [][]any{{"w1", "Finance", "p1", "workbook-owner-luid", nil, ""}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(context.Background(), []string{"projects", "workbooks"}); err != nil {
		t.Fatal(err)
	}
	published, err := writer.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if published.RecordCount != 2 {
		t.Fatalf("record count = %d", published.RecordCount)
	}
	got, err := store.Get(context.Background(), catalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.Owner != "workbook-owner-luid" || got.Record.ProjectPath != "Ops" {
		t.Fatalf("record = %#v", got.Record)
	}
}

func TestSQLiteStoreUsesCompositeCatalogIdentityAndRejectsAmbiguousLUID(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	store := catalog.NewStore(root, func() time.Time { return now })
	_, err := store.Replace(context.Background(), catalog.Generation{
		Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true,
		Records: []catalog.Record{
			{LUID: "shared", Kind: "workbook", Name: "Workbook"},
			{LUID: "shared", Kind: "datasource", Name: "Datasource"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Get(context.Background(), catalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "shared"})
	var ambiguous interface{ AmbiguousCatalogSelector() bool }
	if !errors.As(err, &ambiguous) || !ambiguous.AmbiguousCatalogSelector() {
		t.Fatalf("Get() error = %#v", err)
	}
}

func TestSQLiteStoreScopesGenerationIDsAndPrunesSupersededGeneration(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	store := catalog.NewStore(root, func() time.Time { return now })
	for _, generation := range []catalog.Generation{
		{ID: "shared-id", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Records: []catalog.Record{{LUID: "w1", Kind: "workbook", Name: "First"}}},
		{ID: "shared-id", Environment: "development", Site: "marketing", GeneratedAt: now, Complete: true, Records: []catalog.Record{{LUID: "w2", Kind: "workbook", Name: "Other environment"}}},
		{ID: "replacement", Environment: "production", Site: "marketing", GeneratedAt: now.Add(time.Minute), Complete: true, Records: []catalog.Record{{LUID: "w3", Kind: "workbook", Name: "Replacement"}}},
	} {
		if _, err := store.Replace(context.Background(), generation); err != nil {
			t.Fatal(err)
		}
	}
	db := openRaw(t, filepath.Join(root, "catalog", "catalog.sqlite"))
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM generations`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("retained generation count = %d", count)
	}
}

func TestSQLiteStoreStatusReportsTwelveHourStalenessAndRelativePath(t *testing.T) {
	root := t.TempDir()
	generatedAt := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return generatedAt.Add(13 * time.Hour) })
	publishRecords(t, store, generatedAt, "A")
	status, err := store.Status(context.Background(), catalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Stale || status.Age != 13*time.Hour || status.Path != "catalog/catalog.sqlite" || len(status.Warnings) != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func TestSQLiteStorePermissionIdentityExcludesMutableMode(t *testing.T) {
	root := t.TempDir()
	store := catalog.NewStore(root, time.Now)
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: time.Now().UTC(), RequestedScopes: []string{"permissions"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	columns := []string{"content_type", "content_id", "grantee_type", "grantee_id", "capability", "mode"}
	if err := writer.WriteBatch(context.Background(), catalog.Batch{Scope: "permissions", Columns: columns, Rows: [][]any{{"workbook", "w1", "group", "g1", "Read", "Allow"}}}); err != nil {
		t.Fatal(err)
	}
	err = writer.WriteBatch(context.Background(), catalog.Batch{Scope: "permissions", Columns: columns, Rows: [][]any{{"workbook", "w1", "group", "g1", "Read", "Deny"}}})
	if err == nil {
		t.Fatal("duplicate permission identity was accepted")
	}
}

func publishRecords(t *testing.T, store *catalog.Store, at time.Time, names ...string) catalog.ReplaceResult {
	t.Helper()
	records := make([]catalog.Record, len(names))
	for i, name := range names {
		records[i] = catalog.Record{LUID: "w" + name, Kind: "workbook", Name: name}
	}
	result, err := store.Replace(context.Background(), catalog.Generation{Environment: "production", Site: "marketing", GeneratedAt: at, Complete: true, Records: records})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

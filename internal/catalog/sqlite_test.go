package catalog_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"runtime"
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

func TestSQLiteStoreMigratesVersionOneAndBackfillsResourceReads(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	publishRecords(t, store, now, "Finance")
	db := openRaw(t, filepath.Join(root, "catalog", "catalog.sqlite"))
	for _, statement := range []string{
		`DROP INDEX resource_entries_order_idx`,
		`DROP TABLE resource_entries`,
		`UPDATE catalog_schema SET version=1,signature='tadx-catalog-v1' WHERE singleton=1`,
		`PRAGMA user_version=1`,
	} {
		if _, err := db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	result, err := store.ReadResources(context.Background(), catalog.ResourceQuery{Environment: "production", Site: "marketing", Kind: "workbook", Limit: 25})
	if err != nil {
		t.Fatal(err)
	}
	if result.Coverage != "complete" || result.Total != 1 || result.Entries[0].Name != "Finance" {
		t.Fatalf("migrated resource result = %#v", result)
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

func TestSQLiteStoreNormalizesEnvironmentAndSiteAtBoundary(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	// Refresh stores padded environment/site values.
	if _, err := store.Replace(context.Background(), catalog.Generation{
		Environment: " prod ", Site: " marketing ", GeneratedAt: now, Complete: true,
		Records: []catalog.Record{{LUID: "w1", Kind: "workbook", Name: "Finance"}},
	}); err != nil {
		t.Fatal(err)
	}
	// A later trimmed selector must find the same current generation.
	status, err := store.Status(context.Background(), catalog.Selection{Environment: "prod", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatalf("Status() with trimmed selector = %v", err)
	}
	if status.Environment != "prod" || status.Site != "marketing" || status.RecordCount != 1 {
		t.Fatalf("status = %#v", status)
	}
	search, err := store.Search(context.Background(), catalog.Query{Environment: "prod", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatalf("Search() with trimmed selector = %v", err)
	}
	if len(search.Records) != 1 {
		t.Fatalf("records = %#v", search.Records)
	}
	// A padded selector must resolve to the same stored generation too.
	if _, err := store.Get(context.Background(), catalog.Lookup{Environment: " prod ", Site: " marketing ", SiteSelected: true, LUID: "w1"}); err != nil {
		t.Fatalf("Get() with padded selector = %v", err)
	}
}

func TestSQLiteStoreRejectsIdenticalContentUnderNewID(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := catalog.NewStore(root, func() time.Time { return now })
	records := []catalog.Record{{LUID: "w1", Kind: "workbook", Name: "Finance"}}
	if _, err := store.Replace(context.Background(), catalog.Generation{
		ID: "A", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Records: records,
	}); err != nil {
		t.Fatal(err)
	}
	_, err := store.Replace(context.Background(), catalog.Generation{
		ID: "B", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Records: records,
	})
	var duplicate interface{ CatalogDuplicateContent() bool }
	if !errors.As(err, &duplicate) || !duplicate.CatalogDuplicateContent() {
		t.Fatalf("republish under new id error = %#v", err)
	}
	if strings.Contains(strings.ToLower(errString(err)), "unique constraint") {
		t.Fatalf("raw UNIQUE violation surfaced: %v", err)
	}
}

func TestSQLiteStoreRejectsDuplicateScopes(t *testing.T) {
	root := t.TempDir()
	store := catalog.NewStore(root, time.Now)
	_, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: time.Now().UTC(),
		RequestedScopes: []string{"workbooks", "workbooks"},
	})
	var duplicate interface{ CatalogDuplicateScope() bool }
	if !errors.As(err, &duplicate) || !duplicate.CatalogDuplicateScope() {
		t.Fatalf("duplicate scope error = %#v", err)
	}
}

func TestSQLiteStoreRecordCountExcludesPermissions(t *testing.T) {
	root := t.TempDir()
	store := catalog.NewStore(root, time.Now)
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: time.Now().UTC(),
		RequestedScopes: []string{"workbooks", "permissions"}, ImplicitScopes: []string{"projects"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Rollback()
	if err := writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "projects", Columns: []string{"id", "name", "parent_project_id", "description", "owner_id"},
		Rows: [][]any{{"p1", "Ops", "", "", "u1"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "workbooks", Columns: []string{"id", "name", "project_id", "owner_id", "size", "updated_at"},
		Rows: [][]any{{"w1", "Finance", "p1", "u1", int64(1), "2026-09-01T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteBatch(context.Background(), catalog.Batch{
		Scope: "permissions", Columns: []string{"content_type", "content_id", "grantee_type", "grantee_id", "capability", "mode"},
		Rows: [][]any{{"workbook", "w1", "group", "g1", "Read", "Allow"}, {"workbook", "w1", "group", "g2", "Read", "Allow"}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := writer.CompleteScopes(context.Background(), []string{"projects", "workbooks", "permissions"}); err != nil {
		t.Fatal(err)
	}
	published, err := writer.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// project + workbook are gettable (2); the two permission rows must not count.
	if published.RecordCount != 2 {
		t.Fatalf("record count = %d, want 2 (permissions excluded)", published.RecordCount)
	}
}

func TestSQLiteStoreOpensWhenRootPathContainsURLSignificantCharacters(t *testing.T) {
	directory := "cfg?x#y"
	if runtime.GOOS == "windows" {
		// Windows forbids question marks in filesystem names, but spaces and
		// fragments still exercise SQLite URI escaping on that platform.
		directory = "cfg x#y"
	}
	root := filepath.Join(t.TempDir(), directory)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	store := catalog.NewStore(root, time.Now)
	publishRecords(t, store, time.Now().UTC(), "A")
	// Foreign keys pragma must be applied despite the '?' and '#' in the path;
	// verify by attempting an orphaned insert that violates the FK constraint.
	db := openRaw(t, filepath.Join(root, "catalog", "catalog.sqlite"))
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys=ON`); err != nil {
		t.Fatal(err)
	}
	_, err := db.Exec(`INSERT INTO catalog_records(generation_key,luid,kind,name,project_path,owner,requested) VALUES(999999,'x','workbook','x','','',1)`)
	if err == nil {
		t.Fatal("expected foreign key violation on orphaned catalog_records insert")
	}
	// And the store itself reads back correctly through the '?'/'#' path DSN.
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true}); err != nil {
		t.Fatalf("Search() through '?' path = %v", err)
	}
}

func TestSQLiteStoreEnforcesDatabaseFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on Windows")
	}
	root := t.TempDir()
	store := catalog.NewStore(root, time.Now)
	publishRecords(t, store, time.Now().UTC(), "A")
	dir := filepath.Join(root, "catalog")
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if dirInfo.Mode().Perm() != 0o700 {
		t.Fatalf("catalog dir mode = %o, want 700", dirInfo.Mode().Perm())
	}
	fileInfo, err := os.Stat(filepath.Join(dir, "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if fileInfo.Mode().Perm() != 0o600 {
		t.Fatalf("catalog db mode = %o, want 600", fileInfo.Mode().Perm())
	}
}

func TestSQLiteStoreRejectsSymlinkedCatalogDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, "catalog")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	store := catalog.NewStore(root, time.Now)
	_, err := store.Status(context.Background(), catalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlinked catalog dir error = %v", err)
	}
}

func TestSQLiteStoreReinitializesAfterPartialSchema(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "catalog", "catalog.sqlite")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash during first-time init: the atomic transaction rolled back,
	// leaving a valid but empty database file with user_version still 0. Because
	// the file now exists, the prior code skipped init and validateSchema failed
	// permanently; the recoverable design must re-initialize instead.
	db := openRaw(t, path)
	var userVersion int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatal(err)
	}
	if userVersion != 0 {
		t.Fatalf("seed user_version = %d, want 0", userVersion)
	}
	// Force the file onto disk without any catalog tables.
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("seed file missing: %v", err)
	}
	// The store must recover by re-initializing rather than failing validation.
	store := catalog.NewStore(root, time.Now)
	publishRecords(t, store, time.Now().UTC(), "A")
	if _, err := store.Search(context.Background(), catalog.Query{Environment: "production", Site: "marketing", SiteSelected: true}); err != nil {
		t.Fatalf("Search() after re-init = %v", err)
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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

// TestSQLiteStoreDeterministicFingerprintOverFullRowSet exercises every rows
// iteration loop corrected for the missing rows.Err() check: projectPaths()
// (nested hierarchy recursion), buildCatalogRecords() (all resource kinds plus
// the views join), and both fingerprint() loops (typed scope tables and the
// derived catalog_records table).
//
// A true mid-scan fault-injection test is not feasible here without inventing a
// new abstraction: these loops run against the real modernc.org/sqlite driver
// through the package's public Store API, which exposes no seam for producing a
// *sql.Rows that fails partway through iteration, and the driver does not fault
// mid-scan over well-formed rows. Introducing such a seam solely for testing was
// explicitly out of scope. Instead this drives all corrected loops end-to-end
// over a multi-scope, multi-row generation and asserts the two guarantees the
// rows.Err() checks protect: correct project_path resolution (projectPaths) and
// a deterministic content fingerprint computed over the complete row set - two
// identical generations must yield an identical fingerprint-derived GenerationID,
// which only holds if every loop iterates the full result set.
func TestSQLiteStoreDeterministicFingerprintOverFullRowSet(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	scopes := []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views"}
	batches := []catalog.Batch{
		{Scope: "users", Columns: batchColumnsFor("users"), Rows: [][]any{
			{"u1", "alice", "a@example.com", "Creator", "2026-09-01T00:00:00Z"},
			{"u2", "bob", "b@example.com", "Explorer", "2026-09-01T00:00:00Z"},
		}},
		{Scope: "groups", Columns: batchColumnsFor("groups"), Rows: [][]any{
			{"g1", "Analysts", "local"},
			{"g2", "Admins", "local"},
		}},
		{Scope: "projects", Columns: batchColumnsFor("projects"), Rows: [][]any{
			{"root", "Root", "", "top", "u1"},
			{"child", "Child", "root", "nested", "u1"},
		}},
		{Scope: "workbooks", Columns: batchColumnsFor("workbooks"), Rows: [][]any{
			{"w1", "Finance", "child", "u1", int64(42), "2026-09-01T00:00:00Z"},
		}},
		{Scope: "datasources", Columns: batchColumnsFor("datasources"), Rows: [][]any{
			{"d1", "Sales", "child", "u2", "2026-09-01T00:00:00Z"},
		}},
		{Scope: "flows", Columns: batchColumnsFor("flows"), Rows: [][]any{
			{"f1", "Prep", "root", "u2", "2026-09-01T00:00:00Z"},
		}},
		{Scope: "views", Columns: batchColumnsFor("views"), Rows: [][]any{
			{"v1", "Overview", "w1"},
		}},
	}
	publish := func() catalog.ReplaceResult {
		store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
		writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
			Environment: "production", Site: "marketing", GeneratedAt: now, Source: "tableau-rest",
			RequestedScopes: scopes,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range batches {
			if err := writer.WriteBatch(context.Background(), b); err != nil {
				t.Fatalf("write %s: %v", b.Scope, err)
			}
		}
		if err := writer.CompleteScopes(context.Background(), scopes); err != nil {
			t.Fatal(err)
		}
		result, err := writer.Publish(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		return result
	}

	first := publish()
	second := publish()

	// fingerprint() iterated every typed table and every catalog_records row.
	if first.GenerationID == "" || first.GenerationID != second.GenerationID {
		t.Fatalf("fingerprint not deterministic over full row set: %q vs %q", first.GenerationID, second.GenerationID)
	}
	// 2 users, 2 groups, 2 projects, workbook, datasource, flow, view.
	if first.RecordCount != 10 {
		t.Fatalf("record count = %d, want 10", first.RecordCount)
	}

	// projectPaths() resolved the nested hierarchy for content in the child project.
	store := catalog.NewStore(t.TempDir(), func() time.Time { return now })
	writer, err := store.BeginGeneration(context.Background(), catalog.GenerationMetadata{
		Environment: "production", Site: "marketing", GeneratedAt: now, Source: "tableau-rest",
		RequestedScopes: scopes,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range batches {
		if err := writer.WriteBatch(context.Background(), b); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.CompleteScopes(context.Background(), scopes); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Publish(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), catalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "w1"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Record.ProjectPath != "Root/Child" {
		t.Fatalf("workbook project path = %q, want Root/Child", got.Record.ProjectPath)
	}
}

func batchColumnsFor(scope string) []string {
	columns := map[string][]string{
		"users":       {"id", "name", "email", "site_role", "last_login"},
		"groups":      {"id", "name", "domain"},
		"projects":    {"id", "name", "parent_project_id", "description", "owner_id"},
		"workbooks":   {"id", "name", "project_id", "owner_id", "size", "updated_at"},
		"datasources": {"id", "name", "project_id", "owner_id", "updated_at"},
		"flows":       {"id", "name", "project_id", "owner_id", "updated_at"},
		"views":       {"id", "name", "workbook_id"},
	}
	return columns[scope]
}

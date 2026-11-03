package app

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"modernc.org/sqlite"
)

type cacheSearchObservedConnection interface {
	driver.Validator
	sync.Locker
}

type cacheSearchConnectionObservation struct {
	mu          sync.Mutex
	connections []cacheSearchObservedConnection
}

var cacheSearchConnectionObservers sync.Map

func init() {
	// Driver hooks must be registered before any connection opens. Each probe
	// observes only its own temporary database and removes its map entry.
	sqlite.RegisterConnectionHook(func(connection sqlite.ExecQuerierContext, dsn string) error {
		uri, err := url.Parse(dsn)
		if err != nil {
			return nil
		}
		value, ok := cacheSearchConnectionObservers.Load(uri.Path)
		if !ok {
			return nil
		}
		observed, ok := connection.(cacheSearchObservedConnection)
		if !ok {
			return fmt.Errorf("SQLite connection does not expose its validation/close boundary")
		}
		probe := value.(*cacheSearchConnectionObservation)
		probe.mu.Lock()
		probe.connections = append(probe.connections, observed)
		probe.mu.Unlock()
		return nil
	})
}

func observeCacheSearchConnections(t testing.TB, root string, store *cache.Store) *cacheSearchConnectionObservation {
	t.Helper()
	path, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(store.RelativePath())))
	if err != nil {
		t.Fatal(err)
	}
	key := filepath.ToSlash(path)
	if !strings.HasPrefix(key, "/") {
		key = "/" + key
	}
	probe := new(cacheSearchConnectionObservation)
	cacheSearchConnectionObservers.Store(key, probe)
	t.Cleanup(func() { cacheSearchConnectionObservers.Delete(key) })
	return probe
}

func (o *cacheSearchConnectionObservation) count() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return len(o.connections)
}

func (o *cacheSearchConnectionObservation) requireClosed(t testing.TB) {
	t.Helper()
	o.mu.Lock()
	connections := append([]cacheSearchObservedConnection(nil), o.connections...)
	o.mu.Unlock()
	deadline := time.Now().Add(time.Second)
	for {
		open := 0
		for _, connection := range connections {
			connection.Lock()
			if connection.IsValid() {
				open++
			}
			connection.Unlock()
		}
		if open == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("cached search left %d SQL connections open", open)
		}
		time.Sleep(time.Millisecond)
	}
}

func cachedSearchSessionFixture(t testing.TB, count int) (Options, *cache.Store, cache.ResourceScopeReplacement) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion, DefaultEnvironment: "fixture", Environments: map[string]config.Environment{"fixture": {URL: "https://tableau.example.test", SiteContentURL: "fixture-site", Auth: config.Auth{Type: config.AuthTypePAT}}}}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	options := Options{ConfigPath: configPath, Now: func() time.Time { return now }}
	store := cache.NewTargetStore(filepath.Dir(configPath), "https://tableau.example.test", "fixture-site", options.Now)
	replacement := cache.ResourceScopeReplacement{Environment: "fixture", Site: "fixture-site", Kind: "workbook", Source: "tableau-rest", GeneratedAt: now, Entries: make([]cache.ResourceEntry, count)}
	for index := range replacement.Entries {
		replacement.Entries[index] = cache.ResourceEntry{LUID: fmt.Sprintf("workbook-%05d", index), Name: fmt.Sprintf("Workbook %05d", index), Owner: "fixture-owner", ProjectPath: "Analytics"}
	}
	if _, err := store.ReplaceResourceScope(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	return options, store, replacement
}

func TestCachedSearchCommandUsesOneConnectionForAllActionPages(t *testing.T) {
	options, store, _ := cachedSearchSessionFixture(t, 10000)
	probe := observeCacheSearchConnections(t, filepath.Dir(options.ConfigPath), store)
	var output strings.Builder
	exit := Run(t.Context(), []string{"search", "--cache", "--type", "workbook", "--limit", "2000", "--json", "--full"}, &output, options)
	var result searchaction.Output
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if exit != 0 || len(result.Items) != 2000 || result.Page.Total != 10000 || result.Items[0].LUID != "workbook-00000" || result.Items[1999].LUID != "workbook-01999" {
		t.Fatalf("cached search did not execute the complete 20-page action: exit=%d rows=%d page=%+v", exit, len(result.Items), result.Page)
	}
	if count := probe.count(); count != 1 {
		t.Fatalf("2,000-result cached search opened %d SQL connections, want one command-owned session", count)
	}
	probe.requireClosed(t)
}

func TestCachedSearchGroupedCommandDoesNotClaimOneIndependentScopeGeneration(t *testing.T) {
	options, store, original := cachedSearchSessionFixture(t, 100)
	for index, kind := range []string{"datasource", "flow", "project"} {
		replacement := original
		replacement.Kind = kind
		replacement.GeneratedAt = replacement.GeneratedAt.Add(time.Duration(index+1) * time.Minute)
		replacement.Entries = nil
		if kind == "datasource" {
			replacement.Entries = make([]cache.ResourceEntry, 100)
			for index := range replacement.Entries {
				replacement.Entries[index] = cache.ResourceEntry{LUID: fmt.Sprintf("datasource-%05d", index), Name: fmt.Sprintf("Datasource %05d", index)}
			}
		}
		if _, err := store.ReplaceResourceScope(t.Context(), replacement); err != nil {
			t.Fatal(err)
		}
	}
	probe := observeCacheSearchConnections(t, filepath.Dir(options.ConfigPath), store)
	var output strings.Builder
	exit := Run(t.Context(), []string{"search", "--cache", "--type", "content", "--limit", "200", "--json", "--full"}, &output, options)
	var result searchaction.Output
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatal(err)
	}
	if exit != 0 || len(result.Items) != 200 || result.Items[0].Type != "datasource" || result.Items[199].Type != "workbook" || result.Generation != nil || probe.count() != 1 {
		t.Fatalf("grouped action lost rows, shared ownership, or generation scope: exit=%d rows=%d generation=%+v connections=%d", exit, len(result.Items), result.Generation, probe.count())
	}
	warningCount := 0
	for _, warning := range result.Warnings {
		if warning == searchaction.UnsharedCacheGenerationWarning {
			warningCount++
		}
	}
	if warningCount != 1 {
		t.Fatalf("independent scope warning was lost: %v", result.Warnings)
	}
	probe.requireClosed(t)
}

func TestCachedSearchCommandClosesItsConnectionOnFailureAndCancellation(t *testing.T) {
	for _, cancelRead := range []bool{false, true} {
		name := "unavailable scope"
		if cancelRead {
			name = "cancelled read"
		}
		t.Run(name, func(t *testing.T) {
			options, store, _ := cachedSearchSessionFixture(t, 10000)
			probe := observeCacheSearchConnections(t, filepath.Dir(options.ConfigPath), store)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			clock := options.Now
			input := searchaction.Input{Type: "metric", Cache: true, Limit: 2000}
			if cancelRead {
				input.Type = "workbook"
				clock = func() time.Time {
					// Staleness is evaluated after the session owns its transaction.
					cancel()
					return options.Now()
				}
			}
			commands := newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: clock})
			_, err := commands.Execute(ctx, input)
			if err == nil || (cancelRead && !errors.Is(err, context.Canceled)) || probe.count() != 1 {
				t.Fatalf("failure did not traverse the command-owned session: connections=%d error=%v", probe.count(), err)
			}
			probe.requireClosed(t)
		})
	}
}

func TestCachedSearchNextCommandRevalidatesCursorAndGeneration(t *testing.T) {
	options, store, replacement := cachedSearchSessionFixture(t, 150)
	probe := observeCacheSearchConnections(t, filepath.Dir(options.ConfigPath), store)
	input := searchaction.Input{Type: "workbook", Cache: true, Limit: 20}
	run := func(input searchaction.Input) (searchaction.Output, error) {
		return newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: options.Now}).Execute(t.Context(), input)
	}
	first, err := run(input)
	if err != nil || first.Page.NextCursor == "" || first.Generation == nil {
		t.Fatalf("first command: %+v, %v", first, err)
	}
	probe.requireClosed(t)
	changed := input
	changed.Cursor, changed.Owner = first.Page.NextCursor, "different-owner"
	if _, err := run(changed); err == nil || probe.count() != 1 {
		t.Fatalf("invalid filter continuation opened the cache or was accepted: connections=%d error=%v", probe.count(), err)
	}
	input.Cursor = first.Page.NextCursor
	next, err := run(input)
	if err != nil || len(next.Items) != 20 || next.Items[0].LUID != "workbook-00020" || next.Generation.ID != first.Generation.ID || probe.count() != 2 {
		t.Fatalf("valid next command: %+v, %v connections=%d", next, err, probe.count())
	}
	probe.requireClosed(t)
	replacement.GeneratedAt = replacement.GeneratedAt.Add(time.Minute)
	replacement.Entries[149].Name = "Workbook changed after the prior command"
	if _, err := store.ReplaceResourceScope(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := run(input); err == nil || probe.count() != 4 {
		t.Fatalf("new command reused an old generation: connections=%d error=%v", probe.count(), err)
	}
	probe.requireClosed(t)
}

func TestCachedSearchCommandKeepsItsSnapshotDuringConcurrentRefresh(t *testing.T) {
	// Search actions fetch 100 rows per source page; 101 rows exercise both pages.
	const rowCount = 101
	options, store, replacement := cachedSearchSessionFixture(t, rowCount)
	probe := observeCacheSearchConnections(t, filepath.Dir(options.ConfigPath), store)
	replacement.GeneratedAt = replacement.GeneratedAt.Add(time.Minute)
	for index := range replacement.Entries {
		replacement.Entries[index].Name = fmt.Sprintf("Refreshed %05d", index)
	}
	// Bound a stalled refresh while allowing race-instrumented SQLite work.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	var refreshed sync.Once
	var refreshErr error
	clock := func() time.Time {
		// Metadata and counts have established the command's SQLite snapshot.
		// Publish a new generation before its first payload page is read.
		refreshed.Do(func() { _, refreshErr = store.ReplaceResourceScope(ctx, replacement) })
		return options.Now()
	}
	input := searchaction.Input{Type: "workbook", Cache: true, Limit: rowCount}
	first, err := newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: clock}).Execute(ctx, input)
	if err != nil || refreshErr != nil {
		t.Fatalf("search or concurrent refresh failed: query=%v refresh=%v context=%v", err, refreshErr, ctx.Err())
	}
	if len(first.Items) != rowCount || first.Page.Total != rowCount || first.Generation == nil || probe.count() != 2 {
		t.Fatalf("first search lost rows, generation, or shared ownership: rows=%d page=%+v generation=%+v connections=%d", len(first.Items), first.Page, first.Generation, probe.count())
	}
	for index, item := range first.Items {
		if item.LUID != replacement.Entries[index].LUID || item.Name != fmt.Sprintf("Workbook %05d", index) {
			t.Fatalf("first search crossed generations at row %d: %+v", index, item)
		}
	}
	probe.requireClosed(t)
	next, err := newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: options.Now}).Execute(ctx, input)
	if err != nil {
		t.Fatalf("next search failed: error=%v context=%v", err, ctx.Err())
	}
	if len(next.Items) != rowCount || next.Page.Total != rowCount || next.Generation == nil || next.Generation.ID == first.Generation.ID || probe.count() != 3 {
		t.Fatalf("next command lost rows or retained an old read snapshot: rows=%d page=%+v generation=%+v connections=%d", len(next.Items), next.Page, next.Generation, probe.count())
	}
	for index, item := range next.Items {
		if item.LUID != replacement.Entries[index].LUID || item.Name != replacement.Entries[index].Name {
			t.Fatalf("next search retained an old snapshot at row %d: %+v", index, item)
		}
	}
	probe.requireClosed(t)
}

func TestCachedSearchNewCommandsRetainTargetAndSchemaChecks(t *testing.T) {
	options, firstStore, replacement := cachedSearchSessionFixture(t, 2)
	root := filepath.Dir(options.ConfigPath)
	firstProbe := observeCacheSearchConnections(t, root, firstStore)
	configuration, err := config.Load(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	environment := configuration.Environments["fixture"]
	environment.URL = "https://other-tableau.example.test"
	configuration.Environments["fixture"] = environment
	if err := config.Save(options.ConfigPath, configuration); err != nil {
		t.Fatal(err)
	}
	secondStore := cache.NewTargetStore(root, environment.URL, environment.SiteContentURL, options.Now)
	replacement.Entries = []cache.ResourceEntry{{LUID: "other-workbook", Name: "Other target"}}
	if _, err := secondStore.ReplaceResourceScope(t.Context(), replacement); err != nil {
		t.Fatal(err)
	}
	secondProbe := observeCacheSearchConnections(t, root, secondStore)
	input := searchaction.Input{Type: "workbook", Cache: true, Limit: 2000}
	result, err := newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: options.Now}).Execute(t.Context(), input)
	if err != nil || len(result.Items) != 1 || result.Items[0].LUID != "other-workbook" || firstProbe.count() != 0 || secondProbe.count() != 1 {
		t.Fatalf("cached command crossed selected targets: result=%+v error=%v connections=%d/%d", result, err, firstProbe.count(), secondProbe.count())
	}
	secondProbe.requireClosed(t)
	db, err := sql.Open("sqlite", filepath.Join(root, filepath.FromSlash(secondStore.RelativePath())))
	if err != nil {
		t.Fatal(err)
	}
	_, updateErr := db.ExecContext(t.Context(), `PRAGMA user_version=999`)
	closeErr := db.Close()
	if updateErr != nil || closeErr != nil {
		t.Fatalf("create incompatible schema fixture: %v %v", updateErr, closeErr)
	}
	var output strings.Builder
	exit := Run(t.Context(), []string{"search", "--cache", "--type", "workbook", "--json"}, &output, options)
	var failure errs.Envelope
	if err := json.Unmarshal([]byte(output.String()), &failure); err != nil {
		t.Fatal(err)
	}
	if exit != 1 || failure.Error.ID != "search.cache.setup" || failure.Error.Operation != "search" || failure.Error.Environment != "fixture" || failure.Error.Site != "fixture-site" || !strings.Contains(failure.Error.UpstreamCause, "schema markers are inconsistent") {
		t.Fatalf("new command bypassed schema validation or lost target context: exit=%d output=%s", exit, output.String())
	}
	secondProbe.requireClosed(t)
}

func BenchmarkCachedSearchCompleteCommand(b *testing.B) {
	options, store, _ := cachedSearchSessionFixture(b, 10000)
	probe := observeCacheSearchConnections(b, filepath.Dir(options.ConfigPath), store)
	commands := newSearchCommands(&runtimeDependencies{configPath: options.ConfigPath, now: options.Now})
	input := searchaction.Input{Environment: "fixture", Type: "workbook", Cache: true, Limit: 2000}
	for b.Loop() {
		result, err := commands.Execute(b.Context(), input)
		if err != nil || len(result.Items) != 2000 {
			b.Fatalf("complete cached command: rows=%d error=%v", len(result.Items), err)
		}
	}
	b.ReportMetric(float64(probe.count())/float64(b.N), "connections/command")
	if count := probe.count(); count != b.N {
		b.Fatalf("%d commands opened %d SQL connections", b.N, count)
	}
	probe.requireClosed(b)
}

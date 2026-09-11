package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	cacherefresh "github.com/ahillspace/tadx/actions/cache/refresh"
	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

type cacheExecutorFunc func(context.Context, tableaucache.Request) (tableaucache.Response, error)

func (f cacheExecutorFunc) Do(ctx context.Context, input tableaucache.Request) (tableaucache.Response, error) {
	return f(ctx, input)
}

type cacheRunnerFunc func(context.Context, tableaucache.RunRequest, tableaucache.BatchWriter) (tableaucache.Result, error)

func (f cacheRunnerFunc) Run(ctx context.Context, input tableaucache.RunRequest, writer tableaucache.BatchWriter) (tableaucache.Result, error) {
	return f(ctx, input, writer)
}

func TestCacheHydratorStreamsSQLiteAndReturnsOnlyReceipt(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	executor := cacheExecutorFunc(func(_ context.Context, input tableaucache.Request) (tableaucache.Response, error) {
		var body string
		switch input.Scope {
		case tableaucache.ScopeProjects:
			body = cacheListXML("projects", "project", `<project id="project-1" name="Operations"><owner id="user-1"/></project>`)
		case tableaucache.ScopeWorkbooks:
			body = cacheListXML("workbooks", "workbook", `<workbook id="workbook-1" name="Finance"><project id="project-1"/><owner id="user-2"/></workbook>`)
		default:
			return tableaucache.Response{}, fmt.Errorf("unexpected scope %q", input.Scope)
		}
		return tableaucache.Response{StatusCode: http.StatusOK, Body: []byte(body), TableauRequestID: "request-" + string(input.Scope)}, nil
	})
	store := corecache.NewStore(t.TempDir(), func() time.Time { return now })
	hydrator := cacheHydrator{
		store: store, now: func() time.Time { return now },
		executorFor: func(_ context.Context, environment, site string) (tableaucache.Executor, error) {
			if environment != "production" || site != "marketing" {
				t.Fatalf("target = %s/%s", environment, site)
			}
			return executor, nil
		},
		newRunner: func(executor tableaucache.Executor) (cacheRunner, error) {
			return tableaucache.NewEngine(executor, tableaucache.Config{MaxConcurrency: 2})
		},
	}
	output, err := cacherefresh.New(hydrator).Execute(context.Background(), cacherefresh.Input{
		Environment: "production", Site: "marketing", SiteResolved: true, Scopes: []string{"workbooks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.Status != "refreshed" || output.Path != "catalog/catalog.sqlite" || output.Generation.Records != 2 {
		t.Fatalf("output = %#v", output)
	}
	compact, err := json.Marshal(output.CompactOutput())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(compact), "Finance") || strings.Contains(string(compact), "Operations") || strings.Contains(string(compact), "workbook-1") {
		t.Fatalf("refresh receipt exposed cache content: %s", compact)
	}
	item, err := store.Get(context.Background(), corecache.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "workbook-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Record.Name != "Finance" || item.Record.ProjectPath != "Operations" || item.Record.Owner != "user-2" {
		t.Fatalf("stored workbook = %#v", item.Record)
	}
	status, err := store.Status(context.Background(), corecache.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil || status.RecordCount != 2 {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
}

func TestCacheHydratorReportsPersistedSearchableRecordCount(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := corecache.NewStore(t.TempDir(), func() time.Time { return now })
	runner := cacheRunnerFunc(func(ctx context.Context, _ tableaucache.RunRequest, writer tableaucache.BatchWriter) (tableaucache.Result, error) {
		batches := []tableaucache.Batch{
			{Scope: tableaucache.ScopeProjects, Columns: mustCacheColumns(t, tableaucache.ScopeProjects), Rows: [][]any{{"project-1", "Operations", "", "", "user-1", `{}`}}},
			{Scope: tableaucache.ScopeWorkbooks, Columns: mustCacheColumns(t, tableaucache.ScopeWorkbooks), Rows: [][]any{{"workbook-1", "Finance", "project-1", "user-1", int64(1), "2026-09-01T00:00:00Z", `{}`}}},
			{Scope: tableaucache.ScopePermissions, Columns: mustCacheColumns(t, tableaucache.ScopePermissions), Rows: [][]any{{"workbook", "workbook-1", "user", "user-1", "Read", "Allow"}}},
		}
		for _, batch := range batches {
			if err := writer.WriteBatch(ctx, batch); err != nil {
				return tableaucache.Result{}, err
			}
		}
		return tableaucache.Result{
			RequestedScopes: []tableaucache.Scope{tableaucache.ScopePermissions},
			ImplicitScopes:  []tableaucache.Scope{tableaucache.ScopeProjects, tableaucache.ScopeWorkbooks},
			Counts: map[tableaucache.Scope]int64{
				tableaucache.ScopeProjects: 1, tableaucache.ScopeWorkbooks: 1, tableaucache.ScopePermissions: 1,
			},
		}, nil
	})
	hydrator := cacheHydrator{
		store: store,
		now:   func() time.Time { return now },
		executorFor: func(context.Context, string, string) (tableaucache.Executor, error) {
			return cacheExecutorFunc(func(context.Context, tableaucache.Request) (tableaucache.Response, error) {
				return tableaucache.Response{}, errors.New("unexpected request")
			}), nil
		},
		newRunner: func(tableaucache.Executor) (cacheRunner, error) { return runner, nil },
	}

	result, err := hydrator.Hydrate(context.Background(), cacherefresh.HydrationRequest{
		Environment: "production", Site: "marketing", RequestedScopes: []string{"permissions"}, ImplicitScopes: []string{"projects", "workbooks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RecordCount != 2 || result.HydratedRecordCount != 3 {
		t.Fatalf("result = %#v", result)
	}
	status, err := store.Status(context.Background(), corecache.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.RecordCount != result.RecordCount {
		t.Fatalf("refresh records = %d, status records = %d", result.RecordCount, status.RecordCount)
	}
}

func mustCacheColumns(t *testing.T, scope tableaucache.Scope) []tableaucache.Column {
	t.Helper()
	columns, ok := tableaucache.ColumnsForScope(scope)
	if !ok {
		t.Fatalf("missing columns for %q", scope)
	}
	return columns
}

func TestCacheHydratorRollsBackPartialCollection(t *testing.T) {
	store := corecache.NewStore(t.TempDir(), time.Now)
	executor := cacheExecutorFunc(func(_ context.Context, input tableaucache.Request) (tableaucache.Response, error) {
		if input.Scope == tableaucache.ScopeWorkbooks {
			return tableaucache.Response{}, errors.New("workbook inventory failed")
		}
		return tableaucache.Response{StatusCode: http.StatusOK, Body: []byte(cacheListXML("projects", "project", `<project id="project-1" name="Operations"/>`))}, nil
	})
	hydrator := cacheHydrator{
		store: store, now: time.Now,
		executorFor: func(context.Context, string, string) (tableaucache.Executor, error) { return executor, nil },
		newRunner: func(executor tableaucache.Executor) (cacheRunner, error) {
			return tableaucache.NewEngine(executor, tableaucache.Config{MaxConcurrency: 2})
		},
	}
	_, err := hydrator.Hydrate(context.Background(), cacherefresh.HydrationRequest{
		Environment: "production", Site: "marketing", RequestedScopes: []string{"workbooks"}, ImplicitScopes: []string{"projects"},
	})
	if err == nil {
		t.Fatal("Hydrate() error = nil")
	}
	if status, statusErr := store.Status(context.Background(), corecache.Selection{Environment: "production", Site: "marketing", SiteSelected: true}); statusErr != nil || status.GenerationID != "" {
		t.Fatalf("failed collection status = %#v, error = %v", status, statusErr)
	}
}

type cacheTestSession struct{}

func (cacheTestSession) Authorize(request *http.Request) {
	request.Header.Set("X-Tableau-Auth", "secret-token")
}
func (cacheTestSession) SiteLUID() string { return "site-luid" }
func (cacheTestSession) UserLUID() string { return "user-luid" }
func (cacheTestSession) String() string   { return "redacted session" }

func TestCacheExecutorUsesSharedAuthenticatedTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/sites/site-luid/workbooks" || request.URL.Query().Get("pageSize") != "1000" {
			t.Fatalf("request URL = %s", request.URL.String())
		}
		if request.Header.Get("X-Tableau-Auth") != "secret-token" || request.Header.Get("Accept") != "application/xml" {
			t.Fatalf("request headers = %#v", request.Header)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-1")
		_, _ = writer.Write([]byte(cacheListXML("workbooks", "workbook", "")))
	}))
	defer server.Close()
	transport := tableau.NewTransport(server.Client(), "3.29", func() string { return "correlation-1" })
	executor := cacheTableauExecutor{transport: transport, session: cacheTestSession{}, serverURL: server.URL, siteLUID: "site-luid"}
	response, err := executor.Do(context.Background(), tableaucache.Request{
		Path: "/workbooks", Query: url.Values{"pageNumber": {"1"}, "pageSize": {"1000"}},
		Operation: "cache.workbooks.list", MaxResponseBytes: 1024,
	})
	if err != nil || response.StatusCode != http.StatusOK || response.TableauRequestID != "request-1" {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
}

func cacheListXML(container, item, rows string) string {
	total := 0
	if rows != "" {
		total = 1
	}
	return fmt.Sprintf(`<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="%d"/><%s>%s</%s></tsResponse>`, total, container, rows, container)
}

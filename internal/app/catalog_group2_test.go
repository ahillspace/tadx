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

	catalogrefresh "github.com/ahillspace/tadx/actions/catalog/refresh"
	corecatalog "github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaucatalog "github.com/ahillspace/tadx/internal/tableau/catalog"
)

type catalogExecutorFunc func(context.Context, tableaucatalog.Request) (tableaucatalog.Response, error)

func (f catalogExecutorFunc) Do(ctx context.Context, input tableaucatalog.Request) (tableaucatalog.Response, error) {
	return f(ctx, input)
}

type catalogRunnerFunc func(context.Context, tableaucatalog.RunRequest, tableaucatalog.BatchWriter) (tableaucatalog.Result, error)

func (f catalogRunnerFunc) Run(ctx context.Context, input tableaucatalog.RunRequest, writer tableaucatalog.BatchWriter) (tableaucatalog.Result, error) {
	return f(ctx, input, writer)
}

func TestCatalogHydratorStreamsSQLiteAndReturnsOnlyReceipt(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	executor := catalogExecutorFunc(func(_ context.Context, input tableaucatalog.Request) (tableaucatalog.Response, error) {
		var body string
		switch input.Scope {
		case tableaucatalog.ScopeProjects:
			body = catalogListXML("projects", "project", `<project id="project-1" name="Operations"><owner id="user-1"/></project>`)
		case tableaucatalog.ScopeWorkbooks:
			body = catalogListXML("workbooks", "workbook", `<workbook id="workbook-1" name="Finance"><project id="project-1"/><owner id="user-2"/></workbook>`)
		default:
			return tableaucatalog.Response{}, fmt.Errorf("unexpected scope %q", input.Scope)
		}
		return tableaucatalog.Response{StatusCode: http.StatusOK, Body: []byte(body), TableauRequestID: "request-" + string(input.Scope)}, nil
	})
	store := corecatalog.NewStore(t.TempDir(), func() time.Time { return now })
	hydrator := catalogHydrator{
		store: store, now: func() time.Time { return now },
		executorFor: func(_ context.Context, environment, site string) (tableaucatalog.Executor, error) {
			if environment != "production" || site != "marketing" {
				t.Fatalf("target = %s/%s", environment, site)
			}
			return executor, nil
		},
		newRunner: func(executor tableaucatalog.Executor) (catalogRunner, error) {
			return tableaucatalog.NewEngine(executor, tableaucatalog.Config{MaxConcurrency: 2})
		},
	}
	output, err := catalogrefresh.New(hydrator).Execute(context.Background(), catalogrefresh.Input{
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
		t.Fatalf("refresh receipt exposed catalog content: %s", compact)
	}
	item, err := store.Get(context.Background(), corecatalog.Lookup{Environment: "production", Site: "marketing", SiteSelected: true, LUID: "workbook-1"})
	if err != nil {
		t.Fatal(err)
	}
	if item.Record.Name != "Finance" || item.Record.ProjectPath != "Operations" || item.Record.Owner != "user-2" {
		t.Fatalf("stored workbook = %#v", item.Record)
	}
	status, err := store.Status(context.Background(), corecatalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil || status.RecordCount != 2 {
		t.Fatalf("status = %#v, error = %v", status, err)
	}
}

func TestCatalogHydratorReportsPersistedSearchableRecordCount(t *testing.T) {
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	store := corecatalog.NewStore(t.TempDir(), func() time.Time { return now })
	runner := catalogRunnerFunc(func(ctx context.Context, _ tableaucatalog.RunRequest, writer tableaucatalog.BatchWriter) (tableaucatalog.Result, error) {
		batches := []tableaucatalog.Batch{
			{Scope: tableaucatalog.ScopeProjects, Columns: mustCatalogColumns(t, tableaucatalog.ScopeProjects), Rows: [][]any{{"project-1", "Operations", "", "", "user-1", `{}`}}},
			{Scope: tableaucatalog.ScopeWorkbooks, Columns: mustCatalogColumns(t, tableaucatalog.ScopeWorkbooks), Rows: [][]any{{"workbook-1", "Finance", "project-1", "user-1", int64(1), "2026-09-01T00:00:00Z", `{}`}}},
			{Scope: tableaucatalog.ScopePermissions, Columns: mustCatalogColumns(t, tableaucatalog.ScopePermissions), Rows: [][]any{{"workbook", "workbook-1", "user", "user-1", "Read", "Allow"}}},
		}
		for _, batch := range batches {
			if err := writer.WriteBatch(ctx, batch); err != nil {
				return tableaucatalog.Result{}, err
			}
		}
		return tableaucatalog.Result{
			RequestedScopes: []tableaucatalog.Scope{tableaucatalog.ScopePermissions},
			ImplicitScopes:  []tableaucatalog.Scope{tableaucatalog.ScopeProjects, tableaucatalog.ScopeWorkbooks},
			Counts: map[tableaucatalog.Scope]int64{
				tableaucatalog.ScopeProjects: 1, tableaucatalog.ScopeWorkbooks: 1, tableaucatalog.ScopePermissions: 1,
			},
		}, nil
	})
	hydrator := catalogHydrator{
		store: store,
		now:   func() time.Time { return now },
		executorFor: func(context.Context, string, string) (tableaucatalog.Executor, error) {
			return catalogExecutorFunc(func(context.Context, tableaucatalog.Request) (tableaucatalog.Response, error) {
				return tableaucatalog.Response{}, errors.New("unexpected request")
			}), nil
		},
		newRunner: func(tableaucatalog.Executor) (catalogRunner, error) { return runner, nil },
	}

	result, err := hydrator.Hydrate(context.Background(), catalogrefresh.HydrationRequest{
		Environment: "production", Site: "marketing", RequestedScopes: []string{"permissions"}, ImplicitScopes: []string{"projects", "workbooks"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.RecordCount != 2 || result.HydratedRecordCount != 3 {
		t.Fatalf("result = %#v", result)
	}
	status, err := store.Status(context.Background(), corecatalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true})
	if err != nil {
		t.Fatal(err)
	}
	if status.RecordCount != result.RecordCount {
		t.Fatalf("refresh records = %d, status records = %d", result.RecordCount, status.RecordCount)
	}
}

func mustCatalogColumns(t *testing.T, scope tableaucatalog.Scope) []tableaucatalog.Column {
	t.Helper()
	columns, ok := tableaucatalog.ColumnsForScope(scope)
	if !ok {
		t.Fatalf("missing columns for %q", scope)
	}
	return columns
}

func TestCatalogHydratorRollsBackPartialCollection(t *testing.T) {
	store := corecatalog.NewStore(t.TempDir(), time.Now)
	executor := catalogExecutorFunc(func(_ context.Context, input tableaucatalog.Request) (tableaucatalog.Response, error) {
		if input.Scope == tableaucatalog.ScopeWorkbooks {
			return tableaucatalog.Response{}, errors.New("workbook inventory failed")
		}
		return tableaucatalog.Response{StatusCode: http.StatusOK, Body: []byte(catalogListXML("projects", "project", `<project id="project-1" name="Operations"/>`))}, nil
	})
	hydrator := catalogHydrator{
		store: store, now: time.Now,
		executorFor: func(context.Context, string, string) (tableaucatalog.Executor, error) { return executor, nil },
		newRunner: func(executor tableaucatalog.Executor) (catalogRunner, error) {
			return tableaucatalog.NewEngine(executor, tableaucatalog.Config{MaxConcurrency: 2})
		},
	}
	_, err := hydrator.Hydrate(context.Background(), catalogrefresh.HydrationRequest{
		Environment: "production", Site: "marketing", RequestedScopes: []string{"workbooks"}, ImplicitScopes: []string{"projects"},
	})
	if err == nil {
		t.Fatal("Hydrate() error = nil")
	}
	if _, statusErr := store.Status(context.Background(), corecatalog.Selection{Environment: "production", Site: "marketing", SiteSelected: true}); statusErr == nil {
		t.Fatal("partial generation became current")
	}
}

type catalogTestSession struct{}

func (catalogTestSession) Authorize(request *http.Request) {
	request.Header.Set("X-Tableau-Auth", "secret-token")
}
func (catalogTestSession) SiteLUID() string { return "site-luid" }
func (catalogTestSession) UserLUID() string { return "user-luid" }
func (catalogTestSession) String() string   { return "redacted session" }

func TestCatalogExecutorUsesSharedAuthenticatedTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/api/3.29/sites/site-luid/workbooks" || request.URL.Query().Get("pageSize") != "1000" {
			t.Fatalf("request URL = %s", request.URL.String())
		}
		if request.Header.Get("X-Tableau-Auth") != "secret-token" || request.Header.Get("Accept") != "application/xml" {
			t.Fatalf("request headers = %#v", request.Header)
		}
		writer.Header().Set("X-Tableau-Request-Id", "request-1")
		_, _ = writer.Write([]byte(catalogListXML("workbooks", "workbook", "")))
	}))
	defer server.Close()
	transport := tableau.NewTransport(server.Client(), "3.29", func() string { return "correlation-1" })
	executor := catalogTableauExecutor{transport: transport, session: catalogTestSession{}, serverURL: server.URL, siteLUID: "site-luid"}
	response, err := executor.Do(context.Background(), tableaucatalog.Request{
		Path: "/workbooks", Query: url.Values{"pageNumber": {"1"}, "pageSize": {"1000"}},
		Operation: "catalog.workbooks.list", MaxResponseBytes: 1024,
	})
	if err != nil || response.StatusCode != http.StatusOK || response.TableauRequestID != "request-1" {
		t.Fatalf("response = %#v, error = %v", response, err)
	}
}

func catalogListXML(container, item, rows string) string {
	total := 0
	if rows != "" {
		total = 1
	}
	return fmt.Sprintf(`<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="%d"/><%s>%s</%s></tsResponse>`, total, container, rows, container)
}

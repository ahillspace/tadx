package catalog

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type executorFunc func(context.Context, Request) (Response, error)

func (f executorFunc) Do(ctx context.Context, request Request) (Response, error) {
	return f(ctx, request)
}

type memoryWriter struct {
	mu      sync.Mutex
	batches []Batch
	err     error
}

func (w *memoryWriter) WriteBatch(_ context.Context, batch Batch) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.err != nil {
		return w.err
	}
	w.batches = append(w.batches, batch)
	return nil
}

func (w *memoryWriter) rows(scope Scope) [][]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	var rows [][]any
	for _, batch := range w.batches {
		if batch.Scope == scope {
			rows = append(rows, batch.Rows...)
		}
	}
	return rows
}

func TestEngineCollectsRequestedScopesAndDependencies(t *testing.T) {
	var requestedMu sync.Mutex
	var requested []Request
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		requestedMu.Lock()
		requested = append(requested, request)
		requestedMu.Unlock()
		body := map[Scope]string{
			ScopeProjects:  listXML("projects", "project", 1, 1000, 1, `<project id="p1" name="Operations"><owner id="u1"/></project>`),
			ScopeWorkbooks: listXML("workbooks", "workbook", 1, 1000, 1, `<workbook id="w1" name="Sales"><project id="p1"/><owner id="u1"/></workbook>`),
			ScopeViews:     listXML("views", "view", 1, 1000, 1, `<view id="v1" name="Dashboard"><workbook id="w1"/></view>`),
		}[request.Scope]
		return Response{StatusCode: 200, Body: []byte(body), TableauRequestID: "request-" + string(request.Scope)}, nil
	})
	writer := &memoryWriter{}
	engine, err := NewEngine(executor, Config{MaxConcurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeViews}}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(result.RequestedScopes); got != "[views]" {
		t.Fatalf("requested scopes = %s", got)
	}
	if got := fmt.Sprint(result.ImplicitScopes); got != "[projects workbooks]" {
		t.Fatalf("implicit scopes = %s", got)
	}
	if result.Counts[ScopeProjects] != 1 || result.Counts[ScopeWorkbooks] != 1 || result.Counts[ScopeViews] != 1 {
		t.Fatalf("counts = %#v", result.Counts)
	}
	if got := fmt.Sprint(result.TableauRequestIDs); got != "[request-projects request-views request-workbooks]" {
		t.Fatalf("request IDs = %s", got)
	}
	requestedMu.Lock()
	defer requestedMu.Unlock()
	if len(requested) != 3 {
		t.Fatalf("requests = %#v", requested)
	}
	for _, request := range requested {
		if request.PageNumber != 1 || request.PageSize != 1000 || request.Query.Get("pageNumber") != "1" || request.Query.Get("pageSize") != "1000" {
			t.Fatalf("pagination request = %#v", request)
		}
		if request.MaxResponseBytes <= 0 || request.Operation == "" {
			t.Fatalf("unbounded request = %#v", request)
		}
	}
	if got := writer.rows(ScopeViews); len(got) != 1 || got[0][0] != "v1" || got[0][2] != "w1" {
		t.Fatalf("view rows = %#v", got)
	}
}

func TestEnginePrefetchesClassicPagesAndValidatesTotals(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	executor := executorFunc(func(ctx context.Context, request Request) (Response, error) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			previous := maximum.Load()
			if current <= previous || maximum.CompareAndSwap(previous, current) {
				break
			}
		}
		if request.PageNumber > 1 {
			select {
			case <-time.After(20 * time.Millisecond):
			case <-ctx.Done():
				return Response{}, ctx.Err()
			}
		}
		first := (request.PageNumber-1)*request.PageSize + 1
		count := request.PageSize
		if request.PageNumber == 3 {
			count = 1
		}
		var items strings.Builder
		for index := 0; index < count; index++ {
			id := first + index
			fmt.Fprintf(&items, `<user id="u%d" name="User %d"/>`, id, id)
		}
		return Response{
			StatusCode:       200,
			Body:             []byte(listXML("users", "user", request.PageNumber, 1000, 2001, items.String())),
			TableauRequestID: fmt.Sprintf("page-%d", request.PageNumber),
		}, nil
	})
	engine, err := NewEngine(executor, Config{InitialConcurrency: 2, MaxConcurrency: 4, MaxBatchRows: 100})
	if err != nil {
		t.Fatal(err)
	}
	writer := &memoryWriter{}
	result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if result.Counts[ScopeUsers] != 2001 || len(writer.rows(ScopeUsers)) != 2001 || result.Requests != 3 {
		t.Fatalf("result = %#v; rows = %d", result, len(writer.rows(ScopeUsers)))
	}
	if maximum.Load() < 2 {
		t.Fatalf("pages did not run concurrently: max active = %d", maximum.Load())
	}
	writer.mu.Lock()
	defer writer.mu.Unlock()
	for _, batch := range writer.batches {
		if len(batch.Rows) > 100 {
			t.Fatalf("batch contained %d rows", len(batch.Rows))
		}
	}
}

func TestEngineRejectsDuplicateIdentityAcrossPages(t *testing.T) {
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		item := `<group id="g1" name="One"/>`
		return Response{StatusCode: 200, Body: []byte(listXML("groups", "group", request.PageNumber, 1, 2, item)), TableauRequestID: fmt.Sprintf("duplicate-%d", request.PageNumber)}, nil
	})
	engine, _ := NewEngine(executor, Config{PageSize: 1, MaxConcurrency: 2})
	_, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeGroups}}, &memoryWriter{})
	if err == nil || !strings.Contains(err.Error(), `duplicate groups identity "g1"`) {
		t.Fatalf("error = %v", err)
	}
	if requestID(err) == "" {
		t.Fatalf("protocol error omitted a Tableau request ID: %v", err)
	}
}

func TestEngineRejectsPaginationDrift(t *testing.T) {
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		total := 2
		if request.PageNumber == 2 {
			total = 3
		}
		return Response{
			StatusCode:       200,
			Body:             []byte(listXML("projects", "project", request.PageNumber, 1, total, fmt.Sprintf(`<project id="p%d" name="P%d"/>`, request.PageNumber, request.PageNumber))),
			TableauRequestID: "pagination-drift",
		}, nil
	})
	engine, _ := NewEngine(executor, Config{PageSize: 1, MaxConcurrency: 2})
	_, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeProjects}}, &memoryWriter{})
	if err == nil || !strings.Contains(err.Error(), "changed pagination") || requestID(err) != "pagination-drift" {
		t.Fatalf("error = %v", err)
	}
}

func TestEngineUsesCollectedWorkbooksForPermissionFanout(t *testing.T) {
	var workbookLists atomic.Int32
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		switch {
		case request.Scope == ScopeProjects:
			return Response{StatusCode: 200, Body: []byte(listXML("projects", "project", 1, 1000, 1, `<project id="p1" name="P"/>`))}, nil
		case request.Scope == ScopeWorkbooks:
			workbookLists.Add(1)
			return Response{StatusCode: 200, Body: []byte(listXML("workbooks", "workbook", 1, 1000, 2, `<workbook id="work/book" name="One"><project id="p1"/></workbook><workbook id="w2" name="Two"><project id="p1"/></workbook>`))}, nil
		case request.Scope == ScopePermissions:
			if !strings.Contains(request.Path, "work%2Fbook") && !strings.Contains(request.Path, "/w2/") {
				t.Fatalf("permission path = %q", request.Path)
			}
			id := request.ItemID
			body := fmt.Sprintf(`<tsResponse><permissions><workbook id="%s"/><granteeCapabilities><group id="g1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`, id)
			return Response{StatusCode: 200, Body: []byte(body), TableauRequestID: "permission-" + id}, nil
		default:
			return Response{}, fmt.Errorf("unexpected request: %#v", request)
		}
	})
	engine, _ := NewEngine(executor, Config{MaxConcurrency: 4})
	writer := &memoryWriter{}
	result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopePermissions}}, writer)
	if err != nil {
		t.Fatal(err)
	}
	if workbookLists.Load() != 1 || result.Counts[ScopePermissions] != 2 || len(writer.rows(ScopePermissions)) != 2 {
		t.Fatalf("lists = %d; result = %#v; rows = %#v", workbookLists.Load(), result, writer.rows(ScopePermissions))
	}
	if got := fmt.Sprint(result.ImplicitScopes); got != "[projects workbooks]" {
		t.Fatalf("implicit scopes = %s", got)
	}
}

func TestEnginePropagatesWriterFailureAndCancellation(t *testing.T) {
	want := errors.New("database write failed")
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		return Response{StatusCode: 200, Body: []byte(listXML("users", "user", request.PageNumber, 1000, 1, `<user id="u1" name="One"/>`))}, nil
	})
	engine, _ := NewEngine(executor, Config{})
	_, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{err: want})
	if !errors.Is(err, want) {
		t.Fatalf("error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = engine.Run(ctx, RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestEngineCancelsInFlightRequest(t *testing.T) {
	started := make(chan struct{})
	executor := executorFunc(func(ctx context.Context, _ Request) (Response, error) {
		close(started)
		<-ctx.Done()
		return Response{}, ctx.Err()
	})
	engine, _ := NewEngine(executor, Config{})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := engine.Run(ctx, RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled collection did not return")
	}
}

func TestEngineDoesNotRetryPermanentStatus(t *testing.T) {
	var requests atomic.Int32
	executor := executorFunc(func(_ context.Context, _ Request) (Response, error) {
		requests.Add(1)
		return Response{StatusCode: http.StatusForbidden, TableauRequestID: "forbidden"}, nil
	})
	engine, _ := NewEngine(executor, Config{MaxRetries: 5})
	_, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{})
	if err == nil || requests.Load() != 1 || requestID(err) != "forbidden" {
		t.Fatalf("requests = %d; error = %v", requests.Load(), err)
	}
}

func TestEngineRejectsOversizedResponse(t *testing.T) {
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		return Response{StatusCode: 200, Body: make([]byte, request.MaxResponseBytes+1), TableauRequestID: "too-large"}, nil
	})
	engine, _ := NewEngine(executor, Config{MaxResponseBytes: 1024})
	_, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeUsers}}, &memoryWriter{})
	if err == nil || !strings.Contains(err.Error(), "exceeded 1024-byte limit") || requestID(err) != "too-large" {
		t.Fatalf("error = %v", err)
	}
}

func TestEngineReportsZeroCountForEmptyScope(t *testing.T) {
	executor := executorFunc(func(_ context.Context, request Request) (Response, error) {
		return Response{StatusCode: 200, Body: []byte(listXML("groups", "group", request.PageNumber, 1000, 0, ""))}, nil
	})
	engine, _ := NewEngine(executor, Config{})
	result, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{ScopeGroups}}, &memoryWriter{})
	if err != nil {
		t.Fatal(err)
	}
	count, exists := result.Counts[ScopeGroups]
	if !exists || count != 0 {
		t.Fatalf("counts = %#v", result.Counts)
	}
}

func TestNewEngineRejectsInvalidConfigurationAndScopes(t *testing.T) {
	if _, err := NewEngine(nil, Config{}); err == nil {
		t.Fatal("NewEngine accepted a nil executor")
	}
	if _, err := NewEngine(executorFunc(nil), Config{PageSize: 1001}); err == nil {
		t.Fatal("NewEngine accepted a page size above the Tableau maximum")
	}
	engine, _ := NewEngine(executorFunc(func(context.Context, Request) (Response, error) { return Response{}, nil }), Config{})
	if _, err := engine.Run(context.Background(), RunRequest{RequestedScopes: []Scope{"unknown"}}, &memoryWriter{}); err == nil {
		t.Fatal("Run accepted an unknown scope")
	}
	if _, err := engine.Run(context.Background(), RunRequest{}, nil); err == nil {
		t.Fatal("Run accepted a nil writer")
	}
}

func TestColumnsAreFixedAndDefensivelyCopied(t *testing.T) {
	columns, ok := ColumnsForScope(ScopeWorkbooks)
	if !ok || len(columns) != 6 || columns[0].Name != "id" || columns[5].Name != "updated_at" {
		t.Fatalf("columns = %#v", columns)
	}
	columns[0].Name = "remote_controlled"
	again, _ := ColumnsForScope(ScopeWorkbooks)
	if again[0].Name != "id" {
		t.Fatalf("schema was mutable: %#v", again)
	}
}

func TestPlanScopesExposesStableDependencyClosure(t *testing.T) {
	plan, err := PlanScopes([]Scope{ScopePermissions, ScopeViews, ScopeDatasources})
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(plan.Requested); got != "[datasources views permissions]" {
		t.Fatalf("requested = %s", got)
	}
	if got := fmt.Sprint(plan.Implicit); got != "[projects workbooks]" {
		t.Fatalf("implicit = %s", got)
	}
	if got := fmt.Sprint(plan.Collected); got != "[projects workbooks datasources views permissions]" {
		t.Fatalf("collected = %s", got)
	}
}

func listXML(container, _ string, page, size, total int, items string) string {
	return fmt.Sprintf(`<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><%s>%s</%s></tsResponse>`, page, size, total, container, items, container)
}

func requestID(err error) string {
	var carrier interface{ RequestID() string }
	if errors.As(err, &carrier) {
		return carrier.RequestID()
	}
	return ""
}

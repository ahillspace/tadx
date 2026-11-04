package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
	"github.com/ahillspace/tadx/internal/tableau"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func definitionBufferSource(t *testing.T, handler http.HandlerFunc) func(string) *liveSearchLister {
	t.Helper()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/-/pulse/definitions" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", http.StatusBadRequest)
			return
		}
		handler(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := tableaupulse.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), cacheTestSession{}, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return func(terms string) *liveSearchLister {
		return &liveSearchLister{terms: terms, pulse: client}
	}
}

func writeDefinitionBufferPage(t *testing.T, w http.ResponseWriter, requestID, next string, names ...string) {
	t.Helper()
	definitions := make([]any, len(names))
	for index, name := range names {
		definitions[index] = map[string]any{"metadata": map[string]string{"id": fmt.Sprintf("definition-%d", index+1), "name": name}}
	}
	w.Header().Set("X-Tableau-Request-Id", requestID)
	if err := json.NewEncoder(w).Encode(map[string]any{"definitions": definitions, "next_page_token": next}); err != nil {
		t.Error(err)
	}
}

func checkDefinitionBufferPage(t *testing.T, page resourcesearch.Page, name, requestID, next string) {
	t.Helper()
	if len(page.Items) != 1 || page.Items[0] != (resourcesearch.Item{LUID: "definition-1", Type: "definition", Name: name}) || page.NextCursor != next || page.TableauRequestID != requestID {
		t.Fatalf("definition page = %#v", page)
	}
}

func TestPulseDefinitionSearchReusesCurrentPageWithinCommand(t *testing.T) {
	var calls atomic.Int32
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Query().Get("filter") != "name:has:test" || r.URL.Query().Get("page_size") != "100" || r.URL.Query().Get("page_token") != "" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		writeDefinitionBufferPage(t, w, "request-first", "page-next", "test First")
	})
	source := newSource("test")
	for range 3 {
		page, err := source.List(t.Context(), "definition", "", 100)
		if err != nil {
			t.Fatal(err)
		}
		checkDefinitionBufferPage(t, page, "test First", "request-first", "page-next")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("replaying the same provider page made %d HTTP requests, want 1", got)
	}
}

func TestPulseDefinitionSearchBindsBufferedPageToRequest(t *testing.T) {
	for _, test := range []struct {
		name, term, token string
		limit             int
	}{
		{name: "provider token", term: "test", token: "page-next", limit: 100},
		{name: "search term", term: "other", limit: 100},
		{name: "exact search term", term: "Test", limit: 100},
		{name: "provider page size", term: "test", limit: 50},
	} {
		t.Run(test.name, func(t *testing.T) {
			var calls atomic.Int32
			newSource := definitionBufferSource(t, func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				term, token, limit := "test", "", 100
				if call > 1 {
					term, token, limit = test.term, test.token, test.limit
				}
				if r.URL.Query().Get("filter") != "name:has:"+term || r.URL.Query().Get("page_token") != token || r.URL.Query().Get("page_size") != strconv.Itoa(limit) {
					t.Errorf("request %d query = %s", call, r.URL.RawQuery)
				}
				writeDefinitionBufferPage(t, w, fmt.Sprintf("request-%d", call), "", fmt.Sprintf("test Result %d", call))
			})
			source := newSource("test")
			first, err := source.List(t.Context(), "definition", "", 100)
			if err != nil {
				t.Fatal(err)
			}
			checkDefinitionBufferPage(t, first, "test Result 1", "request-1", "")
			source.terms = test.term
			for range 2 {
				page, err := source.List(t.Context(), "definition", test.token, test.limit)
				if err != nil {
					t.Fatal(err)
				}
				checkDefinitionBufferPage(t, page, "test Result 2", "request-2", "")
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("changed request followed by replay made %d HTTP requests, want 2", got)
			}
		})
	}
}

func TestPulseDefinitionSearchDropsPreviousPageAfterAdvancing(t *testing.T) {
	var calls atomic.Int32
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		wantToken := ""
		if call == 2 {
			wantToken = "page-next"
		}
		if r.URL.Query().Get("page_token") != wantToken {
			t.Errorf("request %d token = %q, want %q", call, r.URL.Query().Get("page_token"), wantToken)
		}
		writeDefinitionBufferPage(t, w, fmt.Sprintf("request-%d", call), "", fmt.Sprintf("test Result %d", call))
	})
	source := newSource("test")
	for index, token := range []string{"", "page-next", ""} {
		page, err := source.List(t.Context(), "definition", token, 100)
		if err != nil {
			t.Fatal(err)
		}
		checkDefinitionBufferPage(t, page, fmt.Sprintf("test Result %d", index+1), fmt.Sprintf("request-%d", index+1), "")
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("returning to an evicted page made %d HTTP requests, want 3", got)
	}
}

func TestPulseDefinitionSearchRejectsCanceledBufferedHit(t *testing.T) {
	var calls atomic.Int32
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeDefinitionBufferPage(t, w, "request-first", "", "test First")
	})
	source := newSource("test")
	if _, err := source.List(t.Context(), "definition", "", 100); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	page, err := source.List(ctx, "definition", "", 100)
	if !errors.Is(err, context.Canceled) || len(page.Items) != 0 {
		t.Fatalf("canceled buffered read returned page = %#v, error = %v", page, err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("canceled buffered read made %d HTTP requests, want 1", got)
	}
}

func TestPulseDefinitionSearchRetriesFailedPageOnSameSource(t *testing.T) {
	var calls atomic.Int32
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
			return
		}
		writeDefinitionBufferPage(t, w, "request-recovered", "", "test Recovered")
	})
	source := newSource("test")
	if page, err := source.List(t.Context(), "definition", "", 100); err == nil || len(page.Items) != 0 {
		t.Fatalf("failed page = %#v, error = %v", page, err)
	}
	for range 2 {
		page, err := source.List(t.Context(), "definition", "", 100)
		if err != nil {
			t.Fatal(err)
		}
		checkDefinitionBufferPage(t, page, "test Recovered", "request-recovered", "")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("retrying failure and replaying success made %d HTTP requests, want 2", got)
	}
}

func TestPulseDefinitionSearchDiscardsPreviousPageBeforeFailedAdvance(t *testing.T) {
	var calls atomic.Int32
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if r.URL.Query().Get("page_token") == "page-next" {
			http.Error(w, "temporary provider failure", http.StatusServiceUnavailable)
			return
		}
		writeDefinitionBufferPage(t, w, fmt.Sprintf("request-%d", call), "page-next", fmt.Sprintf("test Result %d", call))
	})
	source := newSource("test")
	if _, err := source.List(t.Context(), "definition", "", 100); err != nil {
		t.Fatal(err)
	}
	if _, err := source.List(t.Context(), "definition", "page-next", 100); err == nil {
		t.Fatal("advancing to the failed provider page succeeded")
	}
	page, err := source.List(t.Context(), "definition", "", 100)
	if err != nil {
		t.Fatal(err)
	}
	checkDefinitionBufferPage(t, page, "test Result 3", "request-3", "page-next")
	if got := calls.Load(); got != 3 {
		t.Fatalf("revisiting a page after failed advancement made %d HTTP requests, want 3", got)
	}
}

func TestPulseDefinitionSearchDoesNotRememberCanceledFetch(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	newSource := definitionBufferSource(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
			<-r.Context().Done()
			return
		}
		writeDefinitionBufferPage(t, w, "request-recovered", "", "test Recovered")
	})
	source := newSource("test")
	waitCtx, stopWaiting := context.WithTimeout(t.Context(), 5*time.Second)
	defer stopWaiting()
	ctx, cancel := context.WithCancel(waitCtx)
	defer cancel()
	type fetchResult struct {
		page resourcesearch.Page
		err  error
	}
	finished := make(chan fetchResult, 1)
	go func() {
		page, err := source.List(ctx, "definition", "", 100)
		finished <- fetchResult{page: page, err: err}
	}()
	select {
	case <-started:
	case <-waitCtx.Done():
		t.Fatal("provider did not observe the cancelable request")
	}
	cancel()
	select {
	case result := <-finished:
		if result.err == nil || len(result.page.Items) != 0 {
			t.Fatalf("canceled fetch returned page = %#v, error = %v", result.page, result.err)
		}
	case <-waitCtx.Done():
		t.Fatal("provider fetch did not finish after cancellation")
	}
	for range 2 {
		page, err := source.List(t.Context(), "definition", "", 100)
		if err != nil {
			t.Fatal(err)
		}
		checkDefinitionBufferPage(t, page, "test Recovered", "request-recovered", "")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("retrying cancellation and replaying success made %d HTTP requests, want 2", got)
	}
}

func TestPulseDefinitionSearchContinuationRefreshesSourcePage(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprintf("page_changed_%t", drift), func(t *testing.T) {
			var calls, generation atomic.Int32
			newSource := definitionBufferSource(t, func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				first := "test Alpha"
				if generation.Load() != 0 {
					first = "test Changed"
				}
				writeDefinitionBufferPage(t, w, "request-page", "", first, "test Beta")
			})
			input := resourcesearch.Input{Types: []string{"definition"}, Terms: "test", Limit: 1}
			first, err := resourcesearch.NewAdapter(newSource("test")).Search(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			if len(first.Items) != 1 || first.Items[0].LUID != "definition-1" || first.Items[0].Name != "test Alpha" || first.NextCursor == "" {
				t.Fatalf("first search page = %#v", first)
			}
			if drift {
				generation.Store(1)
			}
			input.Cursor = first.NextCursor
			next, err := resourcesearch.NewAdapter(newSource("test")).Search(t.Context(), input)
			if drift {
				cursorErr, ok := errors.AsType[interface {
					error
					InvalidSearchCursor() bool
				}](err)
				if !ok || !cursorErr.InvalidSearchCursor() || len(next.Items) != 0 {
					t.Fatalf("changed source continuation returned page = %#v, error = %v", next, err)
				}
			} else if err != nil || len(next.Items) != 1 || next.Items[0].LUID != "definition-2" || next.Items[0].Name != "test Beta" || next.NextCursor != "" {
				t.Fatalf("unchanged source continuation returned page = %#v, error = %v", next, err)
			}
			if got := calls.Load(); got != 2 {
				t.Fatalf("a fresh command resumed using %d HTTP requests in total, want 2", got)
			}
		})
	}
}

package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
	"github.com/ahillspace/tadx/internal/tableau"
)

func TestCommandTransportDiscoversOncePerCanonicalServer(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/2.4/serverinfo" || r.Header.Get("X-Tableau-Auth") != "" {
			t.Error("unexpected authenticated or non-discovery request")
		}
		_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>3.9</restApiVersion></serverInfo></tsResponse>`)
	}))
	defer server.Close()
	runtime, err := newRuntime(Options{ConfigPath: t.TempDir() + "/config.yaml", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	results := make([]*tableau.Transport, 12)
	var group sync.WaitGroup
	for index := range results {
		group.Go(func() {
			url := server.URL
			if index%2 == 0 {
				url += "/"
			}
			selected, err := runtime.transport(t.Context(), url)
			if err != nil {
				t.Error(err)
				return
			}
			results[index] = selected
		})
	}
	group.Wait()
	for _, result := range results {
		if result != results[0] || result == nil || result.APIVersion() != "3.9" {
			t.Fatal("canonical server transports or negotiated versions differ")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("discovery calls=%d; want one", calls.Load())
	}
	next, err := newRuntime(Options{ConfigPath: t.TempDir() + "/config.yaml", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if _, err := next.transport(t.Context(), server.URL); err != nil || calls.Load() != 2 {
		t.Fatal("a later command must discover its own server version", err)
	}
}

func TestCommandTransportKeepsFailedDiscoveryWithoutSendingCredentials(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/api/2.4/serverinfo" {
			t.Error("business request sent after failed discovery")
		}
		_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>malformed</restApiVersion></serverInfo></tsResponse>`)
	}))
	defer server.Close()
	runtime, err := newRuntime(Options{ConfigPath: t.TempDir() + "/config.yaml", HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	for range 2 {
		if transport, err := runtime.transport(t.Context(), server.URL); transport != nil || err == nil {
			t.Fatal("discovery failure provided an optimistic transport")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("discovery calls=%d; want one failed snapshot", calls.Load())
	}
}

func TestNegotiatedVersionKeepsSearchOperationGate(t *testing.T) {
	var discoveries, signins, searches atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/2.4/serverinfo":
			discoveries.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>3.9</restApiVersion></serverInfo></tsResponse>`)
		case "/api/3.9/auth/signin":
			signins.Add(1)
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/search":
			searches.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	runtime := inventoryListRuntime(t, server)
	defer runtime.Close()
	_, err := newSearchCommands(runtime).Execute(t.Context(), searchaction.Input{Environment: "production", Type: "workbook", Terms: "Sales", Limit: 1})
	if err == nil || discoveries.Load() != 1 || signins.Load() != 1 || searches.Load() != 0 {
		t.Fatalf("negotiated operation gate: discoveries=%d signins=%d searches=%d error=%v", discoveries.Load(), signins.Load(), searches.Load(), err)
	}
}

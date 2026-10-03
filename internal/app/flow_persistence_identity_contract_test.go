package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestFlowPublicationPersistenceFailureRetainsConfirmedIdentity(t *testing.T) {
	for _, format := range []string{"compact", "full"} {
		t.Run(format, func(t *testing.T) {
			var submissions atomic.Int32
			mux := http.NewServeMux()
			mux.HandleFunc("POST /api/{version}/auth/signin", func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			})
			mux.HandleFunc("GET /api/{version}/sites/site-1/projects", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations" topLevelProject="true"/></projects></tsResponse>`)
			})
			mux.HandleFunc("GET /api/{version}/sites/site-1/flows", func(w http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><flows/></tsResponse>`)
			})
			mux.HandleFunc("POST /api/{version}/sites/site-1/flows", func(w http.ResponseWriter, _ *http.Request) {
				submissions.Add(1)
				w.Header().Set("X-Tableau-Request-Id", "flow-acceptance-request")
				w.WriteHeader(http.StatusCreated)
				_, _ = io.WriteString(w, `<tsResponse><flow id="flow-created" name="Daily" fileType="tfl"><project id="project-1"/></flow></tsResponse>`)
			})
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected flow fixture request: %s %s", r.Method, r.URL.Path)
				http.Error(w, "unexpected request", http.StatusNotFound)
			})
			server := httptest.NewTLSServer(mux)
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			file := filepath.Join(t.TempDir(), "Daily.tfl")
			if err := os.WriteFile(file, []byte(`{"flow":"daily"}`), 0o600); err != nil {
				t.Fatal(err)
			}
			blocked := filepath.Join(t.TempDir(), "blocked-receipts")
			if err := os.WriteFile(blocked, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			options := withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), JobDirectory: blocked, Stderr: io.Discard}, true)
			args := []string{"content", "flow", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}
			if format == "full" {
				args = append(args, "--full")
			}
			var output strings.Builder
			if exit := Run(t.Context(), args, &output, options); exit == 0 || submissions.Load() != 1 {
				t.Fatalf("exit=%d submissions=%d output=%s", exit, submissions.Load(), output.String())
			}
			var envelope struct {
				Output struct {
					Result struct {
						Status string `json:"status"`
						ID     string `json:"flow_luid"`
					} `json:"result"`
				} `json:"output"`
				Error struct {
					Phase     string `json:"phase"`
					Outcome   string `json:"outcome"`
					Resource  string `json:"resource"`
					RequestID string `json:"tableau_request_id"`
					Retryable *bool  `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal([]byte(output.String()), &envelope); err != nil {
				t.Fatal(err)
			}
			if envelope.Output.Result.Status != "succeeded" || envelope.Output.Result.ID != "flow-created" || envelope.Error.Resource != "flow-created" || envelope.Error.Phase != "persistence" || envelope.Error.Outcome != "confirmed" || envelope.Error.Retryable == nil || *envelope.Error.Retryable {
				t.Errorf("confirmed flow identity/outcome lost: %s", output.String())
			}
			if envelope.Error.RequestID != "flow-acceptance-request" {
				t.Errorf("confirmed flow request identity lost from error contract: %s", output.String())
			}
		})
	}
}

package app_test

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestProjectCreatePreservesEvidenceAfterAcceptedWriteThroughCLI(t *testing.T) {
	for _, responseLoss := range []bool{false, true} {
		name := "decoded identity with mismatched name"
		if responseLoss {
			name = "accepted write with truncated response"
		}
		t.Run(name, func(t *testing.T) {
			var writes atomic.Int32
			var accepted atomic.Bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/projects":
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><projects/></tsResponse>`)
				case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/projects":
					var request struct {
						Project struct {
							Name   string `xml:"name,attr"`
							Parent string `xml:"parentProjectId,attr"`
						} `xml:"project"`
					}
					if err := xml.NewDecoder(r.Body).Decode(&request); err != nil || request.Project.Name != "Requested" || request.Project.Parent != "" {
						t.Errorf("wrong native create request: %+v, %v", request, err)
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					writes.Add(1)
					accepted.Store(true)
					w.Header().Set("X-Tableau-Request-Id", "project-accepted-request")
					if responseLoss {
						// The write has taken effect, but the declared response is incomplete.
						w.Header().Set("Content-Length", "1024")
						w.WriteHeader(http.StatusCreated)
						_, _ = io.WriteString(w, `<tsResponse>`)
						return
					}
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `<tsResponse><project id="created-project" name="Unexpected"/></tsResponse>`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := withSiteMutationConsent(t, cacheResilienceOptions(t, server), true)
			var out bytes.Buffer
			exit := app.Run(t.Context(), []string{"content", "project", "create", "--environment", "production", "--name", "Requested", "--json"}, &out, options)
			var document struct {
				Error struct {
					Outcome   string `json:"outcome"`
					RequestID string `json:"tableau_request_id"`
					Retryable *bool  `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &document); err != nil {
				t.Fatal(err)
			}
			if exit == 0 || writes.Load() != 1 || !accepted.Load() {
				t.Fatalf("fixture/false-success failure: exit=%d writes=%d accepted=%t output=%s", exit, writes.Load(), accepted.Load(), out.String())
			}
			if document.Error.Outcome != "unknown" && document.Error.Outcome != "partial" {
				t.Errorf("accepted write lacks explicit partial/unknown outcome: %s", out.String())
			}
			if document.Error.RequestID != "project-accepted-request" || document.Error.Retryable == nil || *document.Error.Retryable {
				t.Errorf("accepted write loses request evidence or permits unsafe retry: %s", out.String())
			}
			if !responseLoss {
				var decoded any
				if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
					t.Fatal(err)
				}
				if !projectOutcomeContainsIdentity(decoded, "created-project") {
					t.Errorf("decoded native identity disappeared from structured output: %s", out.String())
				}
			}
		})
	}
}

func projectOutcomeContainsIdentity(value any, id string) bool {
	switch value := value.(type) {
	case map[string]any:
		for key, item := range value {
			if (key == "luid" || key == "project_luid" || key == "resource_id" || key == "resource") && item == id {
				return true
			}
			if projectOutcomeContainsIdentity(item, id) {
				return true
			}
		}
	case []any:
		for _, item := range value {
			if projectOutcomeContainsIdentity(item, id) {
				return true
			}
		}
	}
	return false
}

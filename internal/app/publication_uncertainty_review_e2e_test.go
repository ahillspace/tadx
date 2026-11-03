package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestContentMutationsRetainEvidenceThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, verb := range []string{"move", "delete", "update"} {
			for _, response := range []struct {
				name    string
				status  int
				body    string
				outcome string
			}{
				{"service failure", http.StatusServiceUnavailable, `<tsResponse><error code="503000"><summary>Unavailable</summary></error></tsResponse>`, "unknown"},
				{"unusable acknowledgement", http.StatusOK, `<tsResponse>`, "unknown"},
				{"definite rejection", http.StatusForbidden, `<tsResponse><error code="403000"><summary>Forbidden</summary></error></tsResponse>`, "not_applied"},
			} {
				t.Run(kind+"/"+verb+"/"+response.name, func(t *testing.T) {
					var writes atomic.Int32
					plural := kind + "s"
					server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
						switch {
						case strings.HasSuffix(request.URL.Path, "/auth/signin"):
							_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
						case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
							_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Original" topLevelProject="true"/><project id="project-2" name="Destination" topLevelProject="true"/></projects></tsResponse>`)
						case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/"+plural+"/resource-id"):
							_, _ = fmt.Fprintf(writer, `<tsResponse><%s id="resource-id" name="Daily"><project id="project-1" name="Original"/><owner id="user-1"/></%s></tsResponse>`, kind, kind)
						case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/"+plural):
							_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><%s/></tsResponse>`, plural)
						case (request.Method == http.MethodPut || request.Method == http.MethodDelete) && (strings.HasSuffix(request.URL.Path, "/"+plural+"/resource-id") || (kind == "flow" && verb == "update" && strings.HasSuffix(request.URL.Path, "/flows/resource-id/owner/user-2"))):
							writes.Add(1)
							writer.Header().Set("X-Tableau-Request-Id", "mutation-request")
							writer.WriteHeader(response.status)
							_, _ = io.WriteString(writer, response.body)
						default:
							t.Errorf("unexpected request: %s %s", request.Method, request.URL)
							http.NotFound(writer, request)
						}
					}))
					defer server.Close()
					configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
					t.Setenv("PROD_PAT_NAME", "fixture-name")
					t.Setenv("PROD_PAT_SECRET", "fixture-secret")
					args := []string{"content", kind, verb, "--id", "resource-id", "--environment", "production", "--json", "--full"}
					if verb == "move" {
						args = append(args, "--destination-project-id", "project-2")
					} else if verb == "update" {
						args = append(args, "--owner-id", "user-2")
					}
					var output strings.Builder
					exit := app.Run(t.Context(), args, &output, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir()}, true))
					var document struct {
						Error struct {
							Phase, Outcome, CorrectiveAction string
							Retryable                        bool
						} `json:"error"`
						Output struct {
							Plan   map[string]any `json:"plan"`
							Result map[string]any `json:"result"`
						} `json:"output"`
					}
					if err := json.Unmarshal([]byte(output.String()), &document); err != nil {
						t.Fatalf("decode result: %v: %s", err, output.String())
					}
					if exit == 0 || writes.Load() != 1 || document.Error.Retryable || document.Error.Phase != "submission" || document.Error.Outcome != response.outcome || document.Output.Plan["operation"] != kind+"."+verb {
						t.Fatalf("lost mutation contract: exit=%d writes=%d output=%s", exit, writes.Load(), output.String())
					}
					if response.outcome == "unknown" && (document.Output.Result["status"] != "unknown" || document.Output.Result[kind+"_luid"] != "resource-id" || document.Output.Result["tableau_request_id"] != "mutation-request") {
						t.Fatalf("lost uncertain identity: %s", output.String())
					}
					if strings.Contains(output.String(), "Run without --preview") {
						t.Fatalf("executed mutation retained preview resubmission instructions: %s", output.String())
					}
				})
			}
		}
	}
}

func TestFlowPublishUncertainResponseNeverAdvisesResubmission(t *testing.T) {
	var publishes atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations" topLevelProject="true"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/flows"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><flows/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/flows"):
			publishes.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "uncertain-publish-request")
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = io.WriteString(writer, `<tsResponse><error code="503000"><summary>Unavailable</summary></error></tsResponse>`)
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL)
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	flowPath := filepath.Join(t.TempDir(), "Daily.tfl")
	if err := os.WriteFile(flowPath, []byte(`{"flow":"daily"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	exit := app.Run(t.Context(), []string{"content", "flow", "publish", "--file", flowPath, "--environment", "production", "--project-id", "project-1", "--json", "--full"}, &output, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir()}, true))
	if exit == 0 || publishes.Load() != 1 {
		t.Fatalf("exit=%d publishes=%d output=%s", exit, publishes.Load(), output.String())
	}
	for _, required := range []string{`"outcome":"unknown"`, `"status":"unknown"`, `"retryable":false`, `uncertain-publish-request`} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("missing %s: %s", required, output.String())
		}
	}
	if strings.Contains(output.String(), "Retry after Tableau becomes available") {
		t.Errorf("uncertain write was advised to repeat: %s", output.String())
	}
}

func TestDatasourceAppendRejectsReplacementAfterActualChunkedUpload(t *testing.T) {
	var replaced atomic.Bool
	var chunks, publishes atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/datasources"):
			id := "ds-original"
			if replaced.Load() {
				id = "ds-replacement"
			}
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><datasources><datasource id="%s" name="Sales"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`, id)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/fileUploads"):
			replaced.Store(true)
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="staged-upload"/></tsResponse>`)
		case request.Method == http.MethodPut && strings.HasSuffix(request.URL.Path, "/fileUploads/staged-upload"):
			if _, err := io.Copy(io.Discard, request.Body); err != nil {
				t.Error(err)
			}
			chunks.Add(1)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="staged-upload"/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/datasources"):
			publishes.Add(1)
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-replacement" name="Sales"><project id="project-1"/></datasource></tsResponse>`)
		default:
			t.Errorf("unexpected request: %s %s", request.Method, request.URL)
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	path := filepath.Join(t.TempDir(), "Sales.hyper")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	// Cross the production upload threshold without a test-only transport seam.
	err = file.Truncate(64*1024*1024 + 1)
	closeErr := file.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("create native payload: %v %v", err, closeErr)
	}
	var output strings.Builder
	exit := app.Run(t.Context(), []string{"content", "datasource", "publish", "--file", path, "--environment", "production", "--project-id", "project-1", "--append", "--json", "--full"}, &output, withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir()}, true))
	if exit == 0 || chunks.Load() != 2 || publishes.Load() != 0 || !strings.Contains(output.String(), `"outcome":"not_attempted"`) || !strings.Contains(output.String(), `"existing_datasource_luid":"ds-original"`) {
		t.Fatalf("stale append destination reached final request: exit=%d chunks=%d publishes=%d output=%s", exit, chunks.Load(), publishes.Load(), output.String())
	}
	snapshots, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tadx-publish-snapshot-*"))
	if err != nil || len(snapshots) != 0 {
		t.Fatalf("prepared payload ownership leaked: %v %v", snapshots, err)
	}
}

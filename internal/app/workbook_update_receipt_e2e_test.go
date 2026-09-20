package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookUpdateReceiptReportsDescriptionFromTableauResponse(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
		full bool
	}{
		{name: "compact TOON"},
		{name: "compact JSON", args: []string{"--json"}},
		{name: "full JSON", args: []string{"--json", "--full"}, full: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gets, puts := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				writer.Header().Set("Content-Type", "application/xml")
				switch {
				case request.URL.Path == "/api/3.29/auth/signin":
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1","contentUrl":""},"user":{"id":"user-1"}}}`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
					if puts != 0 {
						t.Error("workbook update performed an extra project read after the write")
					}
					gets++
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
				case request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
					if request.Method == http.MethodPut {
						puts++
						writer.Header().Set("X-Tableau-Request-Id", "sensitive-request-id")
						_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance" description="Confirmed description"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook></tsResponse>`)
						return
					}
					if request.Method != http.MethodGet {
						t.Errorf("unexpected method %s", request.Method)
					}
					if puts != 0 {
						t.Error("workbook update performed an extra workbook read after the write")
					}
					gets++
					_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance" description="Before"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
					http.Error(writer, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()

			config := writePhaseOneConfig(t, server.URL)
			t.Setenv("PROD_PAT_NAME", "fixture-name")
			t.Setenv("PROD_PAT_SECRET", "fixture-secret")
			args := []string{"content", "workbook", "update", "--environment", "production", "--id", "wb-1", "--description", "Confirmed description"}
			args = append(args, test.args...)
			var output strings.Builder
			code := app.Run(t.Context(), args, &output, withSiteMutationConsent(t, app.Options{ConfigPath: config, HTTPClient: server.Client()}, true))
			if code != 0 || gets != 4 || puts != 1 {
				t.Fatalf("code=%d gets=%d puts=%d output=%s", code, gets, puts, output.String())
			}
			for _, want := range []string{"description", "Confirmed description", "evidence_source", "tableau_update_response", "content workbook inspect", "wb-1"} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("receipt missing %q: %s", want, output.String())
				}
			}
			if strings.Contains(output.String(), "fixture-session") || strings.Contains(output.String(), "fixture-secret") {
				t.Fatalf("receipt leaked credentials: %s", output.String())
			}
			if got := strings.Contains(output.String(), "sensitive-request-id"); got != test.full {
				t.Errorf("full=%v request ID visible=%v: %s", test.full, got, output.String())
			}
		})
	}
}

func TestWorkbookUpdateReceiptDistinguishesConfirmedClear(t *testing.T) {
	gets, puts := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1","contentUrl":""},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			gets++
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
			if request.Method == http.MethodPut {
				puts++
				_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance" description=""><project id="project-1"/><owner id="user-1"/></workbook></tsResponse>`)
				return
			}
			gets++
			_, _ = fmt.Fprint(writer, `<tsResponse><workbook id="wb-1" name="Finance" description="Before"><project id="project-1"/><owner id="user-1"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := writePhaseOneConfig(t, server.URL)
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	var output strings.Builder
	code := app.Run(t.Context(), []string{"content", "workbook", "update", "--environment", "production", "--id", "wb-1", "--description=", "--json"}, &output, withSiteMutationConsent(t, app.Options{ConfigPath: config, HTTPClient: server.Client()}, true))
	if code != 0 || gets != 4 || puts != 1 {
		t.Fatalf("code=%d gets=%d puts=%d output=%s", code, gets, puts, output.String())
	}
	for _, want := range []string{`"description":""`, `"evidence_source":"tableau_update_response"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("confirmed clear missing %q: %s", want, output.String())
		}
	}
}

func TestWorkbookUpdateBatchReceiptKeepsPerItemEvidenceBounded(t *testing.T) {
	gets, puts := 0, 0
	descriptions := map[string]string{"wb-1": "Confirmed one", "wb-2": "Confirmed two"}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1","contentUrl":""},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			if puts != 0 && puts != 1 {
				t.Errorf("unexpected project read after completed writes")
			}
			gets++
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case strings.HasPrefix(request.URL.Path, "/api/3.29/sites/site-1/workbooks/"):
			luid := strings.TrimPrefix(request.URL.Path, "/api/3.29/sites/site-1/workbooks/")
			description, ok := descriptions[luid]
			if !ok {
				t.Errorf("unexpected workbook %q", luid)
				http.Error(writer, "unexpected workbook", http.StatusNotFound)
				return
			}
			if request.Method == http.MethodPut {
				puts++
				writer.Header().Set("X-Tableau-Request-Id", "batch-sensitive-"+luid)
				_, _ = fmt.Fprintf(writer, `<tsResponse><workbook id="%s" name="Finance %s" description="%s"><project id="project-1"/><owner id="user-1"/></workbook></tsResponse>`, luid, luid, description)
				return
			}
			if request.Method != http.MethodGet {
				t.Errorf("unexpected method %s", request.Method)
			}
			gets++
			_, _ = fmt.Fprintf(writer, `<tsResponse><workbook id="%s" name="Finance %s" description="Before"><project id="project-1"/><owner id="user-1"/></workbook></tsResponse>`, luid, luid)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	batchFile := filepath.Join(t.TempDir(), "workbook-updates.json")
	if err := os.WriteFile(batchFile, []byte(`{"items":[{"id":"wb-1","description":"Confirmed one"},{"id":"wb-2","description":"Confirmed two"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := writePhaseOneConfig(t, server.URL)
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	var output strings.Builder
	code := app.Run(t.Context(), []string{"content", "workbook", "update", "--environment", "production", "--batch-file", batchFile, "--json"}, &output, withSiteMutationConsent(t, app.Options{ConfigPath: config, HTTPClient: server.Client()}, true))
	if code != 0 || gets != 8 || puts != 2 {
		t.Fatalf("code=%d gets=%d puts=%d output=%s", code, gets, puts, output.String())
	}
	if strings.Count(output.String(), `"evidence_source":"tableau_update_response"`) != 2 || strings.Count(output.String(), "Confirmed one") < 2 || strings.Count(output.String(), "Confirmed two") < 2 {
		t.Fatalf("batch receipt lost per-item evidence: %s", output.String())
	}
	if strings.Contains(output.String(), "batch-sensitive-") || strings.Contains(output.String(), "fixture-secret") || strings.Contains(output.String(), "fixture-session") {
		t.Fatalf("compact batch receipt leaked bounded details or credentials: %s", output.String())
	}
}

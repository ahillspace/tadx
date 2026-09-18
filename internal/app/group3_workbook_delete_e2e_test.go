package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookDeletePreviewAndApplyThroughCLI(t *testing.T) {
	var deletes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1" name="Ops"/><owner id="owner-1"/></workbook></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Ops" topLevelProject="true"/></projects></tsResponse>`, pageNumber, pageSize)
		case request.Method == http.MethodDelete && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
			deletes.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "delete-request")
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected Tableau request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{MutationsEnabled: false, ConfigPath: configPath, HTTPClient: server.Client()}

	preview := runWorkbookDeleteCLI(t, options, "content", "workbook", "delete", "--environment", "production", "--id", "wb-1", "--preview")
	for _, want := range []string{"mode: preview", "operation: workbook.delete", "luid: wb-1"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview missing %q:\n%s", want, preview)
		}
	}
	// The expanded details hint binds --config and points at `last --full`. On
	// Windows the config path contains backslashes, which TOON quotes; on POSIX
	// it renders unquoted. Accept both so the assertion is platform-agnostic.
	if !strings.Contains(preview, "details: tadx --config ") && !strings.Contains(preview, `details: "tadx --config `) {
		t.Fatalf("preview details did not bind --config:\n%s", preview)
	}
	if !strings.Contains(preview, "last --full") {
		t.Fatalf("preview details missing last --full recovery:\n%s", preview)
	}
	if deletes.Load() != 0 {
		t.Fatalf("preview made %d delete requests", deletes.Load())
	}

	options.MutationsEnabled = true
	applied := runWorkbookDeleteCLI(t, options, "content", "workbook", "delete", "--environment", "production", "--id", "wb-1", "--full")
	for _, want := range []string{"status: succeeded", "workbook_luid: wb-1", "tableau_request_id: delete-request"} {
		if !strings.Contains(applied, want) {
			t.Fatalf("apply output missing %q:\n%s", want, applied)
		}
	}
	if deletes.Load() != 1 {
		t.Fatalf("apply made %d delete requests", deletes.Load())
	}
}

func runWorkbookDeleteCLI(t *testing.T, options app.Options, args ...string) string {
	t.Helper()
	var output strings.Builder
	if exit := app.Run(context.Background(), args, &output, options); exit != 0 {
		t.Fatalf("%s exit = %d, output:\n%s", strings.Join(args, " "), exit, output.String())
	}
	return output.String()
}

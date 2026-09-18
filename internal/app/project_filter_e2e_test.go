package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

// TestProjectIDWorkbookListThroughCLI reproduces the publication follow-up
// lookup that used to send Tableau's unsupported projectId REST filter.
func TestProjectIDWorkbookListThroughCLI(t *testing.T) {
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	var mu sync.Mutex
	var filters []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(writer, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Publish Project"/><project id="project-other" name="Other Project"/></projects></tsResponse>`)
		case strings.HasSuffix(request.URL.Path, "/workbooks"):
			filter := request.URL.Query().Get("filter")
			mu.Lock()
			filters = append(filters, filter)
			mu.Unlock()
			if strings.Contains(filter, "projectId") {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, `<tsResponse><error code="400065"><summary>Bad Request</summary><detail>Unknown filter key "projectId" in query</detail></error></tsResponse>`)
				return
			}
			workbooks := []string{
				`<workbook id="wb-other-1" name="A Unrelated Copy"><project id="project-other" name="Other Project"/><owner id="user-1"/></workbook>`,
				`<workbook id="wb-1" name="Published Copy"><project id="project-1" name="Publish Project"/><owner id="user-1"/></workbook>`,
				`<workbook id="wb-other-2" name="Z Unrelated Copy"><project id="project-other" name="Other Project"/><owner id="user-1"/></workbook>`,
			}
			pageNumber, _ := strconv.Atoi(request.URL.Query().Get("pageNumber"))
			pageSize, _ := strconv.Atoi(request.URL.Query().Get("pageSize"))
			start := (pageNumber - 1) * pageSize
			end := min(start+pageSize, len(workbooks))
			if start < 0 || start >= len(workbooks) {
				start, end = len(workbooks), len(workbooks)
			}
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><workbooks>%s</workbooks></tsResponse>`, pageNumber, pageSize, len(workbooks), strings.Join(workbooks[start:end], ""))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var output strings.Builder
	exit := app.Run(t.Context(), []string{
		"content", "workbook", "list", "--environment", "production", "--project-id", "project-1", "--limit", "1",
	}, &output, app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()})
	if exit != 0 {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if strings.Contains(output.String(), "Unknown filter key") || !strings.Contains(output.String(), "project-1") || strings.Contains(output.String(), "project-other") {
		t.Fatalf("project ID lookup was not exact: output=%s", output.String())
	}
	output.Reset()
	exit = app.Run(t.Context(), []string{
		"content", "workbook", "list", "--environment", "production", "--project-id", "project-1", "--all",
	}, &output, app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()})
	if exit != 0 || !strings.Contains(output.String(), "project-1") || strings.Contains(output.String(), "project-other") {
		t.Fatalf("--all project ID lookup was not exact: exit=%d output=%s", exit, output.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, filter := range filters {
		if strings.Contains(filter, "projectId") {
			t.Fatalf("unsupported projectId filter sent: %q", filter)
		}
	}
}

func TestProjectIDDatasourceListThroughCLI(t *testing.T) {
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	var mu sync.Mutex
	var filters []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(writer, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Publish Project"/><project id="project-other" name="Other Project"/></projects></tsResponse>`)
		case strings.HasSuffix(request.URL.Path, "/datasources"):
			filter := request.URL.Query().Get("filter")
			mu.Lock()
			filters = append(filters, filter)
			mu.Unlock()
			if strings.Contains(filter, "projectId") {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, `<tsResponse><error code="400065"><summary>Bad Request</summary><detail>Unknown filter key "projectId" in query</detail></error></tsResponse>`)
				return
			}
			datasources := []string{
				`<datasource id="ds-other-1" name="A Unrelated Source" contentUrl="other-1" type="hyper"><project id="project-other" name="Other Project"/><owner id="user-1"/></datasource>`,
				`<datasource id="ds-1" name="Published Source" contentUrl="published" type="hyper"><project id="project-1" name="Publish Project"/><owner id="user-1"/></datasource>`,
				`<datasource id="ds-other-2" name="Z Unrelated Source" contentUrl="other-2" type="hyper"><project id="project-other" name="Other Project"/><owner id="user-1"/></datasource>`,
			}
			pageNumber, _ := strconv.Atoi(request.URL.Query().Get("pageNumber"))
			pageSize, _ := strconv.Atoi(request.URL.Query().Get("pageSize"))
			start := (pageNumber - 1) * pageSize
			end := min(start+pageSize, len(datasources))
			if start < 0 || start >= len(datasources) {
				start, end = len(datasources), len(datasources)
			}
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><datasources>%s</datasources></tsResponse>`, pageNumber, pageSize, len(datasources), strings.Join(datasources[start:end], ""))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()

	var output strings.Builder
	exit := app.Run(t.Context(), []string{
		"content", "datasource", "list", "--environment", "production", "--project-id", "project-1", "--limit", "1",
	}, &output, app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()})
	if exit != 0 {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if strings.Contains(output.String(), "Unknown filter key") || !strings.Contains(output.String(), "project-1") || strings.Contains(output.String(), "project-other") {
		t.Fatalf("project ID lookup was not exact: output=%s", output.String())
	}
	output.Reset()
	exit = app.Run(t.Context(), []string{
		"content", "datasource", "list", "--environment", "production", "--project-id", "project-1", "--all",
	}, &output, app.Options{ConfigPath: writePhaseOneConfig(t, server.URL), HTTPClient: server.Client()})
	if exit != 0 || !strings.Contains(output.String(), "project-1") || strings.Contains(output.String(), "project-other") {
		t.Fatalf("--all project ID lookup was not exact: exit=%d output=%s", exit, output.String())
	}
	mu.Lock()
	defer mu.Unlock()
	for _, filter := range filters {
		if strings.Contains(filter, "projectId") {
			t.Fatalf("unsupported projectId filter sent: %q", filter)
		}
	}
}

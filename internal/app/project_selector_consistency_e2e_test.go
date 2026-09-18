package app_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestProjectIDInspectSelectorsAreConsistentAcrossContentFamilies(t *testing.T) {
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	server := httptest.NewTLSServer(http.HandlerFunc(projectSelectorFixture(t)))
	defer server.Close()
	configPath := writePhaseOneConfig(t, server.URL)

	for _, test := range []struct {
		kind, name, identity string
	}{
		{kind: "workbook", name: "Shared Workbook", identity: "wb-1"},
		{kind: "datasource", name: "Shared Datasource", identity: "ds-1"},
		{kind: "flow", name: "Shared Flow", identity: "flow-1"},
	} {
		t.Run(test.kind+" success", func(t *testing.T) {
			var output strings.Builder
			exit := runProjectSelectorCLI(t, configPath, server.Client(), []string{"content", test.kind, "inspect", "--environment", "production", "--name", test.name, "--project-id", "project-1", "--json"}, &output)
			if exit != 0 || !strings.Contains(output.String(), test.identity) {
				t.Fatalf("exit=%d output=%s", exit, output.String())
			}
			if test.kind == "flow" {
				var cacheOutput strings.Builder
				cacheClient := &http.Client{Transport: projectSelectorFailingTransport{}}
				cacheExit := runProjectSelectorCLI(t, configPath, cacheClient, []string{"content", "flow", "inspect", "--environment", "production", "--name", test.name, "--project-id", "project-1", "--cache", "--json"}, &cacheOutput)
				if cacheExit != 0 || !strings.Contains(cacheOutput.String(), test.identity) {
					t.Fatalf("cache exit=%d output=%s", cacheExit, cacheOutput.String())
				}
			}
		})
	}

	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind+" ambiguity", func(t *testing.T) {
			var output strings.Builder
			exit := runProjectSelectorCLI(t, configPath, server.Client(), []string{"content", kind, "inspect", "--environment", "production", "--name", "Duplicate", "--project-id", "project-1", "--json"}, &output)
			if exit == 0 || !strings.Contains(strings.ToLower(output.String()), "ambiguous") {
				t.Fatalf("exit=%d output=%s", exit, output.String())
			}
		})
	}
}

func TestFlowInspectRejectsIncompleteBoundedPagination(t *testing.T) {
	t.Setenv("PROD_PAT_NAME", "test-pat-name")
	t.Setenv("PROD_PAT_SECRET", "test-pat-secret")
	server := httptest.NewTLSServer(http.HandlerFunc(projectSelectorFixture(t)))
	defer server.Close()
	configPath := writePhaseOneConfig(t, server.URL)

	var output strings.Builder
	exit := runProjectSelectorCLI(t, configPath, server.Client(), []string{"content", "flow", "inspect", "--environment", "production", "--name", "Overflow", "--project-id", "project-1", "--json"}, &output)
	if exit == 0 || !strings.Contains(strings.ToLower(output.String()), "1000-page bound") {
		t.Fatalf("exit=%d output=%s, want incomplete pagination failure", exit, output.String())
	}
}

type projectSelectorFailingTransport struct{}

func (projectSelectorFailingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("remote HTTP must not be used by cache inspect")
}

func runProjectSelectorCLI(t *testing.T, configPath string, client *http.Client, args []string, output *strings.Builder) int {
	t.Helper()
	return app.Run(t.Context(), args, output, app.Options{
		ConfigPath: configPath,
		HTTPClient: client,
		UserHomeDir: func() (string, error) {
			return filepath.Dir(configPath), nil
		},
	})
}

func projectSelectorFixture(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			_, _ = io.WriteString(writer, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			writeProjectSelectorWorkbooks(writer, request)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			writeProjectSelectorDatasources(writer, request)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/flows":
			writeProjectSelectorFlows(writer, request)
		default:
			http.NotFound(writer, request)
		}
	}
}

func writeProjectSelectorWorkbooks(writer http.ResponseWriter, request *http.Request) {
	name := request.URL.Query().Get("filter")
	items := `<workbook id="wb-1" name="Shared Workbook"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook>`
	if strings.Contains(name, "Duplicate") {
		items += `<workbook id="wb-2" name="Duplicate"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook>`
		items = `<workbook id="wb-1" name="Duplicate"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook>` + items
	}
	writeProjectSelectorPage(writer, request, "workbooks", items, strings.Count(items, "<workbook "))
}

func writeProjectSelectorDatasources(writer http.ResponseWriter, request *http.Request) {
	name := request.URL.Query().Get("filter")
	items := `<datasource id="ds-1" name="Shared Datasource" type="hyper"><project id="project-1" name="Operations"/><owner id="user-1"/></datasource>`
	if strings.Contains(name, "Duplicate") {
		items = `<datasource id="ds-1" name="Duplicate" type="hyper"><project id="project-1" name="Operations"/><owner id="user-1"/></datasource><datasource id="ds-2" name="Duplicate" type="hyper"><project id="project-1" name="Operations"/><owner id="user-1"/></datasource>`
	}
	writeProjectSelectorPage(writer, request, "datasources", items, strings.Count(items, "<datasource "))
}

func writeProjectSelectorFlows(writer http.ResponseWriter, request *http.Request) {
	name := request.URL.Query().Get("filter")
	if strings.Contains(name, "Overflow") {
		pageNumber, _ := strconv.Atoi(request.URL.Query().Get("pageNumber"))
		if pageNumber == 0 {
			pageNumber = 1
		}
		item := `<flow id="flow-overflow" name="Overflow" fileType="tflx"><project id="project-1" name="Operations"/><owner id="user-1"/></flow>`
		if pageNumber != 1 {
			item = fmt.Sprintf(`<flow id="flow-page-%d" name="Other Flow" fileType="tflx"><project id="project-1" name="Operations"/><owner id="user-1"/></flow>`, pageNumber)
		}
		writeProjectSelectorPage(writer, request, "flows", item, 1000001)
		return
	}
	items := `<flow id="flow-1" name="Shared Flow" fileType="tflx"><project id="project-1" name="Operations"/><owner id="user-1"/></flow>`
	if strings.Contains(name, "Duplicate") {
		items = `<flow id="flow-1" name="Duplicate" fileType="tflx"><project id="project-1" name="Operations"/><owner id="user-1"/></flow><flow id="flow-2" name="Duplicate" fileType="tflx"><project id="project-1" name="Operations"/><owner id="user-1"/></flow>`
	}
	writeProjectSelectorPage(writer, request, "flows", items, strings.Count(items, "<flow "))
}

func writeProjectSelectorPage(writer http.ResponseWriter, request *http.Request, collection, items string, total int) {
	pageNumber, _ := strconv.Atoi(request.URL.Query().Get("pageNumber"))
	if pageNumber == 0 {
		pageNumber = 1
	}
	pageSize := request.URL.Query().Get("pageSize")
	if pageSize == "" {
		pageSize = "1000"
	}
	writer.Header().Set("Content-Type", "application/xml")
	_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%d" pageSize="%s" totalAvailable="%d"/><%s>%s</%s></tsResponse>`, pageNumber, pageSize, total, collection, items, collection)
}

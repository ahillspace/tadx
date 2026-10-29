package app_test

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
)

const bindingWorkbook = `<?xml version='1.0' encoding='utf-8' ?>
<workbook version='18.1'>
  <datasources>
    <datasource caption='Sales &amp; Margin' inline='true' name='sqlproxy.0abc' version='18.1'>
      <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />
      <connection channel='https' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='443' server='tableau.example.com'>
      </connection>
    </datasource>
  </datasources>
</workbook>
`

func TestWorkbookPublishBindsPublishedDatasourcesThroughCLI(t *testing.T) {
	var mu sync.Mutex
	var uploads []string
	server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/xml")
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Ops"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><workbooks/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources" && request.URL.Query().Get("filter") == "contentUrl:eq:SalesMargin":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="0"/><datasources/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources" && request.URL.Query().Get("filter") == "name:eq:Sales & Margin":
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><datasources><datasource id="ds-name" name="Sales &amp; Margin" contentUrl="SalesMargin_1" updatedAt="2026-01-02T03:04:05Z"><project id="project-1" name="Ops"/><owner id="user-1"/></datasource></datasources></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-name":
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-name" name="Sales &amp; Margin" contentUrl="SalesMargin_1"><project id="project-1" name="Ops"/><owner id="user-1"/></datasource></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks/validateWorkbook":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"warnings":[]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			body, _ := io.ReadAll(request.Body)
			mu.Lock()
			uploads = append(uploads, string(body))
			mu.Unlock()
			if !strings.Contains(string(body), "dbname='SalesMargin_1'") {
				writer.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(writer, `<tsResponse><error code="400011"><summary>Bad Request</summary><detail>There was a problem publishing the file 'Finance.twb'. PublishingException: Datasource 'SalesMargin' not found for workbook 'Finance'.</detail></error></tsResponse>`)
				return
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-2" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := writePhaseOneConfigWithSite(t, server.URL, "target-site")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir(), Now: func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }, CorrelationID: func() string { return "e2e-correlation" }}, true)
	directory := t.TempDir()
	file := filepath.Join(directory, "Finance.twb")
	if err := os.WriteFile(file, []byte(bindingWorkbook), 0o600); err != nil {
		t.Fatal(err)
	}
	publish := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1"}

	var preview strings.Builder
	if exit := app.Run(context.Background(), append(publish, "--preview"), &preview, options); exit != 0 {
		t.Fatalf("preview exit = %d, output = %s", exit, preview.String())
	}
	if !strings.Contains(preview.String(), "datasource_bindings[1]{datasource,content_url,origin,target_content_url,target_luid}:\n    Sales & Margin,SalesMargin,workbook,SalesMargin,\"\"\n") {
		t.Fatalf("preview missing the workbook datasource list:\n%s", preview.String())
	}
	if strings.Contains(preview.String(), "ds-name") || strings.Contains(preview.String(), "likely fail") || strings.Contains(preview.String(), "candidate search") || len(uploads) != 0 {
		t.Fatalf("preview searched the site or uploaded: uploads=%d\n%s", len(uploads), preview.String())
	}

	var rejected strings.Builder
	if exit := app.Run(context.Background(), publish, &rejected, options); exit == 0 {
		t.Fatalf("unmapped publish succeeded:\n%s", rejected.String())
	}
	for _, want := range []string{"Your workbook contains 1 data source(s) that do not map to a published data source on this Tableau Site.", "LUID ds-name", "--datasource-binding SalesMargin=ds-name", "not_applied", "400011"} {
		if !strings.Contains(rejected.String(), want) {
			t.Fatalf("rejection missing %q:\n%s", want, rejected.String())
		}
	}

	var bound strings.Builder
	if exit := app.Run(context.Background(), append(publish, "--dsb", "Sales & Margin=ds-name"), &bound, options); exit != 0 {
		t.Fatalf("bound publish exit = %d, output = %s", exit, bound.String())
	}
	if !strings.Contains(bound.String(), "workbook_luid: wb-2") || !strings.Contains(bound.String(), "datasource_bindings[1]{datasource,content_url,origin,target_content_url,target_luid}:\n  Sales & Margin,SalesMargin,explicit,SalesMargin_1,ds-name\n") {
		t.Fatalf("bound publish output:\n%s", bound.String())
	}
	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	upload := uploads[len(uploads)-1]
	for _, want := range []string{"dbname='SalesMargin_1'", "server='" + target.Hostname() + "'", "port='" + target.Port() + "'", "id='SalesMargin_1' path='/t/target-site/datasources'"} {
		if !strings.Contains(upload, want) {
			t.Fatalf("upload missing %q:\n%s", want, upload)
		}
	}
	if after, _ := os.ReadFile(file); string(after) != bindingWorkbook {
		t.Fatal("publish changed the source workbook")
	}
	if entries, _ := os.ReadDir(directory); len(entries) != 1 {
		t.Fatalf("publish left files behind: %v", entries)
	}
}

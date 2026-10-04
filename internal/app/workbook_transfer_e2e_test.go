package app_test

import (
	"context"
	"encoding/json"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkbookPullAndPublishThroughCLIDefaultSite(t *testing.T) {
	var publishCalls atomic.Int32
	var validationCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><workbooks><workbook id="wb-1" name="Finance"><project id="project-1" name="Ops"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1" name="Ops"/><owner id="user-1"/></workbook></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1/content":
			writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Finance.twb"`)
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<workbook/>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/metadata/graphql":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-1","embeddedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}]}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="department" name="Department"/><project id="project-1" name="Ops" parentProjectId="department"/></projects></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks/validateWorkbook":
			validationCalls.Add(1)
			writer.Header().Set("Content-Type", "application/json")
			writer.Header().Set("X-Tableau-Request-Id", "validation-request")
			_, _ = io.WriteString(writer, `{"warnings":[{"severity":"WARNING","message":"Unknown map source is used","line":245,"column":18,"elementName":"map"}]}`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			publishCalls.Add(1)
			writer.Header().Set("Content-Type", "application/xml")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-2" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := writeCLIConfig(t, server.URL)
	workspace := createNamedWorkspace(t, configPath, "development")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := withSiteMutationConsent(t, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir(), Now: func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }, CorrelationID: func() string { return "e2e-correlation" }}, true)

	var pullOutput strings.Builder
	if exit := app.Run(context.Background(), []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "development", "--id", "wb-1"}, &pullOutput, options); exit != 0 {
		t.Fatalf("pull exit = %d, output = %s", exit, pullOutput.String())
	}
	artifactEntries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "workbook"))
	if err != nil || len(artifactEntries) != 1 {
		t.Fatalf("workbook artifact entries = %#v, error = %v", artifactEntries, err)
	}
	artifactPath := filepath.Join(workspace, "artifacts", "workbook", artifactEntries[0].Name())
	artifactSelector := filepath.ToSlash(filepath.Join("artifacts", "workbook", artifactEntries[0].Name()))
	if _, err := os.Stat(filepath.Join(artifactPath, "Finance.twb")); err != nil {
		t.Fatalf("pull did not create canonical artifact: %v", err)
	}
	metadataData, err := os.ReadFile(filepath.Join(artifactPath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata artifact.WorkbookMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.SourceServerOrigin != server.URL || metadata.SourceSiteLUID != "site-1" || metadata.SourceProjectName != "Department/Ops" || metadata.SourceProjectID != "project-1" || metadata.Portability != artifact.PortabilityPortable {
		t.Fatalf("artifact source provenance = %#v", metadata)
	}

	var previewOutput strings.Builder
	previewArgs := []string{"content", "workbook", "publish", "--workspace", "development", "--artifact", artifactSelector, "--environment", "production", "--project-id", "project-1", "--overwrite", "--preview"}
	previewOptions := options
	previewOptions = withSiteMutationConsent(t, previewOptions, false)
	if exit := app.Run(context.Background(), previewArgs, &previewOutput, previewOptions); exit != 0 {
		t.Fatalf("preview exit = %d, output = %s", exit, previewOutput.String())
	}
	if publishCalls.Load() != 0 || !strings.Contains(previewOutput.String(), "mode: preview") {
		t.Fatalf("preview mutated Tableau or omitted preview state: calls=%d output=%s", publishCalls.Load(), previewOutput.String())
	}
	if strings.Contains(previewOutput.String(), workspace) || !strings.Contains(previewOutput.String(), "source_luid: wb-1") || !strings.Contains(previewOutput.String(), "artifact_path: artifacts/workbook/") {
		t.Fatalf("preview did not preserve source identity and workspace-relative artifact path: %s", previewOutput.String())
	}

	var applyOutput strings.Builder
	applyArgs := append([]string(nil), previewArgs[:len(previewArgs)-1]...)
	if exit := app.Run(context.Background(), applyArgs, &applyOutput, options); exit != 0 {
		t.Fatalf("apply exit = %d, output = %s", exit, applyOutput.String())
	}
	if validationCalls.Load() != 1 || publishCalls.Load() != 1 || !strings.Contains(applyOutput.String(), "workbook_luid: wb-2") || !strings.Contains(applyOutput.String(), "validation_warnings_omitted: 1") || !strings.Contains(applyOutput.String(), "last --full") || strings.Contains(applyOutput.String(), "Unknown map source is used") {
		t.Fatalf("apply result: validation_calls=%d publish_calls=%d output=%s", validationCalls.Load(), publishCalls.Load(), applyOutput.String())
	}
}

func TestWorkbookPullAcquiresDirectPublishedDatasourceArtifactsThroughCLI(t *testing.T) {
	var datasourceGets atomic.Int32
	var datasourceDownloads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-bound":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-bound" name="Bound Book"><project id="workbook-project" name="Ops"/><owner id="user-1"/></workbook></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="3"/><projects><project id="department" name="Department"/><project id="workbook-project" name="Ops" parentProjectId="department"/><project id="datasource-project" name="Shared" parentProjectId="department"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-bound/content":
			writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Bound Book.twbx"`)
			writer.Header().Set("X-Tableau-Request-Id", "workbook-download")
			_, _ = writer.Write([]byte("native-workbook"))
		case request.Method == http.MethodPost && request.URL.Path == "/api/metadata/graphql":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"luid":"wb-bound","embeddedDatasourcesConnection":{"totalCount":2,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"id":"embedded-1","name":"Sales","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"luid":"ds-1","name":"Sales"}]}},{"id":"embedded-2","name":"Sales duplicate","parentPublishedDatasourcesConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"luid":"ds-1","name":"Sales"}]}}]}}]}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1":
			datasourceGets.Add(1)
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-1" name="Sales"><project id="datasource-project" name="Shared"/></datasource></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1/content":
			datasourceDownloads.Add(1)
			writer.Header().Set("Content-Disposition", `name="tableau_datasource"; filename="Sales.tdsx"`)
			writer.Header().Set("X-Tableau-Request-Id", "datasource-download")
			_, _ = writer.Write([]byte("native-datasource"))
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	configPath := writeCLIConfigWithSite(t, server.URL, "test-site")
	workspace := createNamedWorkspace(t, configPath, "development")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), Now: func() time.Time { return time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC) }}

	var stdout strings.Builder
	args := []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "development", "--id", "wb-bound", "--include-pds"}
	if exit := app.Run(context.Background(), args, &stdout, options); exit != 0 {
		t.Fatalf("pull exit = %d, output = %s", exit, stdout.String())
	}
	if datasourceGets.Load() != 1 || datasourceDownloads.Load() != 1 {
		t.Fatalf("datasource calls: get=%d download=%d", datasourceGets.Load(), datasourceDownloads.Load())
	}
	datasourceEntries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "datasource"))
	if err != nil || len(datasourceEntries) != 1 {
		t.Fatalf("datasource entries = %#v, error = %v", datasourceEntries, err)
	}
	datasourcePath := filepath.Join(workspace, "artifacts", "datasource", datasourceEntries[0].Name())
	payload, err := os.ReadFile(filepath.Join(datasourcePath, "Sales.tdsx"))
	if err != nil || string(payload) != "native-datasource" {
		t.Fatalf("datasource payload = %q, error = %v", payload, err)
	}
	datasourceMetadataData, err := os.ReadFile(filepath.Join(datasourcePath, "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var datasourceMetadata artifact.DatasourceMetadata
	if err := json.Unmarshal(datasourceMetadataData, &datasourceMetadata); err != nil {
		t.Fatal(err)
	}
	if datasourceMetadata.SourceProjectName != "Department/Shared" || datasourceMetadata.SourceProjectID != "datasource-project" {
		t.Fatalf("datasource project provenance = %#v", datasourceMetadata)
	}
	workbookEntries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "workbook"))
	if err != nil || len(workbookEntries) != 1 {
		t.Fatalf("workbook entries = %#v, error = %v", workbookEntries, err)
	}
	metadataData, err := os.ReadFile(filepath.Join(workspace, "artifacts", "workbook", workbookEntries[0].Name(), "metadata.json"))
	if err != nil {
		t.Fatal(err)
	}
	var metadata artifact.WorkbookMetadata
	if err := json.Unmarshal(metadataData, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata.Portability != artifact.PortabilitySourceSiteBound || !metadata.DependenciesAcquired || len(metadata.PublishedDatasources) != 1 || metadata.PublishedDatasources[0].LUID != "ds-1" || metadata.PublishedDatasources[0].LocalArtifactPath == "" || filepath.IsAbs(metadata.PublishedDatasources[0].LocalArtifactPath) {
		t.Fatalf("workbook dependency provenance = %#v", metadata)
	}
	for _, want := range []string{"portability: source-site-bound", "published_datasource_count: 1", "dependencies_acquired: true", "last --full"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("output missing %q: %s", want, stdout.String())
		}
	}
	for _, hidden := range []string{"ds-1,Sales", "baseline_fingerprint", "tableau_request_id"} {
		if strings.Contains(stdout.String(), hidden) {
			t.Fatalf("compact output exposed %q: %s", hidden, stdout.String())
		}
	}

	fullWorkspace := createNamedWorkspace(t, configPath, "full-output")
	var fullOutput strings.Builder
	fullArgs := []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "full-output", "--id", "wb-bound", "--include-pds", "--full"}
	if exit := app.Run(context.Background(), fullArgs, &fullOutput, options); exit != 0 {
		t.Fatalf("full pull exit = %d, output = %s", exit, fullOutput.String())
	}
	if datasourceGets.Load() != 2 || datasourceDownloads.Load() != 2 {
		t.Fatalf("full datasource calls: get=%d download=%d", datasourceGets.Load(), datasourceDownloads.Load())
	}
	for _, want := range []string{"ds-1,Sales", "baseline_fingerprint", "tableau_request_id: workbook-download", "artifacts/datasource/"} {
		if !strings.Contains(fullOutput.String(), want) {
			t.Fatalf("full output missing %q: %s", want, fullOutput.String())
		}
	}
	if strings.Contains(fullOutput.String(), "last --full") || strings.Contains(fullOutput.String(), "dependencies[") {
		t.Fatalf("full output retained compact hint or duplicate dependency list: %s", fullOutput.String())
	}
	if strings.Contains(fullOutput.String(), filepath.ToSlash(fullWorkspace)) || strings.Contains(fullOutput.String(), fullWorkspace) {
		t.Fatalf("full output exposed the runtime workspace root: %s", fullOutput.String())
	}
}

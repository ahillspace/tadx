package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestPhaseOneWorkbookPullAndPublishThroughCLIDefaultSite(t *testing.T) {
	var publishCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><workbooks><workbook id="wb-1" name="Finance"><project id="project-1" name="Ops"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1/content":
			writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Finance.twb"`)
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<workbook/>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			writer.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="department" name="Department"/><project id="project-1" name="Ops" parentProjectId="department"/></projects></tsResponse>`)
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

	configPath := writePhaseOneConfig(t, server.URL)
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), Now: func() time.Time { return time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC) }, CorrelationID: func() string { return "e2e-correlation" }}

	var pullOutput strings.Builder
	if exit := app.Run(context.Background(), []string{"content", "workbook", "pull", "--environment", "production", "--workspace", workspace, "--id", "wb-1"}, &pullOutput, options); exit != 0 {
		t.Fatalf("pull exit = %d, output = %s", exit, pullOutput.String())
	}
	artifactEntries, err := os.ReadDir(filepath.Join(workspace, "artifacts", "workbook"))
	if err != nil || len(artifactEntries) != 1 {
		t.Fatalf("workbook artifact entries = %#v, error = %v", artifactEntries, err)
	}
	artifactPath := filepath.Join(workspace, "artifacts", "workbook", artifactEntries[0].Name())
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
	if metadata.SourceProjectName != "Department/Ops" || metadata.SourceProjectID != "project-1" {
		t.Fatalf("artifact project provenance = %#v", metadata)
	}

	var previewOutput strings.Builder
	previewArgs := []string{"content", "workbook", "publish", "--artifact", artifactPath, "--environment", "production", "--project-id", "project-1", "--overwrite"}
	if exit := app.Run(context.Background(), previewArgs, &previewOutput, options); exit != 0 {
		t.Fatalf("preview exit = %d, output = %s", exit, previewOutput.String())
	}
	if publishCalls.Load() != 0 || !strings.Contains(previewOutput.String(), "applied: false") {
		t.Fatalf("preview mutated Tableau or omitted preview state: calls=%d output=%s", publishCalls.Load(), previewOutput.String())
	}

	var applyOutput strings.Builder
	if exit := app.Run(context.Background(), append(previewArgs, "--apply"), &applyOutput, options); exit != 0 {
		t.Fatalf("apply exit = %d, output = %s", exit, applyOutput.String())
	}
	if publishCalls.Load() != 1 || !strings.Contains(applyOutput.String(), "applied: true") || !strings.Contains(applyOutput.String(), "workbook_luid: wb-2") {
		t.Fatalf("apply result: calls=%d output=%s", publishCalls.Load(), applyOutput.String())
	}
}

func TestWorkbookArtifactCreatedByE2EIsReadable(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "tadx.yaml"), []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := artifact.NewWorkbookManager(time.Now)
	result, err := manager.Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Book.twb", Content: []byte("book"), Metadata: artifact.WorkbookMetadata{Name: "Book", TableauID: "wb", SourceEnvironment: "production", SourceSite: "", SourceProjectName: "Ops", SourceProjectID: "project-1"}})
	if err != nil || result.ArtifactPath == "" {
		t.Fatalf("artifact result = %#v, error = %v", result, err)
	}
}

func writePhaseOneConfig(t *testing.T, serverURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: \"\"\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", serverURL)
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

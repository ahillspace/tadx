package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestFlowPublishPreservesConfirmedPersistenceOutcome(t *testing.T) {
	var publishes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/auth/signin"):
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations" topLevelProject="true"/></projects></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/flows"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><flows/></tsResponse>`)
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/flows"):
			publishes.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "flow-publish-request")
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><flow id="flow-1" name="Daily" fileType="tfl" project="project-1"><project id="project-1"/></flow></tsResponse>`)
		default:
			t.Errorf("unexpected Tableau request: %s %s", request.Method, request.URL.Path)
			http.Error(writer, "unexpected request", http.StatusNotFound)
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
	jobDirectory := filepath.Join(t.TempDir(), "jobs")
	if err := os.WriteFile(jobDirectory, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	var output strings.Builder
	exit := app.Run(context.Background(), []string{"content", "flow", "publish", "--file", flowPath, "--environment", "production", "--project-id", "project-1", "--json"}, &output, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true, JobDirectory: jobDirectory})
	if exit == 0 || publishes.Load() != 1 {
		t.Fatalf("exit=%d publishes=%d output=%s", exit, publishes.Load(), output.String())
	}
	for _, want := range []string{`"status":"succeeded"`, `"phase":"persistence"`, `"outcome":"confirmed"`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("output missing %q: %s", want, output.String())
		}
	}
	for _, unwanted := range []string{`"phase":"submission"`, `"outcome":"unknown"`} {
		if strings.Contains(output.String(), unwanted) {
			t.Errorf("output replaced typed persistence outcome with %q: %s", unwanted, output.String())
		}
	}
}

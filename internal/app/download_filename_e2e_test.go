package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
)

func TestDatasourcePullSavesFormEncodedDownloadFilenameDecoded(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-encoded":
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-encoded" name="Global Sales &amp; Pipeline"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-encoded/content":
			writer.Header().Set("Content-Disposition", `name="tableau_datasource"; filename="Global+Sales+%26+Pipeline.tds"`)
			_, _ = io.WriteString(writer, `<datasource/>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, pageNumber, pageSize)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	runtime, workspace := datasourceLifecycleRuntime(t, server)
	pulled, err := datasourceops.New(datasourceops.Ports{Pull: datasourcePullProvider{commands: newRemoteContentCommands(runtime)}}).PullDatasource(context.Background(), datasourceops.PullInput{Environment: "production", Workspace: "analytics", Selector: identity.Selector{LUID: "ds-encoded"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := path.Base(pulled.Artifact.CanonicalPath); got != "Global Sales & Pipeline.tds" {
		t.Fatalf("canonical payload = %q, want decoded download filename", pulled.Artifact.CanonicalPath)
	}
	if _, err := os.Stat(filepath.Join(workspace, filepath.FromSlash(pulled.Artifact.CanonicalPath))); err != nil {
		t.Fatalf("canonical payload is not on disk: %v", err)
	}
}

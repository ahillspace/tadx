package app

import (
	"bytes"
	"context"
	"fmt"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/identity"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestDatasourcePublishPreviewFindsSpecialCharacterCollisionThroughRemoteComposition(t *testing.T) {
	const datasourceName = "Revenue & Profit, Daily"
	const datasourceXMLName = "Revenue &amp; Profit, Daily"
	var collisionRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-special":
			_, _ = fmt.Fprintf(writer, `<tsResponse><datasource id="ds-special" name="%s"><project id="project-1" name="Analytics"/></datasource></tsResponse>`, datasourceXMLName)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-special/content":
			writer.Header().Set("Content-Disposition", `attachment; filename="Revenue & Profit, Daily.tds"`)
			_, _ = io.WriteString(writer, `<datasource/>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, pageNumber, pageSize)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			collisionRequests.Add(1)
			if request.URL.Query().Get("filter") != "" {
				http.Error(writer, "special-character names cannot use the Tableau filter grammar", http.StatusBadRequest)
				return
			}
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="3"/><datasources><datasource id="ds-other" name="Other"><project id="project-1" name="Analytics"/></datasource><datasource id="ds-wrong-project" name="%s"><project id="project-2" name="Other"/></datasource><datasource id="ds-special" name="%s"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`, pageNumber, pageSize, datasourceXMLName, datasourceXMLName)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	runtime, _ := datasourceLifecycleRuntime(t, server)
	commands := newRemoteContentCommands(runtime)
	pulled, err := commands.PullDatasource(context.Background(), datasourceops.PullInput{Environment: "production", Workspace: "analytics", Selector: identity.Selector{LUID: "ds-special"}})
	if err != nil {
		t.Fatal(err)
	}
	runtime.Close()
	for _, selector := range [][]string{{"--artifact", pulled.Artifact.Path}, {"--id", "ds-special"}, {"--artifact-name", datasourceName}} {
		var output bytes.Buffer
		args := []string{"content", "datasource", "publish", "--workspace", "analytics", "--overwrite", "--project-id", "project-1", "--preview"}
		exitCode := Run(context.Background(), append(args, selector...), &output, withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), Now: runtime.now}, true))
		if exitCode != 0 {
			t.Fatalf("publish preview exit = %d, output = %s", exitCode, output.String())
		}
		for _, expected := range []string{datasourceName, "existing_datasource_luid: ds-special", "project_luid: project-1"} {
			if !strings.Contains(output.String(), expected) {
				t.Fatalf("publish preview omitted %q: %s", expected, output.String())
			}
		}
	}
	if collisionRequests.Load() != 3 {
		t.Fatalf("collision requests = %d", collisionRequests.Load())
	}
}

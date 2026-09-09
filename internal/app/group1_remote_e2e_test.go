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
)

func TestGroupOneProjectAndFlowReadsPullAndLineageThroughCLI(t *testing.T) {
	server, mutations := newGroupOneTableauServer(t)
	defer server.Close()

	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspaceRoot := createNamedWorkspace(t, configPath, "operations")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{
		ConfigPath:    configPath,
		HTTPClient:    server.Client(),
		Now:           func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
		CorrelationID: func() string { return "group-one-e2e" },
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "project list uses the configured default read environment",
			args: []string{"content", "project", "list", "--all"},
			want: []string{"status: listed", "environment: production", "site: team-site", "project-ops", "details: \"--full\""},
		},
		{
			name: "project inspect maps the canonical nested path",
			args: []string{"content", "project", "inspect", "--project", "Department/Operations"},
			want: []string{"status: found", "luid: project-ops", "path: Department/Operations"},
		},
		{
			name: "flow list uses the configured default read environment",
			args: []string{"content", "flow", "list", "--limit", "1"},
			want: []string{"status: listed", "environment: production", "flows[1]{luid,name,project_luid,project_name,file_type,updated_at}:", "flow-1,Daily Prep,project-ops,Operations,tflx"},
		},
		{
			name: "flow inspect maps authoritative identity and project path",
			args: []string{"content", "flow", "inspect", "--id", "flow-1"},
			want: []string{"status: found", "luid: flow-1", "project_luid: project-ops", "project_path: Department/Operations"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			output := runGroupOneCLI(t, options, test.args...)
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("output missing %q:\n%s", want, output)
				}
			}
		})
	}

	pullOutput := runGroupOneCLI(t, options, "content", "flow", "pull", "--workspace", "operations", "--id", "flow-1")
	for _, want := range []string{"status: pulled", "luid: flow-1", "details: \"--full\"", "artifacts/flow/"} {
		if !strings.Contains(pullOutput, want) {
			t.Fatalf("flow pull output missing %q:\n%s", want, pullOutput)
		}
	}
	if strings.Contains(pullOutput, workspaceRoot) || strings.Contains(pullOutput, filepath.ToSlash(workspaceRoot)) {
		t.Fatalf("flow pull exposed the runtime workspace root:\n%s", pullOutput)
	}
	flowEntries, err := os.ReadDir(filepath.Join(workspaceRoot, "artifacts", "flow"))
	if err != nil || len(flowEntries) != 1 {
		t.Fatalf("flow artifact entries = %#v, error = %v", flowEntries, err)
	}
	flowArtifact := filepath.Join(workspaceRoot, "artifacts", "flow", flowEntries[0].Name())
	for _, name := range []string{"Daily Prep.tflx", "metadata.json", "lineage.json"} {
		if _, err := os.Stat(filepath.Join(flowArtifact, name)); err != nil {
			t.Fatalf("flow artifact missing %s: %v", name, err)
		}
	}
	payload, err := os.ReadFile(filepath.Join(flowArtifact, "Daily Prep.tflx"))
	if err != nil || string(payload) != "native\x00flow\r\nbytes" {
		t.Fatalf("native flow payload = %q, error = %v", payload, err)
	}

	lineageOutput := runGroupOneCLI(t, options, "content", "lineage", "pull", "--workspace", "operations", "--kind", "flow", "--id", "flow-1")
	for _, want := range []string{"status: pulled", "kind: flow", "luid: flow-1", "complete: true", "artifacts/lineage/flow/", "details: \"--full\""} {
		if !strings.Contains(lineageOutput, want) {
			t.Fatalf("lineage pull output missing %q:\n%s", want, lineageOutput)
		}
	}
	if strings.Contains(lineageOutput, workspaceRoot) || strings.Contains(lineageOutput, filepath.ToSlash(workspaceRoot)) {
		t.Fatalf("lineage pull exposed the runtime workspace root:\n%s", lineageOutput)
	}
	if mutations.Load() != 0 {
		t.Fatalf("read and local pull commands made %d remote mutation requests", mutations.Load())
	}
}

// TestGroupOneLiveReadSucceedsWhenCatalogWriteThroughFails proves that a live
// read remains authoritative even when the local catalog write-through cannot
// persist. The catalog directory is poisoned with a regular file so the store
// cannot create catalog/catalog.sqlite; the read must still return the live
// result with a Tableau source stamp and a bounded warning rather than failing
// on the cache write or issuing an unsafe cursor for an older snapshot.
func TestGroupOneLiveReadSucceedsWhenCatalogWriteThroughFails(t *testing.T) {
	server, mutations := newGroupOneTableauServer(t)
	defer server.Close()

	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")

	// Poison the catalog directory: a regular file where the store needs a
	// directory forces complete scope publication to fail.
	catalogPath := filepath.Join(filepath.Dir(configPath), "catalog")
	if err := os.WriteFile(catalogPath, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}

	options := app.Options{
		ConfigPath: configPath,
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
	}
	output := runGroupOneCLI(t, options, "content", "project", "list", "--all")
	for _, want := range []string{"status: listed", "environment: production", "project-ops", "mode: tableau", "catalog_warning:", "catalog was not updated"} {
		if !strings.Contains(output, want) {
			t.Fatalf("live read did not survive a failed catalog write-through; output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "next_cursor:") || strings.Contains(output, configPath) {
		t.Fatalf("failed catalog publication exposed an unsafe cursor or local path:\n%s", output)
	}
	// The catalog file must remain the untouched poison, proving write-through
	// neither succeeded nor removed it.
	if data, err := os.ReadFile(catalogPath); err != nil || string(data) != "not a directory" {
		t.Fatalf("catalog poison file = %q, error = %v", data, err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("live read made %d remote mutation requests", mutations.Load())
	}
}

func TestGroupOneFlowMutationPreviewsDoNotMutateThroughCLI(t *testing.T) {
	server, mutations := newGroupOneTableauServer(t)
	defer server.Close()

	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspaceRoot := createNamedWorkspace(t, configPath, "operations")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{
		MutationsEnabled: false,
		ConfigPath:       configPath,
		HTTPClient:       server.Client(),
		Now:              func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) },
	}

	runGroupOneCLI(t, options, "content", "flow", "pull", "--workspace", "operations", "--id", "flow-1")
	entries, err := os.ReadDir(filepath.Join(workspaceRoot, "artifacts", "flow"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("flow artifact entries = %#v, error = %v", entries, err)
	}
	artifactSelector := filepath.ToSlash(filepath.Join("artifacts", "flow", entries[0].Name()))

	publishOutput := runGroupOneCLI(t, options,
		"content", "flow", "publish",
		"--workspace", "operations",
		"--artifact", artifactSelector,
		"--name", "Daily Copy",
		"--preview",
	)
	for _, want := range []string{"mode: preview", "operation: flow.publish", "artifact_path: " + artifactSelector, "environment: production", "project_luid: project-ops"} {
		if !strings.Contains(publishOutput, want) {
			t.Fatalf("flow publish preview missing %q:\n%s", want, publishOutput)
		}
	}
	if strings.Contains(publishOutput, workspaceRoot) || strings.Contains(publishOutput, filepath.ToSlash(workspaceRoot)) {
		t.Fatalf("flow publish preview exposed the runtime workspace root:\n%s", publishOutput)
	}

	moveOutput := runGroupOneCLI(t, options,
		"content", "flow", "move",
		"--environment", "production",
		"--id", "flow-1",
		"--destination-project-id", "project-destination",
		"--preview",
	)
	for _, want := range []string{"mode: preview", "operation: flow.move", "luid: project-destination"} {
		if !strings.Contains(moveOutput, want) {
			t.Fatalf("flow move preview missing %q:\n%s", want, moveOutput)
		}
	}

	deleteOutput := runGroupOneCLI(t, options,
		"content", "flow", "delete",
		"--environment", "production",
		"--id", "flow-1",
		"--preview",
	)
	for _, want := range []string{"mode: preview", "operation: flow.delete", "luid: flow-1"} {
		if !strings.Contains(deleteOutput, want) {
			t.Fatalf("flow delete preview missing %q:\n%s", want, deleteOutput)
		}
	}
	if mutations.Load() != 0 {
		t.Fatalf("preview-only commands made %d remote mutation requests", mutations.Load())
	}
}

func runGroupOneCLI(t *testing.T, options app.Options, args ...string) string {
	t.Helper()
	var output strings.Builder
	if exit := app.Run(context.Background(), args, &output, options); exit != 0 {
		t.Fatalf("%s exit = %d, output:\n%s", strings.Join(args, " "), exit, output.String())
	}
	return output.String()
}

func newGroupOneTableauServer(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	mutations := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			writeGroupOneProjects(writer, request)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/flows/flow-1":
			writer.Header().Set("Content-Type", "application/xml")
			writer.Header().Set("X-Tableau-Request-Id", "flow-get-request")
			_, _ = io.WriteString(writer, `<tsResponse><flowOutputSteps><flowOutputStep id="output-1" name="Published Output"/></flowOutputSteps><flow id="flow-1" name="Daily Prep" description="Daily preparation" fileType="tflx" createdAt="2026-08-01T00:00:00Z" updatedAt="2026-08-02T00:00:00Z"><project id="project-ops" name="Operations"/><owner id="owner-1"/><tags><tag label="certified"/></tags><parameters/></flow></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/flows/flow-1/content":
			writer.Header().Set("Content-Disposition", `attachment; filename="Daily Prep.tflx"`)
			writer.Header().Set("X-Tableau-Request-Id", "flow-download-request")
			_, _ = writer.Write([]byte("native\x00flow\r\nbytes"))
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/flows":
			writeGroupOneFlows(writer, request)
		case request.Method == http.MethodPost && request.URL.Path == "/api/metadata/graphql":
			writeGroupOneLineage(t, writer, request)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/flows":
			mutations.Add(1)
			http.Error(writer, "unexpected publish", http.StatusInternalServerError)
		case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/api/3.29/sites/site-1/flows/"):
			mutations.Add(1)
			http.Error(writer, "unexpected move", http.StatusInternalServerError)
		case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/3.29/sites/site-1/flows/"):
			mutations.Add(1)
			http.Error(writer, "unexpected delete", http.StatusInternalServerError)
		default:
			t.Errorf("unexpected Tableau request: %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	return server, mutations
}

func writeGroupOneProjects(writer http.ResponseWriter, request *http.Request) {
	pageNumber := request.URL.Query().Get("pageNumber")
	pageSize := request.URL.Query().Get("pageSize")
	if pageNumber == "" {
		pageNumber = "1"
	}
	if pageSize == "" {
		pageSize = "1000"
	}
	writer.Header().Set("Content-Type", "application/xml")
	_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="3"/><projects><project id="department" name="Department" topLevelProject="true"/><project id="project-ops" name="Operations" parentProjectId="department" topLevelProject="false"/><project id="project-destination" name="Destination" parentProjectId="department" topLevelProject="false"/></projects></tsResponse>`, pageNumber, pageSize)
}

func writeGroupOneFlows(writer http.ResponseWriter, request *http.Request) {
	pageNumber := request.URL.Query().Get("pageNumber")
	pageSize := request.URL.Query().Get("pageSize")
	if pageNumber == "" {
		pageNumber = "1"
	}
	if pageSize == "" {
		pageSize = "1000"
	}
	filter := request.URL.Query().Get("filter")
	writer.Header().Set("Content-Type", "application/xml")
	if strings.Contains(filter, "name:eq:Daily Copy") {
		_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><flows/></tsResponse>`, pageNumber, pageSize)
		return
	}
	_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><flows><flow id="flow-1" name="Daily Prep" description="Daily preparation" fileType="tflx" createdAt="2026-08-01T00:00:00Z" updatedAt="2026-08-02T00:00:00Z"><project id="project-ops" name="Operations"/><owner id="owner-1"/></flow></flows></tsResponse>`, pageNumber, pageSize)
}

func writeGroupOneLineage(t *testing.T, writer http.ResponseWriter, request *http.Request) {
	t.Helper()
	var body struct {
		Query string `json:"query"`
	}
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		t.Errorf("decode Metadata request: %v", err)
		http.Error(writer, "invalid request", http.StatusBadRequest)
		return
	}
	field := ""
	for _, candidate := range []string{
		"upstreamDatasourcesConnection",
		"upstreamLinkedFlowsConnection",
		"downstreamDatasourcesConnection",
		"downstreamLinkedFlowsConnection",
		"downstreamWorkbooksConnection",
	} {
		if strings.Contains(body.Query, candidate) {
			field = candidate
			break
		}
	}
	node := map[string]any{"id": "flow-meta", "luid": "flow-1", "name": "Daily Prep"}
	if field != "" {
		node[field] = map[string]any{
			"totalCount": 0,
			"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": ""},
			"nodes":      []any{},
		}
	}
	response := map[string]any{"data": map[string]any{"flowsConnection": map[string]any{
		"totalCount": 1,
		"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": ""},
		"nodes":      []any{node},
	}}}
	writer.Header().Set("Content-Type", "application/json")
	writer.Header().Set("X-Tableau-Request-Id", "metadata-request")
	if err := json.NewEncoder(writer).Encode(response); err != nil {
		t.Errorf("encode Metadata response: %v", err)
	}
}

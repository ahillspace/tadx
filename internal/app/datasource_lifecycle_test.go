package app

import (
	"context"
	"errors"
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

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	datasourcepublish "github.com/ahillspace/tadx/actions/datasource/publish"
	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func TestDatasourceLifecycleCompositionPreservesCompositionIdentityAndRelativePaths(t *testing.T) {
	var deletes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/auth/signin":
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1":
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-1" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1/content":
			writer.Header().Set("Content-Disposition", `attachment; filename="Sales.tds"`)
			_, _ = io.WriteString(writer, `<datasource><relation datasource-url="parent-sales"/></datasource>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/projects":
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, pageNumber, pageSize)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			pageNumber, pageSize := request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize")
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><datasources><datasource id="ds-1" name="Sales"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`, pageNumber, pageSize)
		case request.Method == http.MethodDelete && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1":
			deletes.Add(1)
			writer.Header().Set("X-Tableau-Request-Id", "delete-request")
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "metadata unavailable", http.StatusNotFound)
		}
	}))
	defer server.Close()

	runtime, workspace := datasourceLifecycleRuntime(t, server)
	commands := newRemoteContentCommands(runtime)
	pulled, err := commands.PullDatasource(context.Background(), datasourcepull.Input{Environment: "production", Workspace: "analytics", Selector: identity.Selector{LUID: "ds-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if pulled.Artifact.CompositionStatus != artifact.CompositionStatusComposed || strings.Join(pulled.Artifact.ParentDataSourceURLs, ",") != "parent-sales" {
		t.Fatalf("pull composition = %#v", pulled.Artifact)
	}
	for _, path := range []string{pulled.Artifact.Path, pulled.Artifact.CanonicalPath, pulled.Artifact.LineagePath} {
		if filepath.IsAbs(path) || strings.Contains(path, `\`) || strings.Contains(path, workspace) {
			t.Fatalf("pull exposed non-portable path %q", path)
		}
	}
	stored, err := artifact.NewDatasourceManager(runtime.now).Read(context.Background(), filepath.Join(workspace, filepath.FromSlash(pulled.Artifact.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if stored.TableauID != "ds-1" || stored.SourceProjectID != "project-1" || stored.CompositionStatus != artifact.CompositionStatusComposed || strings.Join(stored.ParentDataSourceURLs, ",") != "parent-sales" {
		t.Fatalf("stored artifact = %#v", stored)
	}

	preview, err := commands.PublishDatasource(context.Background(), datasourcepublish.Input{Workspace: "analytics", ArtifactPath: pulled.Artifact.Path, Environment: "production", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourcepublish.ModeOverwrite}, true)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Result != nil || preview.Plan.Target.ExistingLUID != "ds-1" || preview.Plan.Target.ProjectLUID != "project-1" || preview.Plan.ArtifactPath != pulled.Artifact.Path || preview.Plan.CompositionStatus != artifact.CompositionStatusComposed || strings.Join(preview.Plan.ParentDataSourceURLs, ",") != "parent-sales" {
		t.Fatalf("publish preview = %#v", preview)
	}

	deleted, err := commands.DeleteDatasource(context.Background(), datasourcedelete.Input{Environment: "production", Selector: identity.Selector{LUID: "ds-1"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	if deleted.Result == nil || deleted.Result.DatasourceLUID != "ds-1" || deleted.Result.TableauRequestID != "delete-request" || deletes.Load() != 1 {
		t.Fatalf("delete = %#v, calls = %d", deleted, deletes.Load())
	}
}

func TestDatasourceLifecycleReturnsStructuredSetupErrors(t *testing.T) {
	commands := newRemoteContentCommands(&runtimeDependencies{configPath: filepath.Join(t.TempDir(), "missing.yaml"), httpClient: http.DefaultClient, now: time.Now, correlationID: "datasource-test"})
	_, err := commands.PullDatasource(context.Background(), datasourcepull.Input{Environment: "production", Workspace: "analytics", Selector: identity.Selector{LUID: "ds-1"}})
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.pull.workspace" || structured.Operation != "datasource.pull" || structured.Environment != "production" {
		t.Fatalf("error = %#v", err)
	}
}

func TestDatasourcePublishModeMappingIsExhaustive(t *testing.T) {
	for _, mode := range []datasourcepublish.Mode{datasourcepublish.ModeCreate, datasourcepublish.ModeOverwrite, datasourcepublish.ModeAppend, datasourcepublish.ModeReplace} {
		if _, err := datasourcePublishMode(mode); err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
	if _, err := datasourcePublishMode("unexpected"); err == nil {
		t.Fatal("unexpected mode succeeded")
	}
}

func datasourceLifecycleRuntime(t *testing.T, server *httptest.Server) (*runtimeDependencies, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	contents := fmt.Sprintf("version: 1\ndefault_environment: production\nenvironments:\n  production:\n    url: %s\n    site_content_url: team-site\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: PROD_PAT_NAME\n      pat_secret_env: PROD_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	workspace := filepath.Join(t.TempDir(), "analytics")
	if _, err := workspacecore.NewManager(configPath, nil).Create(context.Background(), "analytics", workspace); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	runtime := &runtimeDependencies{configPath: configPath, httpClient: server.Client(), now: func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) }, correlationID: "datasource-test"}
	t.Cleanup(func() { _ = runtime.Close() })
	return runtime, workspace
}

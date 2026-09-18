package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDatasourceInspectAcceptsImportedDisplayNameThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/metadata/graphql":
			w.WriteHeader(http.StatusForbidden)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/projects":
			io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="imported-project" name="(imported)"/></projects></tsResponse>`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources":
			io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><datasources><datasource id="ds-imported" name="Superstore Sales Cloud"><project id="imported-project" name="(imported)"/></datasource></datasources></tsResponse>`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources/ds-imported":
			io.WriteString(w, `<tsResponse><datasource id="ds-imported" name="Superstore Sales Cloud"><project id="imported-project" name="(imported)"/></datasource></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: IMPORTED_PAT_NAME\n      pat_secret_env: IMPORTED_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IMPORTED_PAT_NAME", "test-pat")
	t.Setenv("IMPORTED_PAT_SECRET", "test-secret")
	for _, selector := range []string{"Imported", "imported", "(imported)"} {
		var stdout bytes.Buffer
		code := app.Run(context.Background(), []string{"content", "datasource", "inspect", "--environment", "test", "--name", "Superstore Sales Cloud", "--project", selector}, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
		if code != 0 || !strings.Contains(stdout.String(), "luid: ds-imported") || !strings.Contains(stdout.String(), "(imported)") {
			t.Fatalf("selector=%q code=%d output=%s", selector, code, stdout.String())
		}
	}
	var stdout bytes.Buffer
	code := app.Run(context.Background(), []string{"content", "datasource", "inspect", "--environment", "test", "--name", "Superstore Sales Cloud", "--project-id", "imported-project"}, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if code != 0 || !strings.Contains(stdout.String(), "luid: ds-imported") {
		t.Fatalf("project-id inspect code=%d output=%s", code, stdout.String())
	}
}

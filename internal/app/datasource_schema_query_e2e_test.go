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

func TestDatasourceSchemaQueryMatchesFieldsNotTableThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1":
			io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Sales"><project id="p1" name="Test"/></datasource></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/vizql-data-service/read-metadata":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"data":[{"fieldName":"Sales Amount","fieldCaption":"Sales Amount","dataType":"REAL","fieldRole":"MEASURE","logicalTableId":"Sales"},{"fieldName":"Profit","fieldCaption":"Profit","dataType":"REAL","fieldRole":"MEASURE","logicalTableId":"Sales"},{"fieldName":"Order Date","fieldCaption":"Order Date","dataType":"DATE","fieldRole":"DIMENSION","logicalTableId":"Sales"}]}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: SCHEMA_QUERY_PAT_NAME\n      pat_secret_env: SCHEMA_QUERY_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SCHEMA_QUERY_PAT_NAME", "test-pat")
	t.Setenv("SCHEMA_QUERY_PAT_SECRET", "test-secret")
	var stdout bytes.Buffer
	code := app.Run(context.Background(), []string{"content", "datasource", "schema", "--environment", "test", "--id", "ds-1", "--query", "Sales"}, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if code != 0 || !strings.Contains(stdout.String(), "Sales Amount") || strings.Contains(stdout.String(), "Profit") || strings.Contains(stdout.String(), "Order Date") || !strings.Contains(stdout.String(), "returned: 1") {
		t.Fatalf("code=%d output=%s", code, stdout.String())
	}
}

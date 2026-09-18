package app

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatasourceAppendAcceptsPreparedHyperThroughCLI(t *testing.T) {
	writes := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/datasources"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><datasources><datasource id="ds-1" name="Sales"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		default:
			writes++
			http.Error(w, "preview must not write", 400)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Sales.hyper")
	// Binary compatibility belongs to Tableau; preview fingerprints without a local Hyper engine.
	if err := os.WriteFile(file, []byte("opaque prepared extract fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	exit := Run(t.Context(), []string{"content", "datasource", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--append", "--preview", "--json"}, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client()})
	if exit != 0 || writes != 0 || !strings.Contains(out.String(), `"publish_mode":"append"`) || !strings.Contains(out.String(), "Sales.hyper") {
		t.Fatalf("exit=%d writes=%d output=%s", exit, writes, out.String())
	}
}

func TestDatasourceAppendRejectsPackageBeforeSetup(t *testing.T) {
	var out strings.Builder
	exit := Run(t.Context(), []string{"content", "datasource", "publish", "--file", "Sales.tdsx", "--environment", "production", "--project-id", "project-1", "--append", "--preview"}, &out, Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yml")})
	if exit == 0 || !strings.Contains(out.String(), "prepared .hyper") {
		t.Fatalf("exit=%d output=%s", exit, out.String())
	}
}

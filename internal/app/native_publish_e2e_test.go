package app

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/artifact"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativePublishPreviewThroughCLIWithoutManagedArtifact(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			writes := 0
			expectedBody := map[string]string{"workbook": "<workbook/>", "datasource": "<datasource/>", "flow": `{"nodes":{}}`}[kind]
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/user-1"):
					io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="Creator"/></tsResponse>`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><%ss/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"), kind)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks/validateWorkbook"):
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"errors":[],"warnings":[]}`)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					writes++
					if r.URL.Query().Get("asJob") == "true" {
						t.Error("non-administrator publication must retain the synchronous contract")
					}
					body, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(body), expectedBody) {
						t.Errorf("publish omitted original native bytes")
					}
					w.Header().Set("Content-Type", "application/xml")
					w.WriteHeader(http.StatusCreated)
					fmt.Fprintf(w, `<tsResponse><%s id="created-1" name="Native"><project id="project-1"/></%s></tsResponse>`, kind, kind)
				case strings.HasSuffix(r.URL.Path, "/auth/signout"):
					w.WriteHeader(204)
				default:
					writes++
					http.Error(w, "unexpected request", 400)
				}
			}))
			defer server.Close()
			runtime, workspace := datasourceLifecycleRuntime(t, server)
			extension, definition := ".twb", "<workbook/>"
			if kind == "datasource" {
				extension, definition = ".tds", "<datasource/>"
			}
			if kind == "flow" {
				extension, definition = ".tfl", `{"nodes":{}}`
			}
			file := filepath.Join(t.TempDir(), "Native"+extension)
			if err := os.WriteFile(file, []byte(definition), 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{"content", kind, "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--preview"}
			if kind == "datasource" {
				args = append(args, "--create")
			}
			var out strings.Builder
			if exit := Run(context.Background(), args, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client()}); exit != 0 {
				t.Fatalf("exit=%d %s", exit, out.String())
			}
			if writes != 0 || !strings.Contains(out.String(), "Native") || !strings.Contains(out.String(), filepath.ToSlash(file)) || strings.Contains(out.String(), "fingerprint") {
				t.Fatalf("writes=%d output=%s", writes, out.String())
			}
			inventory, err := artifact.Inventory(context.Background(), workspace, artifact.InventoryOptions{Limit: 20})
			if err != nil || inventory.Total != 0 {
				t.Fatalf("native preview changed inventory: %#v %v", inventory, err)
			}
			applyArgs := []string{}
			for _, arg := range args {
				if arg != "--preview" {
					applyArgs = append(applyArgs, arg)
				}
			}
			out.Reset()
			if exit := Run(context.Background(), applyArgs, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), MutationsEnabled: true, JobDirectory: t.TempDir()}); exit != 0 {
				t.Fatalf("apply exit%d %s", exit, out.String())
			}
			if writes != 1 || !strings.Contains(out.String(), "created-1") {
				t.Fatalf("writes%d output%s", writes, out.String())
			}
		})
	}
}

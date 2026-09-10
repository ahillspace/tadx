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
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestRoutinePullKeepsOptionalEnrichmentDiagnosticsFullOnly(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.URL.Path == "/api/3.29/sites/site-1/projects":
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Shared"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s/item-1/content":
					ext := map[string]string{"workbook": "twb", "datasource": "tds", "flow": "tfl"}[kind]
					w.Header().Set("Content-Disposition", `attachment; filename="Example.`+ext+`"`)
					_, _ = io.WriteString(w, "<"+kind+"/>")
				case r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s/item-1":
					_, _ = fmt.Fprintf(w, `<tsResponse><%s id="item-1" name="Example" fileType="tfl"><project id="project-1" name="Shared"/></%s></tsResponse>`, kind, kind)
				default:
					http.Error(w, "metadata unavailable", http.StatusForbidden)
				}
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			workspaceRoot := filepath.Join(t.TempDir(), "workspace")
			runGroupOneCLI(t, options, "workspace", "create", "work", "--path", workspaceRoot)
			output := runGroupOneCLI(t, options, "content", kind, "pull", "--id", "item-1", "--workspace", "work", "--environment", "test")
			if !strings.Contains(output, "status: pulled") || strings.Contains(output, "Lineage capture") || strings.Contains(output, "portability: unknown") || strings.Contains(output, "portability remains unknown") {
				t.Fatalf("routine pull was noisy: %s", output)
			}
			full := runGroupOneCLI(t, options, "last")
			if !strings.Contains(full, "Lineage capture") {
				t.Fatalf("saved full result lost enrichment diagnostics: %s", full)
			}
			paths, err := filepath.Glob(filepath.Join(workspaceRoot, "artifacts", kind, "*", "lineage.json"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("lineage sidecar missing: %v %v", paths, err)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			var lineage map[string]any
			if err := json.Unmarshal(data, &lineage); err != nil || lineage["complete"] != false {
				t.Fatalf("lineage metadata no longer records uncertainty: %s %v", data, err)
			}
			if kind == "workbook" {
				var failure strings.Builder
				code := app.Run(context.Background(), []string{"content", "workbook", "pull", "--id", "item-1", "--workspace", "work", "--environment", "test", "--include-pds"}, &failure, options)
				if code == 0 || !strings.Contains(failure.String(), "workbook.pull.references") {
					t.Fatalf("explicit incomplete acquisition was hidden: %s", failure.String())
				}
			}
		})
	}
}

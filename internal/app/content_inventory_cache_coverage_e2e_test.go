package app_test

import (
	"context"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestLimitedLiveInventoryDoesNotCollectOrDependOnCacheThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		t.Run(kind, func(t *testing.T) {
			reads := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				resource, total := kind, 205
				if strings.HasSuffix(r.URL.Path, "/projects") && kind != "project" {
					resource, total = "project", 1
				} else {
					reads++
					if r.URL.Query().Get("pageSize") != "3" || r.URL.Query().Get("pageNumber") != "1" {
						t.Errorf("limited read expanded provider request: %s", r.URL.RawQuery)
					}
				}
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				if size <= 0 {
					size = 1000
				}
				_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%d" totalAvailable="%d"/><%ss>`, size, total, resource)
				for i := 0; i < min(size, total); i++ {
					_, _ = fmt.Fprintf(w, `<%s id="%s-%03d" name="Item %03d" siteRole="Viewer" type="hyper" fileType="tflx"><project id="project-000" name="Item 000"/><owner id="user-1"/></%s>`, resource, resource, i, i, resource)
				}
				_, _ = fmt.Fprintf(w, `</%ss></tsResponse>`, resource)
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			// An unusable SQLite location must not participate in a limited live answer.
			if err := os.WriteFile(filepath.Join(filepath.Dir(options.ConfigPath), "catalog"), []byte("not a directory"), 0600); err != nil {
				t.Fatal(err)
			}
			root := "content"
			if kind == "user" || kind == "group" {
				root = "admin"
			}
			var out strings.Builder
			exit := app.Run(context.Background(), []string{root, kind, "list", "--environment", "production", "--limit", "3"}, &out, options)
			if exit != 0 || reads != 1 || !strings.Contains(out.String(), "returned: 3") || !strings.Contains(out.String(), "more_available: true") {
				t.Fatalf("reads=%d exit=%d output=%s", reads, exit, out.String())
			}
		})
	}
}

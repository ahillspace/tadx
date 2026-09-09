package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestInventoryAllThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		t.Run(kind, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				resource := kind
				total := 205
				if strings.HasSuffix(r.URL.Path, "/projects") && kind != "project" {
					resource = "project"
					total = 1
				}
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				if size == 0 {
					size = 1000
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
				if page == 0 {
					page = 1
				}
				_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><%ss>`, page, size, total, resource)
				for i := (page - 1) * size; i < min(page*size, total); i++ {
					_, _ = fmt.Fprintf(w, `<%s id="%s-%03d" name="Item %03d" siteRole="Viewer" type="hyper" fileType="tflx"><project id="project-000"/><owner id="user-1"/></%s>`, resource, resource, i, i, resource)
				}
				_, _ = fmt.Fprintf(w, `</%ss></tsResponse>`, resource)
			}))
			defer server.Close()
			options := catalogResilienceOptions(t, server)
			root := "content"
			if kind == "user" || kind == "group" {
				root = "admin"
			}
			for _, cached := range []bool{false, true} {
				args := []string{root, kind, "list", "--environment", "production", "--all", "--full"}
				if cached {
					args = append(args, "--catalog")
				}
				var out strings.Builder
				exit := app.Run(context.Background(), args, &out, options)
				if exit != 0 || !strings.Contains(out.String(), "returned: 205") || !strings.Contains(out.String(), "more_available: false") || strings.Contains(out.String(), "next_cursor") {
					t.Fatalf("args=%v exit=%d output=%s", args, exit, out.String())
				}
			}
		})
	}
}

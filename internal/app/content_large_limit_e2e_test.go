package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestContentLargeLimitsUseBoundedProviderPagesThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project"} {
		t.Run(kind, func(t *testing.T) {
			var resourcePages []int
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/auth/signout") {
					w.WriteHeader(204)
					return
				}
				page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				resource := kind
				total := 4000
				if strings.HasSuffix(r.URL.Path, "/projects") && kind != "project" {
					resource = "project"
					total = 1
				} else if !strings.HasSuffix(r.URL.Path, "/"+kind+"s") {
					http.Error(w, "unexpected", 400)
					return
				} else {
					resourcePages = append(resourcePages, size)
				}
				if size > 1000 || size < 1 {
					t.Errorf("invalid provider size %d", size)
				}
				fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><%ss>`, page, size, total, resource)
				for i := (page - 1) * size; i < min(page*size, total); i++ {
					if resource == "project" {
						fmt.Fprintf(w, `<project id="project-%d" name="Project%d" topLevelProject="true"/>`, i, i)
					} else {
						fmt.Fprintf(w, `<%s id="item-%d" name="Item%d"><project id="project-0" name="Project0"/></%s>`, resource, i, i, resource)
					}
				}
				fmt.Fprintf(w, `</%ss></tsResponse>`, resource)
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			for _, limit := range []int{20, 1501} {
				resourcePages = nil
				var out strings.Builder
				exit := Run(context.Background(), []string{"content", kind, "list", "--environment", "production", "--limit", strconv.Itoa(limit)}, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client()})
				if exit != 0 {
					t.Fatalf("limit%d exit%d %s", limit, exit, out.String())
				}
				wantCalls := 1
				if limit > 1000 {
					wantCalls = 2
				}
				if len(resourcePages) != wantCalls || !strings.Contains(out.String(), fmt.Sprintf("returned: %d", limit)) || !strings.Contains(out.String(), "more_available: true") {
					t.Fatalf("limit%d pages=%v output=%s", limit, resourcePages, out.String())
				}
			}
		})
	}
}

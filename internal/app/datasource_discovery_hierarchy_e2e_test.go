package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDatasourceDiscoveryReusesHierarchyOnlyWithinInvocationThroughCLI(t *testing.T) {
	projectReads, datasourceReads := 0, 0
	revision := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		number, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
		size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
		switch {
		case strings.HasSuffix(r.URL.Path, "/projects"):
			projectReads++
			if size != 1000 {
				t.Errorf("project page size=%d", size)
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="1001"/><projects>`, number, size)
			for i := (number - 1) * size; i < min(number*size, 1001); i++ {
				if i == 1000 {
					parent, name := "p-0", "Orders"
					if revision > 0 {
						parent, name = "p-1", "Renamed"
					}
					_, _ = fmt.Fprintf(w, `<project id="child" name="%s" parentProjectId="%s"/>`, name, parent)
				} else {
					_, _ = fmt.Fprintf(w, `<project id="p-%d" name="Root%d"/>`, i, i)
				}
			}
			_, _ = io.WriteString(w, `</projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/datasources"):
			datasourceReads++
			if size > 100 {
				t.Errorf("datasource page exceeds requested bound: %d", size)
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="205"/><datasources>`, number, size)
			for i := (number - 1) * size; i < min(number*size, 205); i++ {
				_, _ = fmt.Fprintf(w, `<datasource id="ds-%03d" name="Data %03d" type="hyper"><project id="child" name="Orders"/></datasource>`, i, i)
			}
			_, _ = io.WriteString(w, `</datasources></tsResponse>`)
		default:
			t.Errorf("unexpected request %s", r.URL)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := catalogResilienceOptions(t, server)
	poisoned := filepath.Join(filepath.Dir(options.ConfigPath), "catalog")
	if err := os.WriteFile(poisoned, []byte("not a directory"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		projectReads, datasourceReads = 0, 0
		args := []string{"search", "--type", "datasource", "--environment", "production", "--limit", "205"}
		if full {
			args = append(args, "--full")
		}
		var out strings.Builder
		exit := app.Run(context.Background(), args, &out, options)
		expected := "Root0/Orders"
		if revision > 0 {
			expected = "Root1/Renamed"
		}
		if exit != 0 || projectReads != 2 || datasourceReads != 3 || !strings.Contains(out.String(), expected) || !strings.Contains(out.String(), "returned: 205") {
			t.Fatalf("full=%t exit=%d project reads=%d datasource reads=%d output=%s", full, exit, projectReads, datasourceReads, out.String())
		}
		revision++
	}
	// A later standalone list receives its own fresh lazy hierarchy as well.
	projectReads, datasourceReads = 0, 0
	out := runGroupOneCLI(t, options, "content", "datasource", "list", "--environment", "production", "--limit", "20", "--full")
	if projectReads != 2 || datasourceReads != 1 || !strings.Contains(out, "Root1/Renamed") {
		t.Fatalf("standalone reads=%d/%d output=%s", projectReads, datasourceReads, out)
	}
	if data, err := os.ReadFile(poisoned); err != nil || string(data) != "not a directory" {
		t.Fatal("discovery changed catalog state")
	}
}

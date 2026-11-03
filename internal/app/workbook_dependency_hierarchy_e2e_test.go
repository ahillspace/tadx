package app_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookDependencyPullSharesDiscoveryHierarchy(t *testing.T) {
	const dependencies = 20
	var projectReads, downloads atomic.Int32
	var workbook strings.Builder
	workbook.WriteString(`<workbook><datasources>`)
	for index := range dependencies {
		fmt.Fprintf(&workbook, `<datasource name="source-%d"><connection class="sqlproxy" dbname="Source%d" server="tableau.example.test"/></datasource>`, index, index)
	}
	workbook.WriteString(`</datasources></workbook>`)
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case strings.HasSuffix(r.URL.Path, "/workbooks/book-1"):
			fmt.Fprint(w, `<tsResponse><workbook id="book-1" name="Book"><project id="project-1" name="Operations"/></workbook></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			projectReads.Add(1)
			fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="parent-1" name="Department"/><project id="project-1" name="Operations" parentProjectId="parent-1"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/workbooks/book-1/content"):
			w.Header().Set("Content-Disposition", `attachment; filename="Book.twb"`)
			fmt.Fprint(w, workbook.String())
		case strings.HasSuffix(r.URL.Path, "/datasources"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="%d"/><datasources>`, dependencies)
			for index := range dependencies {
				name := fmt.Sprintf("Source%d", index)
				fmt.Fprintf(w, `<datasource id="%s" name="%s" contentUrl="%s"><project id="project-1" name="Operations"/></datasource>`, name, name, name)
			}
			fmt.Fprint(w, `</datasources></tsResponse>`)
		case strings.Contains(r.URL.Path, "/datasources/"):
			name := strings.TrimSuffix(r.URL.Path[strings.LastIndex(r.URL.Path, "/datasources/")+len("/datasources/"):], "/content")
			if strings.HasSuffix(r.URL.Path, "/content") {
				downloads.Add(1)
				w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.tds"`, name))
				fmt.Fprint(w, `<datasource><connection class="postgres"/></datasource>`)
			} else {
				fmt.Fprintf(w, `<tsResponse><datasource id="%s" name="%s"><project id="project-1" name="Operations"/></datasource></tsResponse>`, name, name)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	root := createNamedWorkspace(t, options.ConfigPath, "work")
	var out strings.Builder
	if code := app.Run(t.Context(), []string{"content", "workbook", "pull", "--env", "test", "--workspace", "work", "--id", "book-1", "--include-pds", "--json"}, &out, options); code != 0 {
		t.Fatalf("code=%d output=%s", code, out.String())
	}
	entries, err := os.ReadDir(filepath.Join(root, "artifacts", "datasource"))
	if err != nil || len(entries) != dependencies || downloads.Load() != dependencies {
		t.Fatalf("dependencies=%d downloads=%d err=%v output=%s", len(entries), downloads.Load(), err, out.String())
	}
	t.Logf("downloaded %d dependencies with %d project hierarchy reads", downloads.Load(), projectReads.Load())
	if projectReads.Load() != 1 {
		t.Fatalf("hierarchy reads=%d, want one shared discovery snapshot", projectReads.Load())
	}
}

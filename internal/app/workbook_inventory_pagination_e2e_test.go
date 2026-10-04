package app_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProjectWorkbookAllScansOnceThroughCLI(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprintf("drift=%t", drift), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><projects><project id="project-1" name="Ops"/><project id="project-2" name="Other"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/workbooks"):
					calls.Add(1)
					if strings.Contains(r.URL.Query().Get("filter"), "projectId") {
						t.Error("unsupported projectId filter")
					}
					number, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
					size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
					if number < 1 || size < 1 {
						http.Error(w, "invalid page", 400)
						return
					}
					total := 5000
					if drift && number > 1 {
						total++
					}
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><workbooks>`, number, size, total)
					for index := (number - 1) * size; index < min(number*size, 5000); index++ {
						project := "project-2"
						if index%2 == 0 {
							project = "project-1"
						}
						fmt.Fprintf(w, `<workbook id="wb-%04d" name="Workbook %04d"><project id="%s"/><owner id="user-1"/></workbook>`, index, index, project)
					}
					fmt.Fprint(w, `</workbooks></tsResponse>`)
				default:
					t.Errorf("unexpected request: %s", r.URL)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			code, payload := resultContractJSON(t, []string{"content", "workbook", "list", "--environment", "test", "--project-id", "project-1", "--all"}, diagnosticOptions(t, server))
			t.Logf("workbook inventory GETs=%d", calls.Load())
			if drift {
				if code == 0 {
					t.Fatalf("pagination drift accepted: %#v", payload)
				}
				return
			}
			if code != 0 {
				t.Fatalf("list failed: %#v", payload)
			}
			items, ok := payload["workbooks"].([]any)
			if !ok || len(items) != 2500 {
				t.Fatalf("returned %d rows", len(items))
			}
			seen := map[string]bool{}
			for _, raw := range items {
				item := contractObject(t, raw)
				id, _ := item["luid"].(string)
				index, err := strconv.Atoi(strings.TrimPrefix(id, "wb-"))
				if err != nil || index < 0 || index >= 5000 || index%2 != 0 || seen[id] || item["project_luid"] != "project-1" {
					t.Fatalf("incorrect identity: %#v", item)
				}
				seen[id] = true
			}
			if calls.Load() != 5 {
				t.Fatalf("workbook inventory GETs=%d, want 5", calls.Load())
			}
		})
	}
}
